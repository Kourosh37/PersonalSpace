package httpapi

import (
	"database/sql"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"space/backend/internal/auth"
	"space/backend/internal/middleware"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type shareRecord struct {
	ID                string
	OwnerID           string
	TargetType        string
	TargetID          string
	TokenHash         string
	PasswordHash      *string
	ExpiresAt         *time.Time
	AllowPreview      bool
	AllowDownload     bool
	AllowFolderBrowse bool
	MaxDownloads      *int
	DownloadCount     int
	IsRevoked         bool
	CreatedAt         time.Time
}

func (h Handler) registerShareRoutes(api chi.Router, authMW middleware.AuthMiddleware) {
	api.With(authMW.RequireAuth).Post("/shares", h.createShare)
	api.With(authMW.RequireAuth).Get("/shares", h.listShares)
	api.With(authMW.RequireAuth).Post("/shares/{id}/revoke", h.revokeShare)

	api.Get("/public/shares/{token}", h.publicShareInfo)
	api.Post("/public/shares/{token}/password", h.publicSharePasswordCheck)
	api.Get("/public/shares/{token}/items", h.publicShareItems)
	api.Get("/public/shares/{token}/files/{fileId}/download", h.publicShareFileDownload)
	api.Get("/public/shares/{token}/files/{fileId}/preview", h.publicShareFilePreview)
}

type createShareRequest struct {
	TargetType        string     `json:"targetType"`
	TargetID          string     `json:"targetId"`
	Password          *string    `json:"password"`
	ExpiresAt         *time.Time `json:"expiresAt"`
	AllowPreview      *bool      `json:"allowPreview"`
	AllowDownload     *bool      `json:"allowDownload"`
	AllowFolderBrowse *bool      `json:"allowFolderBrowse"`
	MaxDownloads      *int       `json:"maxDownloads"`
}

func (h Handler) createShare(w http.ResponseWriter, r *http.Request) {
	user, ok := middleware.CurrentUser(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "authentication required"})
		return
	}

	var req createShareRequest
	if err := ReadJSON(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	req.TargetType = strings.TrimSpace(req.TargetType)
	if req.TargetType != "file" && req.TargetType != "folder" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "targetType must be file or folder"})
		return
	}

	if _, err := uuid.Parse(req.TargetID); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid targetId"})
		return
	}

	if req.ExpiresAt != nil && req.ExpiresAt.Before(time.Now().UTC()) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "expiresAt must be in the future"})
		return
	}

	if req.MaxDownloads != nil && *req.MaxDownloads <= 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "maxDownloads must be greater than zero"})
		return
	}

	if req.TargetType == "file" {
		var exists bool
		if err := h.DB.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM files WHERE id=$1 AND owner_id=$2 AND deleted_at IS NULL)`, req.TargetID, user.ID).Scan(&exists); err != nil || !exists {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "target file not found"})
			return
		}
	} else {
		var exists bool
		if err := h.DB.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM folders WHERE id=$1 AND owner_id=$2 AND deleted_at IS NULL)`, req.TargetID, user.ID).Scan(&exists); err != nil || !exists {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "target folder not found"})
			return
		}
	}

	rawToken, tokenHash, err := auth.NewSessionToken()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not create share token"})
		return
	}

	var passwordHash any = nil
	if req.Password != nil && strings.TrimSpace(*req.Password) != "" {
		hash, err := auth.HashPassword(strings.TrimSpace(*req.Password))
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not hash share password"})
			return
		}
		passwordHash = hash
	}

	allowPreview := true
	if req.AllowPreview != nil {
		allowPreview = *req.AllowPreview
	}
	allowDownload := true
	if req.AllowDownload != nil {
		allowDownload = *req.AllowDownload
	}
	allowFolderBrowse := true
	if req.AllowFolderBrowse != nil {
		allowFolderBrowse = *req.AllowFolderBrowse
	}

	id := uuid.NewString()
	now := time.Now().UTC()
	_, err = h.DB.Exec(r.Context(), `
		INSERT INTO share_links (
			id, owner_id, target_type, target_id, token_hash, token_preview, password_hash, expires_at,
			allow_preview, allow_download, allow_folder_browse, max_downloads, download_count, is_revoked, created_at, updated_at
		)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,0,false,$13,$14)
	`, id, user.ID, req.TargetType, req.TargetID, tokenHash, rawToken[:8], passwordHash, req.ExpiresAt, allowPreview, allowDownload, allowFolderBrowse, req.MaxDownloads, now, now)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not create share"})
		return
	}

	shareURL := strings.TrimRight(h.Cfg.PublicBaseURL, "/") + "/s/" + rawToken
	h.insertAudit(r.Context(), &user.ID, "share.created", "share", &id, clientIP(r), r.UserAgent(), map[string]any{"targetType": req.TargetType, "targetId": req.TargetID})
	writeJSON(w, http.StatusCreated, map[string]any{"id": id, "url": shareURL, "token": rawToken})
}

func (h Handler) listShares(w http.ResponseWriter, r *http.Request) {
	user, ok := middleware.CurrentUser(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "authentication required"})
		return
	}

	rows, err := h.DB.Query(r.Context(), `
		SELECT id, target_type, target_id, token_preview, expires_at, allow_preview, allow_download, allow_folder_browse, max_downloads, download_count, is_revoked, created_at
		FROM share_links
		WHERE owner_id=$1
		ORDER BY created_at DESC
	`, user.ID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not list shares"})
		return
	}
	defer rows.Close()

	items := make([]map[string]any, 0)
	for rows.Next() {
		var id, targetType, targetID, tokenPreview string
		var expiresAt *time.Time
		var allowPreview, allowDownload, allowFolderBrowse bool
		var maxDownloads *int
		var downloadCount int
		var isRevoked bool
		var createdAt time.Time
		if err := rows.Scan(&id, &targetType, &targetID, &tokenPreview, &expiresAt, &allowPreview, &allowDownload, &allowFolderBrowse, &maxDownloads, &downloadCount, &isRevoked, &createdAt); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not decode shares"})
			return
		}
		items = append(items, map[string]any{
			"id":                id,
			"targetType":        targetType,
			"targetId":          targetID,
			"tokenPreview":      tokenPreview,
			"expiresAt":         expiresAt,
			"allowPreview":      allowPreview,
			"allowDownload":     allowDownload,
			"allowFolderBrowse": allowFolderBrowse,
			"maxDownloads":      maxDownloads,
			"downloadCount":     downloadCount,
			"isRevoked":         isRevoked,
			"createdAt":         createdAt,
		})
	}

	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (h Handler) revokeShare(w http.ResponseWriter, r *http.Request) {
	user, ok := middleware.CurrentUser(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "authentication required"})
		return
	}

	shareID := chi.URLParam(r, "id")
	if _, err := uuid.Parse(shareID); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid share id"})
		return
	}

	cmd, err := h.DB.Exec(r.Context(), `
		UPDATE share_links SET is_revoked=true, updated_at=now()
		WHERE id=$1 AND owner_id=$2
	`, shareID, user.ID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not revoke share"})
		return
	}
	if cmd.RowsAffected() == 0 {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "share not found"})
		return
	}

	h.insertAudit(r.Context(), &user.ID, "share.revoked", "share", &shareID, clientIP(r), r.UserAgent(), map[string]any{})
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (h Handler) publicShareInfo(w http.ResponseWriter, r *http.Request) {
	share, err := h.resolvePublicShare(r, "", false)
	if err != nil {
		h.publicShareErr(w, err)
		return
	}

	name, err := h.shareTargetName(r, share)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "shared target not found"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"id":                share.ID,
		"targetType":        share.TargetType,
		"targetId":          share.TargetID,
		"name":              name,
		"allowPreview":      share.AllowPreview,
		"allowDownload":     share.AllowDownload,
		"allowFolderBrowse": share.AllowFolderBrowse,
		"expiresAt":         share.ExpiresAt,
		"passwordRequired":  share.PasswordHash != nil,
	})
}

func (h Handler) publicSharePasswordCheck(w http.ResponseWriter, r *http.Request) {
	password := strings.TrimSpace(r.URL.Query().Get("password"))
	if password == "" {
		var req struct {
			Password string `json:"password"`
		}
		if err := ReadJSON(r, &req); err == nil {
			password = strings.TrimSpace(req.Password)
		}
	}

	share, err := h.resolvePublicShare(r, password, true)
	if err != nil {
		h.publicShareErr(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "shareId": share.ID})
}

func (h Handler) publicShareItems(w http.ResponseWriter, r *http.Request) {
	password := getPublicSharePassword(r)
	share, err := h.resolvePublicShare(r, password, true)
	if err != nil {
		h.publicShareErr(w, err)
		return
	}
	if share.TargetType != "folder" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "share target is not a folder"})
		return
	}
	if !share.AllowFolderBrowse {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "folder browsing is disabled for this share"})
		return
	}

	parentID, err := optionalUUIDFromQuery(r, "parentId")
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	if parentID != nil {
		var inTree bool
		err := h.DB.QueryRow(r.Context(), `
			WITH RECURSIVE tree AS (
				SELECT id FROM folders WHERE id=$1 AND deleted_at IS NULL
				UNION ALL
				SELECT f.id FROM folders f JOIN tree t ON f.parent_id=t.id WHERE f.deleted_at IS NULL
			)
			SELECT EXISTS(SELECT 1 FROM tree WHERE id=$2)
		`, share.TargetID, *parentID).Scan(&inTree)
		if err != nil || !inTree {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid parentId for this share"})
			return
		}
	}

	items := make([]browserItem, 0, 128)
	folderRows, err := h.DB.Query(r.Context(), `
		SELECT id, name, parent_id, created_at, updated_at
		FROM folders
		WHERE deleted_at IS NULL
		  AND (($2::uuid IS NULL AND parent_id = $1::uuid) OR parent_id = $2::uuid)
		ORDER BY name ASC
	`, share.TargetID, parentID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not load folder items"})
		return
	}
	defer folderRows.Close()

	for folderRows.Next() {
		var item browserItem
		item.Type = itemTypeFolder
		if err := folderRows.Scan(&item.ID, &item.Name, &item.ParentID, &item.CreatedAt, &item.ModifiedAt); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not decode folder rows"})
			return
		}
		items = append(items, item)
	}

	fileRows, err := h.DB.Query(r.Context(), `
		SELECT id, name, folder_id, size_bytes, mime_type, extension, created_at, updated_at
		FROM files
		WHERE deleted_at IS NULL
		  AND (($2::uuid IS NULL AND folder_id = $1::uuid) OR folder_id = $2::uuid)
		ORDER BY name ASC
	`, share.TargetID, parentID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not load file items"})
		return
	}
	defer fileRows.Close()

	for fileRows.Next() {
		var item browserItem
		item.Type = itemTypeFile
		if err := fileRows.Scan(&item.ID, &item.Name, &item.ParentID, &item.SizeBytes, &item.MimeType, &item.Extension, &item.CreatedAt, &item.ModifiedAt); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not decode file rows"})
			return
		}
		items = append(items, item)
	}

	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (h Handler) publicShareFileDownload(w http.ResponseWriter, r *http.Request) {
	h.publicShareFileStream(w, r, false)
}

func (h Handler) publicShareFilePreview(w http.ResponseWriter, r *http.Request) {
	h.publicShareFileStream(w, r, true)
}

func (h Handler) publicShareFileStream(w http.ResponseWriter, r *http.Request, inline bool) {
	password := getPublicSharePassword(r)
	share, err := h.resolvePublicShare(r, password, true)
	if err != nil {
		h.publicShareErr(w, err)
		return
	}

	if inline && !share.AllowPreview {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "preview is disabled for this share"})
		return
	}
	if !inline && !share.AllowDownload {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "download is disabled for this share"})
		return
	}

	fileID := chi.URLParam(r, "fileId")
	if _, err := uuid.Parse(fileID); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid file id"})
		return
	}

	rec, err := h.fetchFileForShare(r, share, fileID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "file not found in this share"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not load file"})
		return
	}

	if share.MaxDownloads != nil && share.DownloadCount >= *share.MaxDownloads {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "share download limit exceeded"})
		return
	}

	obj, err := h.Storage.Stat(r.Context(), rec.StorageKey)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "file data not found"})
		return
	}

	mimeType := "application/octet-stream"
	if rec.MimeType != nil && *rec.MimeType != "" {
		mimeType = *rec.MimeType
	}
	w.Header().Set("Content-Type", mimeType)
	w.Header().Set("Accept-Ranges", "bytes")
	w.Header().Set("ETag", obj.ETag)
	w.Header().Set("Last-Modified", rec.UpdatedAt.UTC().Format(http.TimeFormat))
	if inline {
		w.Header().Set("Content-Disposition", fmt.Sprintf("inline; filename=%q", rec.OriginalName))
	} else {
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", rec.OriginalName))
	}

	rangeHeader := strings.TrimSpace(r.Header.Get("Range"))
	if rangeHeader == "" {
		w.Header().Set("Content-Length", fmt.Sprintf("%d", obj.Size))
		stream, err := h.Storage.GetStream(r.Context(), rec.StorageKey)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not open file"})
			return
		}
		defer stream.Close()
		w.WriteHeader(http.StatusOK)
		_, _ = io.Copy(w, stream)
		h.bumpShareDownloadCount(r, share)
		return
	}

	start, end, ok := parseSingleRange(rangeHeader, obj.Size)
	if !ok {
		w.Header().Set("Content-Range", fmt.Sprintf("bytes */%d", obj.Size))
		writeJSON(w, http.StatusRequestedRangeNotSatisfiable, map[string]string{"error": "invalid range"})
		return
	}
	stream, err := h.Storage.GetRangeStream(r.Context(), rec.StorageKey, start, end)
	if err != nil {
		writeJSON(w, http.StatusRequestedRangeNotSatisfiable, map[string]string{"error": "invalid range"})
		return
	}
	defer stream.Close()
	length := end - start + 1
	w.Header().Set("Content-Length", fmt.Sprintf("%d", length))
	w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, obj.Size))
	w.WriteHeader(http.StatusPartialContent)
	_, _ = io.Copy(w, stream)
	h.bumpShareDownloadCount(r, share)
}

func (h Handler) fetchFileForShare(r *http.Request, share shareRecord, fileID string) (fileRecord, error) {
	if share.TargetType == "file" {
		if share.TargetID != fileID {
			return fileRecord{}, pgx.ErrNoRows
		}
	}

	var rec fileRecord
	if share.TargetType == "file" {
		err := h.DB.QueryRow(r.Context(), `
			SELECT id, owner_id, folder_id, name, original_name, storage_key, size_bytes, mime_type, extension, updated_at
			FROM files
			WHERE id=$1 AND deleted_at IS NULL
		`, fileID).Scan(&rec.ID, &rec.OwnerID, &rec.FolderID, &rec.Name, &rec.OriginalName, &rec.StorageKey, &rec.SizeBytes, &rec.MimeType, &rec.Extension, &rec.UpdatedAt)
		if err != nil {
			return fileRecord{}, err
		}
		return rec, nil
	}

	err := h.DB.QueryRow(r.Context(), `
		WITH RECURSIVE tree AS (
			SELECT id FROM folders WHERE id=$1 AND deleted_at IS NULL
			UNION ALL
			SELECT f.id FROM folders f JOIN tree t ON f.parent_id=t.id WHERE f.deleted_at IS NULL
		)
		SELECT fi.id, fi.owner_id, fi.folder_id, fi.name, fi.original_name, fi.storage_key, fi.size_bytes, fi.mime_type, fi.extension, fi.updated_at
		FROM files fi
		WHERE fi.id=$2 AND fi.deleted_at IS NULL AND fi.folder_id IN (SELECT id FROM tree)
	`, share.TargetID, fileID).Scan(&rec.ID, &rec.OwnerID, &rec.FolderID, &rec.Name, &rec.OriginalName, &rec.StorageKey, &rec.SizeBytes, &rec.MimeType, &rec.Extension, &rec.UpdatedAt)
	if err != nil {
		return fileRecord{}, err
	}
	return rec, nil
}

func getPublicSharePassword(r *http.Request) string {
	if value := strings.TrimSpace(r.Header.Get("X-Share-Password")); value != "" {
		return value
	}
	return strings.TrimSpace(r.URL.Query().Get("password"))
}

func (h Handler) resolvePublicShare(r *http.Request, providedPassword string, enforcePassword bool) (shareRecord, error) {
	rawToken := strings.TrimSpace(chi.URLParam(r, "token"))
	if rawToken == "" {
		return shareRecord{}, errors.New("invalid share token")
	}

	tokenHash := auth.HashToken(rawToken)
	var rec shareRecord
	var expiresAt sql.NullTime
	var passwordHash sql.NullString
	var maxDownloads sql.NullInt32
	err := h.DB.QueryRow(r.Context(), `
		SELECT id, owner_id, target_type, target_id, token_hash, password_hash, expires_at, allow_preview, allow_download, allow_folder_browse, max_downloads, download_count, is_revoked, created_at
		FROM share_links
		WHERE token_hash=$1
	`, tokenHash).Scan(
		&rec.ID,
		&rec.OwnerID,
		&rec.TargetType,
		&rec.TargetID,
		&rec.TokenHash,
		&passwordHash,
		&expiresAt,
		&rec.AllowPreview,
		&rec.AllowDownload,
		&rec.AllowFolderBrowse,
		&maxDownloads,
		&rec.DownloadCount,
		&rec.IsRevoked,
		&rec.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return shareRecord{}, fmt.Errorf("share_not_found")
		}
		return shareRecord{}, err
	}

	if passwordHash.Valid {
		value := passwordHash.String
		rec.PasswordHash = &value
	}
	if expiresAt.Valid {
		value := expiresAt.Time
		rec.ExpiresAt = &value
	}
	if maxDownloads.Valid {
		v := int(maxDownloads.Int32)
		rec.MaxDownloads = &v
	}

	if rec.IsRevoked {
		return shareRecord{}, fmt.Errorf("share_revoked")
	}
	if rec.ExpiresAt != nil && rec.ExpiresAt.Before(time.Now().UTC()) {
		return shareRecord{}, fmt.Errorf("share_expired")
	}
	if enforcePassword && rec.PasswordHash != nil {
		if strings.TrimSpace(providedPassword) == "" {
			return shareRecord{}, fmt.Errorf("share_password_required")
		}
		if !auth.VerifyPassword(*rec.PasswordHash, providedPassword) {
			return shareRecord{}, fmt.Errorf("share_password_invalid")
		}
	}

	return rec, nil
}

func (h Handler) shareTargetName(r *http.Request, share shareRecord) (string, error) {
	if share.TargetType == "file" {
		var name string
		err := h.DB.QueryRow(r.Context(), `SELECT name FROM files WHERE id=$1 AND deleted_at IS NULL`, share.TargetID).Scan(&name)
		return name, err
	}
	var name string
	err := h.DB.QueryRow(r.Context(), `SELECT name FROM folders WHERE id=$1 AND deleted_at IS NULL`, share.TargetID).Scan(&name)
	return name, err
}

func (h Handler) publicShareErr(w http.ResponseWriter, err error) {
	code := http.StatusUnauthorized
	message := "share access denied"
	switch err.Error() {
	case "share_not_found":
		code = http.StatusNotFound
		message = "share not found"
	case "share_revoked":
		code = http.StatusGone
		message = "share revoked"
	case "share_expired":
		code = http.StatusGone
		message = "share expired"
	case "share_password_required":
		code = http.StatusUnauthorized
		message = "share password required"
	case "share_password_invalid":
		code = http.StatusUnauthorized
		message = "invalid share password"
	}
	writeJSON(w, code, map[string]string{"error": message})
}

func (h Handler) bumpShareDownloadCount(r *http.Request, share shareRecord) {
	_, _ = h.DB.Exec(r.Context(), `UPDATE share_links SET download_count = download_count + 1, updated_at=now() WHERE id=$1`, share.ID)
}
