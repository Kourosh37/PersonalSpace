package httpapi

import (
	"context"
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
	"github.com/jackc/pgx/v5/pgxpool"
)

type uploadSessionRecord struct {
	ID            string
	OwnerID       string
	FolderID      *string
	FileID        *string
	Protocol      string
	OriginalName  string
	TargetName    string
	TotalSize     *int64
	UploadedBytes int64
	TempKey       string
	Status        string
	ErrorMessage  *string
}

func (h Handler) registerUploadRoutes(api chi.Router, authMW middleware.AuthMiddleware) {
	api.With(authMW.RequireAuth).Post("/uploads/init", h.uploadInit)
	api.With(authMW.RequireAuth).Patch("/uploads/{id}/chunk", h.uploadChunk)
	api.With(authMW.RequireAuth).Get("/uploads/{id}/status", h.uploadStatus)
	api.With(authMW.RequireAuth).Post("/uploads/{id}/complete", h.uploadComplete)
	api.With(authMW.RequireAuth).Delete("/uploads/{id}/cancel", h.uploadCancel)
}

type uploadInitRequest struct {
	FolderID       *string `json:"folderId"`
	OriginalName   string  `json:"originalName"`
	TargetName     string  `json:"targetName"`
	TotalSizeBytes *int64  `json:"totalSizeBytes"`
}

func (h Handler) uploadInit(w http.ResponseWriter, r *http.Request) {
	user, ok := middleware.CurrentUser(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "authentication required"})
		return
	}
	if !h.enforceRateLimitWithSubject(w, r, "upload_init", user.ID, h.Cfg.UploadInitRatePerMin) {
		return
	}

	var req uploadInitRequest
	if err := ReadJSON(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	folderID, err := optionalUUIDPtr(req.FolderID)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if err := h.validateParentOwnership(r, user.ID, folderID); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	originalName := strings.TrimSpace(req.OriginalName)
	if originalName == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "originalName is required"})
		return
	}

	targetName := normalizeNodeName(req.TargetName)
	if targetName == "" {
		targetName = normalizeNodeName(filepath.Base(originalName))
	}
	if targetName == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid file name"})
		return
	}

	if req.TotalSizeBytes != nil && *req.TotalSizeBytes <= 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "totalSizeBytes must be greater than zero when provided"})
		return
	}

	uploadCfg, err := settings.GetUploadSettings(r.Context(), h.DB)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not load upload settings"})
		return
	}
	if uploadCfg.Mode == "custom" && uploadCfg.MaxFileSizeBytes != nil && req.TotalSizeBytes != nil && *req.TotalSizeBytes > *uploadCfg.MaxFileSizeBytes {
		writeJSON(w, http.StatusRequestEntityTooLarge, map[string]string{"error": "This file exceeds the maximum allowed upload size."})
		return
	}
	if req.TotalSizeBytes != nil {
		allowed, err := h.canUserStoreBytes(r.Context(), user.ID, *req.TotalSizeBytes)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not validate user storage quota"})
			return
		}
		if !allowed {
			writeJSON(w, http.StatusRequestEntityTooLarge, map[string]string{"error": "upload exceeds your storage quota"})
			return
		}
	}

	uploadID := uuid.NewString()
	tempKey := fmt.Sprintf("tmp/uploads/%s/%s.part", user.ID, uploadID)
	expiresAt := time.Now().UTC().Add(7 * 24 * time.Hour)
	now := time.Now().UTC()
	_, err = h.DB.Exec(r.Context(), `
		INSERT INTO upload_sessions (
			id, owner_id, folder_id, file_id, upload_protocol, original_name, target_name, total_size_bytes,
			uploaded_bytes, storage_key_temp, status, error_message, expires_at, created_at, updated_at
		)
		VALUES ($1,$2,$3,NULL,'custom',$4,$5,$6,0,$7,'initialized',NULL,$8,$9,$10)
	`, uploadID, user.ID, folderID, originalName, targetName, req.TotalSizeBytes, tempKey, expiresAt, now, now)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not initialize upload session"})
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{
		"id":             uploadID,
		"folderId":       folderID,
		"originalName":   originalName,
		"targetName":     targetName,
		"totalSizeBytes": req.TotalSizeBytes,
		"uploadedBytes":  0,
		"status":         "initialized",
	})
}

func (h Handler) uploadChunk(w http.ResponseWriter, r *http.Request) {
	user, ok := middleware.CurrentUser(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "authentication required"})
		return
	}

	uploadID := chi.URLParam(r, "id")
	if _, err := uuid.Parse(uploadID); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid upload id"})
		return
	}

	uploadCfg, err := settings.GetUploadSettings(r.Context(), h.DB)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not load upload settings"})
		return
	}

	tx, err := h.DB.Begin(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not begin upload chunk transaction"})
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

	if session.Status == "completed" || session.Status == "canceled" || session.Status == "failed" || session.Status == "expired" {
		writeJSON(w, http.StatusConflict, map[string]any{"error": "upload session is not writable", "status": session.Status})
		return
	}

	offsetHeader := strings.TrimSpace(r.Header.Get("Upload-Offset"))
	if offsetHeader != "" {
		offset, err := strconv.ParseInt(offsetHeader, 10, 64)
		if err != nil || offset < 0 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid Upload-Offset"})
			return
		}
		if offset != session.UploadedBytes {
			writeJSON(w, http.StatusConflict, map[string]any{"error": "offset mismatch", "expectedOffset": session.UploadedBytes})
			return
		}
	}

	reader := io.Reader(r.Body)
	maxAllowed := int64(-1)
	if uploadCfg.Mode == "custom" && uploadCfg.MaxFileSizeBytes != nil {
		maxAllowed = *uploadCfg.MaxFileSizeBytes
	}
	if maxAllowed > -1 {
		remaining := maxAllowed - session.UploadedBytes
		if remaining < 0 {
			_, _ = tx.Exec(r.Context(), `UPDATE upload_sessions SET status='failed', error_message=$1, updated_at=now() WHERE id=$2`, "file exceeds max upload size", uploadID)
			writeJSON(w, http.StatusRequestEntityTooLarge, map[string]string{"error": "This file exceeds the maximum allowed upload size."})
			return
		}
		reader = io.LimitReader(r.Body, remaining+1)
	}

	written, err := h.Storage.AppendAt(r.Context(), session.TempKey, session.UploadedBytes, reader)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "offset mismatch") {
			writeJSON(w, http.StatusConflict, map[string]any{"error": "offset mismatch", "expectedOffset": session.UploadedBytes})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not write upload chunk"})
		return
	}

	newOffset := session.UploadedBytes + written
	if maxAllowed > -1 && newOffset > maxAllowed {
		_, _ = tx.Exec(r.Context(), `UPDATE upload_sessions SET status='failed', error_message=$1, updated_at=now() WHERE id=$2`, "file exceeds max upload size", uploadID)
		writeJSON(w, http.StatusRequestEntityTooLarge, map[string]string{"error": "This file exceeds the maximum allowed upload size."})
		return
	}
	if session.TotalSize != nil && newOffset > *session.TotalSize {
		_, _ = tx.Exec(r.Context(), `UPDATE upload_sessions SET status='failed', error_message=$1, updated_at=now() WHERE id=$2`, "uploaded data exceeds declared size", uploadID)
		writeJSON(w, http.StatusConflict, map[string]string{"error": "uploaded bytes exceed declared total size"})
		return
	}

	status := "uploading"
	if session.TotalSize != nil && newOffset == *session.TotalSize {
		status = "uploading"
	}

	_, err = tx.Exec(r.Context(), `
		UPDATE upload_sessions
		SET uploaded_bytes=$1, status=$2, error_message=NULL, updated_at=now()
		WHERE id=$3
	`, newOffset, status, uploadID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not update upload session"})
		return
	}

	if err := tx.Commit(r.Context()); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not commit upload chunk"})
		return
	}

	w.Header().Set("Upload-Offset", strconv.FormatInt(newOffset, 10))
	writeJSON(w, http.StatusOK, map[string]any{"id": uploadID, "uploadedBytes": newOffset, "totalSizeBytes": session.TotalSize, "status": status})
}

func (h Handler) uploadStatus(w http.ResponseWriter, r *http.Request) {
	user, ok := middleware.CurrentUser(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "authentication required"})
		return
	}

	uploadID := chi.URLParam(r, "id")
	if _, err := uuid.Parse(uploadID); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid upload id"})
		return
	}

	session, err := getUploadSession(r, h.DB, user.ID, uploadID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "upload session not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not read upload session"})
		return
	}

	w.Header().Set("Upload-Offset", strconv.FormatInt(session.UploadedBytes, 10))
	writeJSON(w, http.StatusOK, map[string]any{
		"id":             session.ID,
		"status":         session.Status,
		"uploadedBytes":  session.UploadedBytes,
		"totalSizeBytes": session.TotalSize,
		"targetName":     session.TargetName,
		"originalName":   session.OriginalName,
	})
}

func (h Handler) uploadComplete(w http.ResponseWriter, r *http.Request) {
	user, ok := middleware.CurrentUser(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "authentication required"})
		return
	}
	if !h.enforceRateLimitWithSubject(w, r, "upload_complete", user.ID, h.Cfg.UploadCompleteRatePerMin) {
		return
	}

	uploadID := chi.URLParam(r, "id")
	if _, err := uuid.Parse(uploadID); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid upload id"})
		return
	}

	tx, err := h.DB.Begin(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not begin complete transaction"})
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

	if session.Status == "completed" {
		writeJSON(w, http.StatusOK, map[string]any{"id": uploadID, "status": "completed", "fileId": session.FileID})
		return
	}
	if session.Status == "canceled" || session.Status == "failed" || session.Status == "expired" {
		writeJSON(w, http.StatusConflict, map[string]any{"error": "upload session is not completable", "status": session.Status})
		return
	}

	if session.TotalSize != nil && session.UploadedBytes != *session.TotalSize {
		writeJSON(w, http.StatusConflict, map[string]any{"error": "upload is incomplete", "uploadedBytes": session.UploadedBytes, "totalSizeBytes": session.TotalSize})
		return
	}

	finalKey := fmt.Sprintf("files/%s/%s/%s.bin", user.ID, time.Now().UTC().Format("2006/01/02"), uuid.NewString())
	if err := h.Storage.Move(r.Context(), session.TempKey, finalKey); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not finalize upload file"})
		return
	}

	fileID := uuid.NewString()
	now := time.Now().UTC()
	_, err = tx.Exec(r.Context(), `
		INSERT INTO files (id, owner_id, folder_id, name, original_name, storage_key, size_bytes, mime_type, extension, checksum_sha256, status, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,NULL,NULL,NULL,'ready',$8,$9)
	`, fileID, user.ID, session.FolderID, session.TargetName, session.OriginalName, finalKey, session.UploadedBytes, now, now)
	if err != nil {
		_ = h.Storage.Move(r.Context(), finalKey, session.TempKey)
		if isUniqueViolation(err) {
			writeJSON(w, http.StatusConflict, map[string]string{"error": "an item with this name already exists in this folder"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not create file metadata"})
		return
	}

	_, err = tx.Exec(r.Context(), `UPDATE users SET used_storage_bytes = used_storage_bytes + $1, updated_at = now() WHERE id=$2`, session.UploadedBytes, user.ID)
	if err != nil {
		_ = h.Storage.Move(r.Context(), finalKey, session.TempKey)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not update user storage usage"})
		return
	}
	if err := h.ensureQuotaAllowsSizeTx(r.Context(), tx, user.ID, session.UploadedBytes); err != nil {
		_ = h.Storage.Move(r.Context(), finalKey, session.TempKey)
		writeJSON(w, http.StatusRequestEntityTooLarge, map[string]string{"error": "upload exceeds your storage quota"})
		return
	}

	_, err = tx.Exec(r.Context(), `
		UPDATE upload_sessions
		SET status='completed', file_id=$1, updated_at=now()
		WHERE id=$2
	`, fileID, uploadID)
	if err != nil {
		_ = h.Storage.Move(r.Context(), finalKey, session.TempKey)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not complete upload session"})
		return
	}

	if err := tx.Commit(r.Context()); err != nil {
		_ = h.Storage.Move(r.Context(), finalKey, session.TempKey)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not commit upload completion"})
		return
	}

	h.insertAudit(r.Context(), &user.ID, "upload.completed", "upload_session", &uploadID, clientIP(r), r.UserAgent(), map[string]any{"fileId": fileID, "size": session.UploadedBytes})
	observability.AddUploadedBytes(session.UploadedBytes)
	writeJSON(w, http.StatusOK, map[string]any{"id": uploadID, "status": "completed", "fileId": fileID})
}

func (h Handler) uploadCancel(w http.ResponseWriter, r *http.Request) {
	user, ok := middleware.CurrentUser(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "authentication required"})
		return
	}

	uploadID := chi.URLParam(r, "id")
	if _, err := uuid.Parse(uploadID); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid upload id"})
		return
	}

	session, err := getUploadSession(r, h.DB, user.ID, uploadID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "upload session not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not load upload session"})
		return
	}

	_, err = h.DB.Exec(r.Context(), `UPDATE upload_sessions SET status='canceled', updated_at=now() WHERE id=$1 AND owner_id=$2`, uploadID, user.ID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not cancel upload session"})
		return
	}

	_ = h.Storage.Delete(r.Context(), session.TempKey)
	h.insertAudit(r.Context(), &user.ID, "upload.canceled", "upload_session", &uploadID, clientIP(r), r.UserAgent(), map[string]any{})
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func getUploadSession(r *http.Request, db *pgxpool.Pool, ownerID string, uploadID string) (uploadSessionRecord, error) {
	var rec uploadSessionRecord
	err := db.QueryRow(r.Context(), `
		SELECT id, owner_id, folder_id, file_id, upload_protocol, original_name, target_name, total_size_bytes, uploaded_bytes, storage_key_temp, status, error_message
		FROM upload_sessions
		WHERE id=$1 AND owner_id=$2
	`, uploadID, ownerID).Scan(
		&rec.ID,
		&rec.OwnerID,
		&rec.FolderID,
		&rec.FileID,
		&rec.Protocol,
		&rec.OriginalName,
		&rec.TargetName,
		&rec.TotalSize,
		&rec.UploadedBytes,
		&rec.TempKey,
		&rec.Status,
		&rec.ErrorMessage,
	)
	if err != nil {
		return uploadSessionRecord{}, err
	}
	return rec, nil
}

func lockUploadSession(r *http.Request, tx pgx.Tx, ownerID string, uploadID string) (uploadSessionRecord, error) {
	var rec uploadSessionRecord
	err := tx.QueryRow(r.Context(), `
		SELECT id, owner_id, folder_id, file_id, upload_protocol, original_name, target_name, total_size_bytes, uploaded_bytes, storage_key_temp, status, error_message
		FROM upload_sessions
		WHERE id=$1 AND owner_id=$2
		FOR UPDATE
	`, uploadID, ownerID).Scan(
		&rec.ID,
		&rec.OwnerID,
		&rec.FolderID,
		&rec.FileID,
		&rec.Protocol,
		&rec.OriginalName,
		&rec.TargetName,
		&rec.TotalSize,
		&rec.UploadedBytes,
		&rec.TempKey,
		&rec.Status,
		&rec.ErrorMessage,
	)
	if err != nil {
		return uploadSessionRecord{}, err
	}
	return rec, nil
}

func (h Handler) canUserStoreBytes(ctx context.Context, userID string, additionalBytes int64) (bool, error) {
	var quota *int64
	var used int64
	if err := h.DB.QueryRow(ctx, `SELECT storage_quota_bytes, used_storage_bytes FROM users WHERE id=$1`, userID).Scan(&quota, &used); err != nil {
		return false, err
	}
	if quota == nil {
		return true, nil
	}
	return used+additionalBytes <= *quota, nil
}

func (h Handler) ensureQuotaAllowsSizeTx(ctx context.Context, tx pgx.Tx, userID string, additionalBytes int64) error {
	var quota *int64
	var used int64
	if err := tx.QueryRow(ctx, `SELECT storage_quota_bytes, used_storage_bytes FROM users WHERE id=$1 FOR UPDATE`, userID).Scan(&quota, &used); err != nil {
		return err
	}
	if quota != nil && used > *quota {
		return errors.New("quota exceeded")
	}
	return nil
}
