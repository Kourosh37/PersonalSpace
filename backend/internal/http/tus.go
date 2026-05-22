package httpapi

import (
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"space/backend/internal/middleware"
	"space/backend/internal/observability"
	"space/backend/internal/settings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

const tusVersion = "1.0.0"

func (h Handler) registerTusRoutes(api chi.Router, authMW middleware.AuthMiddleware) {
	api.Group(func(r chi.Router) {
		r.Use(authMW.RequireAuth)
		r.Options("/uploads/tus", h.tusOptions)
		r.Post("/uploads/tus", h.tusCreate)
		r.Options("/uploads/tus/{id}", h.tusOptions)
		r.Head("/uploads/tus/{id}", h.tusHead)
		r.Patch("/uploads/tus/{id}", h.tusPatch)
		r.Delete("/uploads/tus/{id}", h.tusDelete)
	})
}

func (h Handler) tusOptions(w http.ResponseWriter, r *http.Request) {
	h.setTusHeaders(w)
	w.Header().Set("Tus-Version", tusVersion)
	w.Header().Set("Tus-Extension", "creation,termination")
	w.Header().Set("Tus-Max-Size", "0")
	w.WriteHeader(http.StatusNoContent)
}

func (h Handler) tusCreate(w http.ResponseWriter, r *http.Request) {
	if !h.validateTusVersion(w, r) {
		return
	}
	user, ok := middleware.CurrentUser(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "authentication required"})
		return
	}
	if !h.enforceRateLimitWithSubject(w, r, "tus_create", user.ID, h.Cfg.TusCreateRatePerMin) {
		return
	}

	uploadLengthHeader := strings.TrimSpace(r.Header.Get("Upload-Length"))
	if uploadLengthHeader == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Upload-Length is required"})
		return
	}
	uploadLength, err := strconv.ParseInt(uploadLengthHeader, 10, 64)
	if err != nil || uploadLength <= 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid Upload-Length"})
		return
	}

	uploadCfg, err := settings.GetUploadSettings(r.Context(), h.DB)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not load upload settings"})
		return
	}
	if uploadCfg.Mode == "custom" && uploadCfg.MaxFileSizeBytes != nil && uploadLength > *uploadCfg.MaxFileSizeBytes {
		writeJSON(w, http.StatusRequestEntityTooLarge, map[string]string{"error": "This file exceeds the maximum allowed upload size."})
		return
	}
	allowed, err := h.canUserStoreBytes(r.Context(), user.ID, uploadLength)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not validate user storage quota"})
		return
	}
	if !allowed {
		writeJSON(w, http.StatusRequestEntityTooLarge, map[string]string{"error": "upload exceeds your storage quota"})
		return
	}

	meta := parseTusMetadata(r.Header.Get("Upload-Metadata"))
	filename := strings.TrimSpace(meta["filename"])
	if filename == "" {
		filename = strings.TrimSpace(meta["name"])
	}
	filename = normalizeNodeName(filepath.Base(filename))
	if filename == "" {
		filename = "upload-" + uuid.NewString()
	}

	var folderID *string
	if rawFolder, ok := meta["folderid"]; ok && strings.TrimSpace(rawFolder) != "" {
		f := strings.TrimSpace(rawFolder)
		if _, err := uuid.Parse(f); err == nil {
			folderID = &f
		}
	}
	if err := h.validateParentOwnership(r, user.ID, folderID); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	uploadID := uuid.NewString()
	tempKey := fmt.Sprintf("tmp/uploads/%s/%s.part", user.ID, uploadID)
	now := time.Now().UTC()
	expiresAt := now.Add(7 * 24 * time.Hour)

	_, err = h.DB.Exec(r.Context(), `
		INSERT INTO upload_sessions (
			id, owner_id, folder_id, file_id, upload_protocol, original_name, target_name,
			total_size_bytes, uploaded_bytes, storage_key_temp, status, error_message, expires_at, created_at, updated_at
		)
		VALUES ($1,$2,$3,NULL,'tus',$4,$5,$6,0,$7,'initialized',NULL,$8,$9,$10)
	`, uploadID, user.ID, folderID, filename, filename, uploadLength, tempKey, expiresAt, now, now)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not initialize tus upload session"})
		return
	}

	h.setTusHeaders(w)
	w.Header().Set("Location", "/api/uploads/tus/"+uploadID)
	w.Header().Set("Upload-Offset", "0")
	w.Header().Set("Upload-Length", strconv.FormatInt(uploadLength, 10))
	w.WriteHeader(http.StatusCreated)
}

func (h Handler) tusHead(w http.ResponseWriter, r *http.Request) {
	if !h.validateTusVersion(w, r) {
		return
	}
	user, ok := middleware.CurrentUser(r.Context())
	if !ok {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	uploadID := chi.URLParam(r, "id")
	if _, err := uuid.Parse(uploadID); err != nil {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	session, err := getUploadSession(r, h.DB, user.ID, uploadID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	h.setTusHeaders(w)
	w.Header().Set("Upload-Offset", strconv.FormatInt(session.UploadedBytes, 10))
	if session.TotalSize != nil {
		w.Header().Set("Upload-Length", strconv.FormatInt(*session.TotalSize, 10))
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h Handler) tusPatch(w http.ResponseWriter, r *http.Request) {
	if !h.validateTusVersion(w, r) {
		return
	}
	if contentType := strings.TrimSpace(r.Header.Get("Content-Type")); !strings.HasPrefix(contentType, "application/offset+octet-stream") {
		writeJSON(w, http.StatusUnsupportedMediaType, map[string]string{"error": "Content-Type must be application/offset+octet-stream"})
		return
	}

	user, ok := middleware.CurrentUser(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "authentication required"})
		return
	}

	uploadID := chi.URLParam(r, "id")
	if _, err := uuid.Parse(uploadID); err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "upload session not found"})
		return
	}

	offset, err := strconv.ParseInt(strings.TrimSpace(r.Header.Get("Upload-Offset")), 10, 64)
	if err != nil || offset < 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid Upload-Offset"})
		return
	}

	tx, err := h.DB.Begin(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not begin tus transaction"})
		return
	}
	defer tx.Rollback(r.Context())

	session, err := lockUploadSession(r, tx, user.ID, uploadID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "upload session not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not load upload session"})
		return
	}
	if session.TotalSize == nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing total size"})
		return
	}
	if session.Status == "completed" {
		h.setTusHeaders(w)
		w.Header().Set("Upload-Offset", strconv.FormatInt(session.UploadedBytes, 10))
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if session.Status == "canceled" || session.Status == "failed" || session.Status == "expired" {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "upload session is not writable"})
		return
	}
	if offset != session.UploadedBytes {
		h.setTusHeaders(w)
		w.Header().Set("Upload-Offset", strconv.FormatInt(session.UploadedBytes, 10))
		w.WriteHeader(http.StatusConflict)
		return
	}

	remaining := *session.TotalSize - session.UploadedBytes
	if remaining < 0 {
		remaining = 0
	}
	written, err := h.Storage.AppendAt(r.Context(), session.TempKey, session.UploadedBytes, io.LimitReader(r.Body, remaining+1))
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not append upload chunk"})
		return
	}

	newOffset := session.UploadedBytes + written
	if newOffset > *session.TotalSize {
		_, _ = tx.Exec(r.Context(), `UPDATE upload_sessions SET status='failed', error_message='uploaded data exceeds declared total size', updated_at=now() WHERE id=$1`, uploadID)
		writeJSON(w, http.StatusConflict, map[string]string{"error": "uploaded data exceeds declared total size"})
		return
	}

	_, err = tx.Exec(r.Context(), `UPDATE upload_sessions SET uploaded_bytes=$1, status='uploading', updated_at=now() WHERE id=$2`, newOffset, uploadID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not update upload session"})
		return
	}

	if newOffset == *session.TotalSize {
		session.UploadedBytes = newOffset
		if err := h.finalizeTusSessionTx(r, tx, session); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not finalize tus upload"})
			return
		}
	}

	if err := tx.Commit(r.Context()); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not commit tus upload chunk"})
		return
	}

	h.setTusHeaders(w)
	w.Header().Set("Upload-Offset", strconv.FormatInt(newOffset, 10))
	w.WriteHeader(http.StatusNoContent)
}

func (h Handler) tusDelete(w http.ResponseWriter, r *http.Request) {
	if !h.validateTusVersion(w, r) {
		return
	}
	user, ok := middleware.CurrentUser(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "authentication required"})
		return
	}

	uploadID := chi.URLParam(r, "id")
	if _, err := uuid.Parse(uploadID); err != nil {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	session, err := getUploadSession(r, h.DB, user.ID, uploadID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not load upload session"})
		return
	}

	_, _ = h.DB.Exec(r.Context(), `UPDATE upload_sessions SET status='canceled', updated_at=now() WHERE id=$1 AND owner_id=$2`, uploadID, user.ID)
	_ = h.Storage.Delete(r.Context(), session.TempKey)

	h.setTusHeaders(w)
	w.WriteHeader(http.StatusNoContent)
}

func (h Handler) finalizeTusSessionTx(r *http.Request, tx pgx.Tx, session uploadSessionRecord) error {
	finalKey := fmt.Sprintf("files/%s/%s/%s.bin", session.OwnerID, time.Now().UTC().Format("2006/01/02"), uuid.NewString())
	if err := h.Storage.Move(r.Context(), session.TempKey, finalKey); err != nil {
		return err
	}

	fileID := uuid.NewString()
	now := time.Now().UTC()
	_, err := tx.Exec(r.Context(), `
		INSERT INTO files (id, owner_id, folder_id, name, original_name, storage_key, size_bytes, mime_type, extension, checksum_sha256, status, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,NULL,NULL,NULL,'ready',$8,$9)
	`, fileID, session.OwnerID, session.FolderID, session.TargetName, session.OriginalName, finalKey, session.UploadedBytes, now, now)
	if err != nil {
		_ = h.Storage.Move(r.Context(), finalKey, session.TempKey)
		return err
	}

	_, err = tx.Exec(r.Context(), `UPDATE users SET used_storage_bytes = used_storage_bytes + $1, updated_at=now() WHERE id=$2`, session.UploadedBytes, session.OwnerID)
	if err != nil {
		_ = h.Storage.Move(r.Context(), finalKey, session.TempKey)
		return err
	}
	if err := h.ensureQuotaAllowsSizeTx(r.Context(), tx, session.OwnerID, session.UploadedBytes); err != nil {
		_ = h.Storage.Move(r.Context(), finalKey, session.TempKey)
		return err
	}

	_, err = tx.Exec(r.Context(), `UPDATE upload_sessions SET status='completed', file_id=$1, updated_at=now() WHERE id=$2`, fileID, session.ID)
	if err != nil {
		_ = h.Storage.Move(r.Context(), finalKey, session.TempKey)
		return err
	}

	h.insertAudit(r.Context(), &session.OwnerID, "upload.completed", "upload_session", &session.ID, clientIP(r), r.UserAgent(), map[string]any{"fileId": fileID, "protocol": "tus"})
	observability.AddUploadedBytes(session.UploadedBytes)
	return nil
}

func (h Handler) setTusHeaders(w http.ResponseWriter) {
	w.Header().Set("Tus-Resumable", tusVersion)
	w.Header().Set("Cache-Control", "no-store")
}

func (h Handler) validateTusVersion(w http.ResponseWriter, r *http.Request) bool {
	version := strings.TrimSpace(r.Header.Get("Tus-Resumable"))
	if version == "" {
		version = tusVersion
	}
	if version != tusVersion {
		w.Header().Set("Tus-Version", tusVersion)
		w.WriteHeader(http.StatusPreconditionFailed)
		return false
	}
	return true
}

func parseTusMetadata(raw string) map[string]string {
	result := make(map[string]string)
	for _, part := range strings.Split(raw, ",") {
		segment := strings.TrimSpace(part)
		if segment == "" {
			continue
		}
		pieces := strings.SplitN(segment, " ", 2)
		if len(pieces) != 2 {
			continue
		}
		key := strings.ToLower(strings.TrimSpace(pieces[0]))
		valueB64 := strings.TrimSpace(pieces[1])
		decoded, err := base64.StdEncoding.DecodeString(valueB64)
		if err != nil {
			decoded, err = base64.RawStdEncoding.DecodeString(valueB64)
			if err != nil {
				continue
			}
		}
		result[key] = string(decoded)
	}
	return result
}
