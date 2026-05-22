package httpapi

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"space/backend/internal/middleware"
	"space/backend/internal/settings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

var errUploadExceeded = errors.New("file exceeds the maximum allowed upload size")

type uploadedFileResult struct {
	ID           string `json:"id,omitempty"`
	Name         string `json:"name,omitempty"`
	OriginalName string `json:"originalName,omitempty"`
	SizeBytes    int64  `json:"sizeBytes,omitempty"`
	Error        string `json:"error,omitempty"`
}

func (h Handler) registerFileRoutes(api chi.Router, authMW middleware.AuthMiddleware) {
	api.With(authMW.RequireAuth).Post("/files/upload", h.uploadFiles)
	api.With(authMW.RequireAuth).Get("/files/{id}", h.getFileMetadata)
	api.With(authMW.RequireAuth).Get("/files/{id}/metadata", h.getFileMetadata)
	api.With(authMW.RequireAuth).Get("/files/{id}/preview-info", h.getFilePreviewInfo)
	api.With(authMW.RequireAuth).Get("/files/{id}/preview-content", h.getFilePreviewContent)
	api.With(authMW.RequireAuth).Post("/files/{id}/preview-jobs", h.createFilePreviewJob)
	api.With(authMW.RequireAuth).Get("/files/{id}/preview-jobs", h.listFilePreviewJobs)
	api.With(authMW.RequireAuth).Get("/files/{id}/download", h.downloadFile)
	api.With(authMW.RequireAuth).Get("/files/{id}/preview", h.previewFile)
	api.With(authMW.RequireAuth).Patch("/files/{id}", h.renameFile)
	api.With(authMW.RequireAuth).Delete("/files/{id}", h.deleteFile)
	api.With(authMW.RequireAuth).Post("/files/{id}/move", h.moveFile)
}

func (h Handler) uploadFiles(w http.ResponseWriter, r *http.Request) {
	user, ok := middleware.CurrentUser(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "authentication required"})
		return
	}

	folderID, err := optionalUUIDFromQuery(r, "folderId")
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if err := h.validateParentOwnership(r, user.ID, folderID); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	uploadCfg, err := settings.GetUploadSettings(r.Context(), h.DB)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not read upload settings"})
		return
	}

	mr, err := r.MultipartReader()
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "expected multipart/form-data body"})
		return
	}

	results := make([]uploadedFileResult, 0, 8)
	created := 0

	for {
		part, err := mr.NextPart()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid multipart data"})
			return
		}

		if part.FormName() != "file" {
			_ = part.Close()
			continue
		}

		originalName := strings.TrimSpace(part.FileName())
		if originalName == "" {
			_ = part.Close()
			continue
		}

		targetName := normalizeNodeName(filepath.Base(originalName))
		if targetName == "" {
			_ = part.Close()
			results = append(results, uploadedFileResult{OriginalName: originalName, Error: "invalid file name"})
			continue
		}

		fileID := uuid.NewString()
		tmpKey := fmt.Sprintf("tmp/uploads/%s/%s.part", user.ID, fileID)
		finalKey := fmt.Sprintf("files/%s/%s/%s.bin", user.ID, time.Now().UTC().Format("2006/01/02"), fileID)

		size, checksum, mimeType, ext, copyErr := h.copyPartToStorage(r.Context(), part, tmpKey, uploadCfg.MaxFileSizeBytes)
		_ = part.Close()
		if copyErr != nil {
			_ = h.Storage.Delete(r.Context(), tmpKey)
			status := http.StatusBadRequest
			message := copyErr.Error()
			if errors.Is(copyErr, errUploadExceeded) {
				status = http.StatusRequestEntityTooLarge
				message = "This file exceeds the maximum allowed upload size."
			}
			results = append(results, uploadedFileResult{OriginalName: originalName, Error: message})
			if status == http.StatusRequestEntityTooLarge {
				writeJSON(w, status, map[string]any{"results": results})
				return
			}
			continue
		}

		if err := h.Storage.Move(r.Context(), tmpKey, finalKey); err != nil {
			_ = h.Storage.Delete(r.Context(), tmpKey)
			results = append(results, uploadedFileResult{OriginalName: originalName, Error: "could not finalize uploaded file"})
			continue
		}

		now := time.Now().UTC()
		tx, err := h.DB.Begin(r.Context())
		if err != nil {
			_ = h.Storage.Delete(r.Context(), finalKey)
			results = append(results, uploadedFileResult{OriginalName: originalName, Error: "could not persist file metadata"})
			continue
		}

		_, err = tx.Exec(r.Context(), `
			INSERT INTO files (id, owner_id, folder_id, name, original_name, storage_key, size_bytes, mime_type, extension, checksum_sha256, status, created_at, updated_at)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,'ready',$11,$12)
		`, fileID, user.ID, folderID, targetName, originalName, finalKey, size, mimeTypeOrNil(mimeType), extOrNil(ext), checksum, now, now)
		if err != nil {
			tx.Rollback(r.Context())
			_ = h.Storage.Delete(r.Context(), finalKey)
			if isUniqueViolation(err) {
				results = append(results, uploadedFileResult{OriginalName: originalName, Error: "an item with this name already exists in this folder"})
				continue
			}
			results = append(results, uploadedFileResult{OriginalName: originalName, Error: "could not persist file metadata"})
			continue
		}

		_, err = tx.Exec(r.Context(), `UPDATE users SET used_storage_bytes = used_storage_bytes + $1, updated_at = now() WHERE id=$2`, size, user.ID)
		if err != nil {
			tx.Rollback(r.Context())
			_ = h.Storage.Delete(r.Context(), finalKey)
			results = append(results, uploadedFileResult{OriginalName: originalName, Error: "could not update user usage"})
			continue
		}

		if err := tx.Commit(r.Context()); err != nil {
			_ = h.Storage.Delete(r.Context(), finalKey)
			results = append(results, uploadedFileResult{OriginalName: originalName, Error: "could not commit upload"})
			continue
		}

		created++
		h.insertAudit(r.Context(), &user.ID, "file.uploaded", "file", &fileID, clientIP(r), r.UserAgent(), map[string]any{"name": targetName, "size": size})
		results = append(results, uploadedFileResult{ID: fileID, Name: targetName, OriginalName: originalName, SizeBytes: size})
	}

	status := http.StatusCreated
	if created == 0 {
		status = http.StatusBadRequest
	}
	writeJSON(w, status, map[string]any{"results": results})
}

func (h Handler) copyPartToStorage(ctx context.Context, part io.Reader, storageKey string, maxBytes *int64) (size int64, checksum string, detectedMIME string, extension string, err error) {
	hasher := sha256.New()
	buffer := make([]byte, 32*1024)
	sniff := make([]byte, 0, 512)

	pr, pw := io.Pipe()
	copyErrCh := make(chan error, 1)

	go func() {
		defer pw.Close()
		for {
			n, readErr := part.Read(buffer)
			if n > 0 {
				chunk := buffer[:n]
				if maxBytes != nil && size+int64(n) > *maxBytes {
					copyErrCh <- errUploadExceeded
					return
				}
				size += int64(n)
				if len(sniff) < 512 {
					need := 512 - len(sniff)
					if need > n {
						need = n
					}
					sniff = append(sniff, chunk[:need]...)
				}
				if _, err := hasher.Write(chunk); err != nil {
					copyErrCh <- err
					return
				}
				if _, err := pw.Write(chunk); err != nil {
					copyErrCh <- err
					return
				}
			}

			if readErr != nil {
				if errors.Is(readErr, io.EOF) {
					copyErrCh <- nil
					return
				}
				copyErrCh <- readErr
				return
			}
		}
	}()

	storeErr := h.Storage.PutStream(ctx, storageKey, pr)
	streamErr := <-copyErrCh

	if streamErr != nil {
		_ = pr.CloseWithError(streamErr)
		return 0, "", "", "", streamErr
	}
	if storeErr != nil {
		return 0, "", "", "", storeErr
	}

	detectedMIME = http.DetectContentType(sniff)
	checksum = hex.EncodeToString(hasher.Sum(nil))
	if exts, _ := mime.ExtensionsByType(detectedMIME); len(exts) > 0 {
		extension = strings.TrimPrefix(exts[0], ".")
	}
	return size, checksum, detectedMIME, extension, nil
}

func mimeTypeOrNil(v string) any {
	if strings.TrimSpace(v) == "" {
		return nil
	}
	return v
}

func extOrNil(v string) any {
	if strings.TrimSpace(v) == "" {
		return nil
	}
	return v
}

type fileRecord struct {
	ID           string
	OwnerID      string
	FolderID     *string
	Name         string
	OriginalName string
	StorageKey   string
	SizeBytes    int64
	MimeType     *string
	Extension    *string
	UpdatedAt    time.Time
}

type filePreviewRecord struct {
	ID         string
	Type       string
	StorageKey *string
	MimeType   *string
	SizeBytes  *int64
	Status     string
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

func (h Handler) fetchOwnedFile(ctx context.Context, userID string, fileID string) (fileRecord, error) {
	var rec fileRecord
	err := h.DB.QueryRow(ctx, `
		SELECT id, owner_id, folder_id, name, original_name, storage_key, size_bytes, mime_type, extension, updated_at
		FROM files
		WHERE id=$1 AND owner_id=$2 AND deleted_at IS NULL
	`, fileID, userID).Scan(&rec.ID, &rec.OwnerID, &rec.FolderID, &rec.Name, &rec.OriginalName, &rec.StorageKey, &rec.SizeBytes, &rec.MimeType, &rec.Extension, &rec.UpdatedAt)
	if err != nil {
		return fileRecord{}, err
	}
	return rec, nil
}

func (h Handler) listFilePreviews(ctx context.Context, fileID string) ([]filePreviewRecord, error) {
	rows, err := h.DB.Query(ctx, `
		SELECT id, preview_type, storage_key, mime_type, size_bytes, status, created_at, updated_at
		FROM file_previews
		WHERE file_id=$1
		ORDER BY created_at DESC
	`, fileID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]filePreviewRecord, 0, 8)
	for rows.Next() {
		var rec filePreviewRecord
		if err := rows.Scan(&rec.ID, &rec.Type, &rec.StorageKey, &rec.MimeType, &rec.SizeBytes, &rec.Status, &rec.CreatedAt, &rec.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, rec)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (h Handler) getReadyFilePreview(ctx context.Context, fileID string, previewType string) (filePreviewRecord, error) {
	var rec filePreviewRecord
	err := h.DB.QueryRow(ctx, `
		SELECT id, preview_type, storage_key, mime_type, size_bytes, status, created_at, updated_at
		FROM file_previews
		WHERE file_id=$1
		  AND preview_type=$2
		  AND status='ready'
		  AND storage_key IS NOT NULL
		ORDER BY updated_at DESC
		LIMIT 1
	`, fileID, previewType).Scan(&rec.ID, &rec.Type, &rec.StorageKey, &rec.MimeType, &rec.SizeBytes, &rec.Status, &rec.CreatedAt, &rec.UpdatedAt)
	if err != nil {
		return filePreviewRecord{}, err
	}
	return rec, nil
}

func (h Handler) getFileMetadata(w http.ResponseWriter, r *http.Request) {
	user, ok := middleware.CurrentUser(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "authentication required"})
		return
	}

	fileID := chi.URLParam(r, "id")
	if _, err := uuid.Parse(fileID); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid file id"})
		return
	}

	rec, err := h.fetchOwnedFile(r.Context(), user.ID, fileID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "file not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not load file metadata"})
		return
	}
	previewCfg, cfgErr := h.getPreviewRuntimeSettings(r.Context())
	if cfgErr != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not load preview settings"})
		return
	}
	if !previewCfg.Enabled {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "preview generation is disabled by admin settings"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"id":           rec.ID,
		"name":         rec.Name,
		"originalName": rec.OriginalName,
		"folderId":     rec.FolderID,
		"sizeBytes":    rec.SizeBytes,
		"mimeType":     rec.MimeType,
		"extension":    rec.Extension,
		"updatedAt":    rec.UpdatedAt,
	})
}

func (h Handler) getFilePreviewInfo(w http.ResponseWriter, r *http.Request) {
	user, ok := middleware.CurrentUser(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "authentication required"})
		return
	}

	fileID := chi.URLParam(r, "id")
	if _, err := uuid.Parse(fileID); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid file id"})
		return
	}

	rec, err := h.fetchOwnedFile(r.Context(), user.ID, fileID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "file not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not load file metadata"})
		return
	}

	category, method, supported := detectPreviewMode(rec)
	previewCfg, cfgErr := h.getPreviewRuntimeSettings(r.Context())
	if cfgErr != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not load preview settings"})
		return
	}
	previews, previewsErr := h.listFilePreviews(r.Context(), rec.ID)
	if previewsErr != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not load file previews"})
		return
	}
	previewItems := make([]map[string]any, 0, len(previews))
	for _, p := range previews {
		previewItems = append(previewItems, map[string]any{
			"id":         p.ID,
			"type":       p.Type,
			"storageKey": p.StorageKey,
			"mimeType":   p.MimeType,
			"sizeBytes":  p.SizeBytes,
			"status":     p.Status,
			"createdAt":  p.CreatedAt,
			"updatedAt":  p.UpdatedAt,
		})
	}
	if allowed, reason := previewAllowedByConfig(previewCfg, category, false); !allowed {
		supported = false
		method = "disabled"
		if reason == "" {
			reason = "preview is disabled by admin settings"
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"fileId":            rec.ID,
			"name":              rec.Name,
			"mimeType":          rec.MimeType,
			"sizeBytes":         rec.SizeBytes,
			"category":          category,
			"method":            method,
			"supported":         supported,
			"textMaxBytes":      h.previewTextMaxBytes(r),
			"streamPreviewURL":  "/api/files/" + rec.ID + "/preview",
			"textPreviewURL":    "/api/files/" + rec.ID + "/preview-content",
			"thumbnailURL":      "/api/files/" + rec.ID + "/preview?variant=thumbnail",
			"pdfPreviewURL":     "/api/files/" + rec.ID + "/preview?variant=pdf",
			"generatedPreviews": previewItems,
			"reason":            reason,
		})
		return
	}

	textMaxBytes := h.previewTextMaxBytes(r)
	writeJSON(w, http.StatusOK, map[string]any{
		"fileId":            rec.ID,
		"name":              rec.Name,
		"mimeType":          rec.MimeType,
		"sizeBytes":         rec.SizeBytes,
		"category":          category,
		"method":            method,
		"supported":         supported,
		"textMaxBytes":      textMaxBytes,
		"streamPreviewURL":  "/api/files/" + rec.ID + "/preview",
		"textPreviewURL":    "/api/files/" + rec.ID + "/preview-content",
		"thumbnailURL":      "/api/files/" + rec.ID + "/preview?variant=thumbnail",
		"pdfPreviewURL":     "/api/files/" + rec.ID + "/preview?variant=pdf",
		"generatedPreviews": previewItems,
	})
}

func (h Handler) getFilePreviewContent(w http.ResponseWriter, r *http.Request) {
	user, ok := middleware.CurrentUser(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "authentication required"})
		return
	}

	fileID := chi.URLParam(r, "id")
	if _, err := uuid.Parse(fileID); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid file id"})
		return
	}

	rec, err := h.fetchOwnedFile(r.Context(), user.ID, fileID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "file not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not load file metadata"})
		return
	}

	category, method, supported := detectPreviewMode(rec)
	previewCfg, cfgErr := h.getPreviewRuntimeSettings(r.Context())
	if cfgErr != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not load preview settings"})
		return
	}
	if allowed, reason := previewAllowedByConfig(previewCfg, category, false); !allowed {
		if reason == "" {
			reason = "preview is disabled by admin settings"
		}
		writeJSON(w, http.StatusForbidden, map[string]string{"error": reason})
		return
	}
	if !supported || method != "text_partial" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "text preview is not available for this file type"})
		return
	}

	limit := h.previewTextMaxBytes(r)
	if limit <= 0 {
		limit = 1 * 1024 * 1024
	}
	stream, err := h.Storage.GetStream(r.Context(), rec.StorageKey)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "file data not found"})
		return
	}
	defer stream.Close()

	limited := io.LimitReader(stream, limit+1)
	data, err := io.ReadAll(limited)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not read file preview content"})
		return
	}

	truncated := int64(len(data)) > limit
	if truncated {
		data = data[:limit]
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"fileId":      rec.ID,
		"category":    category,
		"content":     string(data),
		"truncated":   truncated,
		"limitBytes":  limit,
		"sizeBytes":   rec.SizeBytes,
		"encoding":    "utf-8",
		"downloadURL": "/api/files/" + rec.ID + "/download",
	})
}

func (h Handler) createFilePreviewJob(w http.ResponseWriter, r *http.Request) {
	user, ok := middleware.CurrentUser(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "authentication required"})
		return
	}

	fileID := chi.URLParam(r, "id")
	if _, err := uuid.Parse(fileID); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid file id"})
		return
	}

	rec, err := h.fetchOwnedFile(r.Context(), user.ID, fileID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "file not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not load file metadata"})
		return
	}
	previewCfg, cfgErr := h.getPreviewRuntimeSettings(r.Context())
	if cfgErr != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not load preview settings"})
		return
	}
	if !previewCfg.Enabled {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "preview generation is disabled by admin settings"})
		return
	}

	type createPreviewJobRequest struct {
		JobType string `json:"jobType"`
	}
	var req createPreviewJobRequest
	if strings.TrimSpace(r.Header.Get("Content-Type")) != "" {
		_ = json.NewDecoder(r.Body).Decode(&req)
	}

	jobType := strings.ToLower(strings.TrimSpace(req.JobType))
	if jobType == "" {
		jobType = "metadata"
	}
	if jobType != "metadata" && jobType != "thumbnail" && jobType != "office_to_pdf" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "unsupported preview job type"})
		return
	}
	category, _, _ := detectPreviewMode(rec)
	if jobType == "thumbnail" && category != "image" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "thumbnail preview is only supported for image files"})
		return
	}
	if jobType == "office_to_pdf" && category != "office" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "office_to_pdf preview is only supported for office document files"})
		return
	}
	if jobType == "office_to_pdf" && !previewCfg.OfficeEnabled {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "office preview is disabled by admin settings"})
		return
	}

	var existingJobID string
	var existingStatus string
	err = h.DB.QueryRow(r.Context(), `
		SELECT id, status
		FROM preview_jobs
		WHERE file_id=$1 AND job_type=$2 AND status IN ('queued','processing')
		ORDER BY created_at DESC
		LIMIT 1
	`, rec.ID, jobType).Scan(&existingJobID, &existingStatus)
	if err == nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"jobId":         existingJobID,
			"status":        existingStatus,
			"alreadyQueued": true,
			"jobType":       jobType,
		})
		return
	}
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not check existing preview jobs"})
		return
	}

	jobID := uuid.NewString()
	_, err = h.DB.Exec(r.Context(), `
		INSERT INTO preview_jobs (id, file_id, job_type, status, output_storage_key, error_message, attempts, created_at, updated_at)
		VALUES ($1,$2,$3,'queued',NULL,NULL,0,now(),now())
	`, jobID, rec.ID, jobType)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not enqueue preview job"})
		return
	}

	h.insertAudit(r.Context(), &user.ID, "preview.job.created", "preview_job", &jobID, clientIP(r), r.UserAgent(), map[string]any{"fileId": rec.ID, "jobType": jobType})
	writeJSON(w, http.StatusAccepted, map[string]any{"jobId": jobID, "status": "queued", "jobType": jobType})
}

func (h Handler) listFilePreviewJobs(w http.ResponseWriter, r *http.Request) {
	user, ok := middleware.CurrentUser(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "authentication required"})
		return
	}

	fileID := chi.URLParam(r, "id")
	if _, err := uuid.Parse(fileID); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid file id"})
		return
	}

	rec, err := h.fetchOwnedFile(r.Context(), user.ID, fileID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "file not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not load file metadata"})
		return
	}
	rows, err := h.DB.Query(r.Context(), `
		SELECT id, job_type, status, output_storage_key, error_message, attempts, created_at, updated_at
		FROM preview_jobs
		WHERE file_id=$1
		ORDER BY created_at DESC
		LIMIT 50
	`, rec.ID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not list preview jobs"})
		return
	}
	defer rows.Close()

	jobs := make([]map[string]any, 0, 16)
	for rows.Next() {
		var id string
		var jobType string
		var status string
		var outputKey *string
		var errorMessage *string
		var attempts int
		var createdAt time.Time
		var updatedAt time.Time

		if err := rows.Scan(&id, &jobType, &status, &outputKey, &errorMessage, &attempts, &createdAt, &updatedAt); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not scan preview jobs"})
			return
		}

		jobs = append(jobs, map[string]any{
			"id":               id,
			"jobType":          jobType,
			"status":           status,
			"outputStorageKey": outputKey,
			"errorMessage":     errorMessage,
			"attempts":         attempts,
			"createdAt":        createdAt,
			"updatedAt":        updatedAt,
		})
	}
	if rows.Err() != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not read preview jobs"})
		return
	}

	previewRecords, err := h.listFilePreviews(r.Context(), rec.ID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not list file previews"})
		return
	}
	previews := make([]map[string]any, 0, 8)
	for _, p := range previewRecords {
		previews = append(previews, map[string]any{
			"id":         p.ID,
			"type":       p.Type,
			"storageKey": p.StorageKey,
			"mimeType":   p.MimeType,
			"sizeBytes":  p.SizeBytes,
			"status":     p.Status,
			"createdAt":  p.CreatedAt,
			"updatedAt":  p.UpdatedAt,
		})
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"fileId":   rec.ID,
		"jobs":     jobs,
		"previews": previews,
	})
}

func detectPreviewMode(rec fileRecord) (category string, method string, supported bool) {
	mimeType := ""
	if rec.MimeType != nil {
		mimeType = strings.ToLower(strings.TrimSpace(*rec.MimeType))
	}
	ext := ""
	if rec.Extension != nil {
		ext = strings.ToLower(strings.TrimSpace(*rec.Extension))
	}

	switch {
	case strings.HasPrefix(mimeType, "image/"):
		return "image", "stream", true
	case strings.HasPrefix(mimeType, "video/"):
		return "video", "stream", true
	case strings.HasPrefix(mimeType, "audio/"):
		return "audio", "stream", true
	case mimeType == "application/pdf":
		return "pdf", "stream", true
	case isOfficeLikeMime(mimeType) || isOfficeLikeExt(ext):
		return "office", "async_generated", true
	case isTextLikeMime(mimeType) || isTextLikeExt(ext):
		return "text", "text_partial", true
	default:
		return "binary", "unsupported", false
	}
}

func previewAllowedByConfig(cfg previewRuntimeSettings, category string, public bool) (bool, string) {
	if !cfg.Enabled {
		return false, "preview is disabled by admin settings"
	}
	if public && !cfg.PublicPreviewEnabled {
		return false, "public preview is disabled by admin settings"
	}
	if (category == "video" || category == "audio") && !cfg.MediaEnabled {
		return false, "media preview is disabled by admin settings"
	}
	if category == "office" && !cfg.OfficeEnabled {
		return false, "office preview is disabled by admin settings"
	}
	return true, ""
}

func isTextLikeMime(mimeType string) bool {
	if strings.HasPrefix(mimeType, "text/") {
		return true
	}
	switch mimeType {
	case "application/json", "application/xml", "application/javascript", "application/x-sh", "application/x-httpd-php":
		return true
	default:
		return false
	}
}

func isTextLikeExt(ext string) bool {
	switch ext {
	case "txt", "md", "markdown", "json", "xml", "yaml", "yml", "csv", "log", "ini", "env", "conf", "toml", "sql",
		"js", "jsx", "ts", "tsx", "html", "css", "scss", "go", "py", "java", "c", "cpp", "h", "hpp", "cs",
		"php", "rb", "rs", "swift", "kt", "sh", "bash", "zsh", "ps1", "dockerfile", "makefile":
		return true
	default:
		return false
	}
}

func isOfficeLikeMime(mimeType string) bool {
	switch mimeType {
	case "application/msword",
		"application/vnd.openxmlformats-officedocument.wordprocessingml.document",
		"application/vnd.ms-excel",
		"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
		"application/vnd.ms-powerpoint",
		"application/vnd.openxmlformats-officedocument.presentationml.presentation",
		"application/vnd.oasis.opendocument.text",
		"application/vnd.oasis.opendocument.spreadsheet",
		"application/vnd.oasis.opendocument.presentation",
		"application/rtf":
		return true
	default:
		return false
	}
}

func isOfficeLikeExt(ext string) bool {
	switch ext {
	case "doc", "docx", "xls", "xlsx", "ppt", "pptx", "odt", "ods", "odp", "rtf":
		return true
	default:
		return false
	}
}

func (h Handler) previewTextMaxBytes(r *http.Request) int64 {
	var raw json.RawMessage
	err := h.DB.QueryRow(r.Context(), `SELECT value FROM system_settings WHERE key='preview.text_max_bytes'`).Scan(&raw)
	if err != nil {
		return 1 * 1024 * 1024
	}
	var val int64
	if err := json.Unmarshal(raw, &val); err != nil || val <= 0 {
		return 1 * 1024 * 1024
	}
	return val
}

func (h Handler) previewFile(w http.ResponseWriter, r *http.Request) {
	h.streamOwnedFile(w, r, true)
}

func (h Handler) downloadFile(w http.ResponseWriter, r *http.Request) {
	h.streamOwnedFile(w, r, false)
}

func (h Handler) streamOwnedFile(w http.ResponseWriter, r *http.Request, inline bool) {
	user, ok := middleware.CurrentUser(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "authentication required"})
		return
	}

	fileID := chi.URLParam(r, "id")
	if _, err := uuid.Parse(fileID); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid file id"})
		return
	}

	rec, err := h.fetchOwnedFile(r.Context(), user.ID, fileID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "file not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not read file metadata"})
		return
	}

	storageKey := rec.StorageKey
	lastModified := rec.UpdatedAt
	displayName := rec.OriginalName
	mimeType := "application/octet-stream"
	if rec.MimeType != nil && *rec.MimeType != "" {
		mimeType = *rec.MimeType
	}

	if inline {
		category, _, _ := detectPreviewMode(rec)
		previewCfg, cfgErr := h.getPreviewRuntimeSettings(r.Context())
		if cfgErr != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not load preview settings"})
			return
		}
		if allowed, reason := previewAllowedByConfig(previewCfg, category, false); !allowed {
			if reason == "" {
				reason = "preview is disabled by admin settings"
			}
			writeJSON(w, http.StatusForbidden, map[string]string{"error": reason})
			return
		}

		variant := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("variant")))
		if variant != "" {
			if variant != "thumbnail" && variant != "pdf" && variant != "metadata" {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "unsupported preview variant"})
				return
			}
			previewRec, err := h.getReadyFilePreview(r.Context(), rec.ID, variant)
			if err != nil {
				if errors.Is(err, pgx.ErrNoRows) {
					writeJSON(w, http.StatusNotFound, map[string]string{"error": "requested preview variant is not ready"})
					return
				}
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not load preview variant"})
				return
			}
			if previewRec.StorageKey == nil || strings.TrimSpace(*previewRec.StorageKey) == "" {
				writeJSON(w, http.StatusNotFound, map[string]string{"error": "requested preview variant is not ready"})
				return
			}

			storageKey = *previewRec.StorageKey
			lastModified = previewRec.UpdatedAt
			if previewRec.MimeType != nil && strings.TrimSpace(*previewRec.MimeType) != "" {
				mimeType = *previewRec.MimeType
			}
		}
	}

	obj, err := h.Storage.Stat(r.Context(), storageKey)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "file data not found"})
		return
	}

	dispositionType := "attachment"
	if inline {
		dispositionType = "inline"
	}

	w.Header().Set("Content-Type", mimeType)
	w.Header().Set("Accept-Ranges", "bytes")
	w.Header().Set("ETag", obj.ETag)
	w.Header().Set("Last-Modified", lastModified.UTC().Format(http.TimeFormat))
	w.Header().Set("Content-Disposition", fmt.Sprintf("%s; filename=%q", dispositionType, displayName))

	rangeHeader := strings.TrimSpace(r.Header.Get("Range"))
	if rangeHeader == "" {
		w.Header().Set("Content-Length", strconv.FormatInt(obj.Size, 10))
		stream, err := h.Storage.GetStream(r.Context(), storageKey)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not open file"})
			return
		}
		defer stream.Close()
		w.WriteHeader(http.StatusOK)
		_, _ = io.Copy(w, stream)
		return
	}

	start, end, ok := parseSingleRange(rangeHeader, obj.Size)
	if !ok {
		w.Header().Set("Content-Range", fmt.Sprintf("bytes */%d", obj.Size))
		writeJSON(w, http.StatusRequestedRangeNotSatisfiable, map[string]string{"error": "invalid range"})
		return
	}

	stream, err := h.Storage.GetRangeStream(r.Context(), storageKey, start, end)
	if err != nil {
		writeJSON(w, http.StatusRequestedRangeNotSatisfiable, map[string]string{"error": "invalid range"})
		return
	}
	defer stream.Close()

	length := end - start + 1
	w.Header().Set("Content-Length", strconv.FormatInt(length, 10))
	w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, obj.Size))
	w.WriteHeader(http.StatusPartialContent)
	_, _ = io.Copy(w, stream)
}

func parseSingleRange(header string, size int64) (start int64, end int64, ok bool) {
	if !strings.HasPrefix(header, "bytes=") {
		return 0, 0, false
	}
	value := strings.TrimPrefix(header, "bytes=")
	if strings.Contains(value, ",") {
		return 0, 0, false
	}

	parts := strings.SplitN(value, "-", 2)
	if len(parts) != 2 {
		return 0, 0, false
	}

	left := strings.TrimSpace(parts[0])
	right := strings.TrimSpace(parts[1])

	if left == "" {
		suffix, err := strconv.ParseInt(right, 10, 64)
		if err != nil || suffix <= 0 {
			return 0, 0, false
		}
		if suffix > size {
			suffix = size
		}
		return size - suffix, size - 1, true
	}

	start, err := strconv.ParseInt(left, 10, 64)
	if err != nil || start < 0 || start >= size {
		return 0, 0, false
	}

	if right == "" {
		return start, size - 1, true
	}

	end, err = strconv.ParseInt(right, 10, 64)
	if err != nil || end < start {
		return 0, 0, false
	}
	if end >= size {
		end = size - 1
	}

	return start, end, true
}

type renameFileRequest struct {
	Name string `json:"name"`
}

func (h Handler) renameFile(w http.ResponseWriter, r *http.Request) {
	user, ok := middleware.CurrentUser(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "authentication required"})
		return
	}

	fileID := chi.URLParam(r, "id")
	if _, err := uuid.Parse(fileID); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid file id"})
		return
	}

	var req renameFileRequest
	if err := ReadJSON(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	name := normalizeNodeName(req.Name)
	if name == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name is required"})
		return
	}

	cmd, err := h.DB.Exec(r.Context(), `
		UPDATE files
		SET name=$1, updated_at=now()
		WHERE id=$2 AND owner_id=$3 AND deleted_at IS NULL
	`, name, fileID, user.ID)
	if err != nil {
		if isUniqueViolation(err) {
			writeJSON(w, http.StatusConflict, map[string]string{"error": "an item with this name already exists in this folder"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not rename file"})
		return
	}
	if cmd.RowsAffected() == 0 {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "file not found"})
		return
	}

	h.insertAudit(r.Context(), &user.ID, "file.renamed", "file", &fileID, clientIP(r), r.UserAgent(), map[string]any{"name": name})
	writeJSON(w, http.StatusOK, map[string]any{"id": fileID, "name": name})
}

type moveFileRequest struct {
	FolderID *string `json:"folderId"`
}

func (h Handler) moveFile(w http.ResponseWriter, r *http.Request) {
	user, ok := middleware.CurrentUser(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "authentication required"})
		return
	}

	fileID := chi.URLParam(r, "id")
	if _, err := uuid.Parse(fileID); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid file id"})
		return
	}

	var req moveFileRequest
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

	cmd, err := h.DB.Exec(r.Context(), `
		UPDATE files
		SET folder_id=$1, updated_at=now()
		WHERE id=$2 AND owner_id=$3 AND deleted_at IS NULL
	`, folderID, fileID, user.ID)
	if err != nil {
		if isUniqueViolation(err) {
			writeJSON(w, http.StatusConflict, map[string]string{"error": "an item with this name already exists in destination folder"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not move file"})
		return
	}
	if cmd.RowsAffected() == 0 {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "file not found"})
		return
	}

	h.insertAudit(r.Context(), &user.ID, "file.moved", "file", &fileID, clientIP(r), r.UserAgent(), map[string]any{"folderId": folderID})
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (h Handler) deleteFile(w http.ResponseWriter, r *http.Request) {
	user, ok := middleware.CurrentUser(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "authentication required"})
		return
	}

	fileID := chi.URLParam(r, "id")
	if _, err := uuid.Parse(fileID); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid file id"})
		return
	}

	rec, err := h.fetchOwnedFile(r.Context(), user.ID, fileID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "file not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not read file metadata"})
		return
	}

	tx, err := h.DB.Begin(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not begin delete transaction"})
		return
	}
	defer tx.Rollback(r.Context())

	cmd, err := tx.Exec(r.Context(), `
		UPDATE files
		SET deleted_at=now(), status='deleted', updated_at=now()
		WHERE id=$1 AND owner_id=$2 AND deleted_at IS NULL
	`, fileID, user.ID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not delete file"})
		return
	}
	if cmd.RowsAffected() == 0 {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "file not found"})
		return
	}

	if _, err := tx.Exec(r.Context(), `UPDATE users SET used_storage_bytes = GREATEST(0, used_storage_bytes - $1), updated_at=now() WHERE id=$2`, rec.SizeBytes, user.ID); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not update usage"})
		return
	}

	if err := tx.Commit(r.Context()); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not commit delete"})
		return
	}

	_ = h.Storage.Delete(r.Context(), rec.StorageKey)
	h.insertAudit(r.Context(), &user.ID, "file.deleted", "file", &fileID, clientIP(r), r.UserAgent(), map[string]any{})
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
