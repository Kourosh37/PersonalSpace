package httpapi

import (
	"database/sql"
	"encoding/csv"
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
	api.With(authMW.RequireAuth).Get("/shares/{id}", h.getShare)
	api.With(authMW.RequireAuth).Patch("/shares/{id}", h.patchShare)
	api.With(authMW.RequireAuth).Delete("/shares/{id}", h.deleteShare)
	api.With(authMW.RequireAuth).Post("/shares/{id}/revoke", h.revokeShare)

	api.Get("/public/shares/{token}", h.publicShareInfo)
	api.Post("/public/shares/{token}/password", h.publicSharePasswordCheck)
	api.Get("/public/shares/{token}/items", h.publicShareItems)
	api.Get("/public/shares/{token}/files/{fileId}/preview-info", h.publicShareFilePreviewInfo)
	api.Get("/public/shares/{token}/files/{fileId}/preview-content", h.publicShareFilePreviewContent)
	api.Get("/public/shares/{token}/files/{fileId}/download", h.publicShareFileDownload)
	api.Get("/public/shares/{token}/files/{fileId}/preview", h.publicShareFilePreview)
	api.Get("/public/shares/{token}/folders/{folderId}/download-zip", h.publicShareFolderZip)
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

	if err := h.enforceSharePolicyForCreate(r, allowPreview, allowDownload); err != nil {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": err.Error()})
		return
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

func (h Handler) getShare(w http.ResponseWriter, r *http.Request) {
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

	share, err := h.fetchOwnedShare(r, user.ID, shareID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "share not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not load share"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"id":                share.ID,
		"targetType":        share.TargetType,
		"targetId":          share.TargetID,
		"expiresAt":         share.ExpiresAt,
		"allowPreview":      share.AllowPreview,
		"allowDownload":     share.AllowDownload,
		"allowFolderBrowse": share.AllowFolderBrowse,
		"maxDownloads":      share.MaxDownloads,
		"downloadCount":     share.DownloadCount,
		"isRevoked":         share.IsRevoked,
		"createdAt":         share.CreatedAt,
	})
}

type patchShareRequest struct {
	Password          *string    `json:"password"`
	ExpiresAt         *time.Time `json:"expiresAt"`
	ClearPassword     *bool      `json:"clearPassword"`
	ClearExpiration   *bool      `json:"clearExpiration"`
	AllowPreview      *bool      `json:"allowPreview"`
	AllowDownload     *bool      `json:"allowDownload"`
	AllowFolderBrowse *bool      `json:"allowFolderBrowse"`
	MaxDownloads      *int       `json:"maxDownloads"`
	ClearMaxDownloads *bool      `json:"clearMaxDownloads"`
	IsRevoked         *bool      `json:"isRevoked"`
}

func (h Handler) patchShare(w http.ResponseWriter, r *http.Request) {
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

	current, err := h.fetchOwnedShare(r, user.ID, shareID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "share not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not load share"})
		return
	}

	var req patchShareRequest
	if err := ReadJSON(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	passwordHash := current.PasswordHash
	if req.ClearPassword != nil && *req.ClearPassword {
		passwordHash = nil
	}
	if req.Password != nil {
		plain := strings.TrimSpace(*req.Password)
		if plain == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "password cannot be empty"})
			return
		}
		hash, err := auth.HashPassword(plain)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not hash password"})
			return
		}
		passwordHash = &hash
	}

	expiresAt := current.ExpiresAt
	if req.ClearExpiration != nil && *req.ClearExpiration {
		expiresAt = nil
	}
	if req.ExpiresAt != nil {
		if req.ExpiresAt.Before(time.Now().UTC()) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "expiresAt must be in the future"})
			return
		}
		t := req.ExpiresAt.UTC()
		expiresAt = &t
	}

	maxDownloads := current.MaxDownloads
	if req.ClearMaxDownloads != nil && *req.ClearMaxDownloads {
		maxDownloads = nil
	}
	if req.MaxDownloads != nil {
		if *req.MaxDownloads <= 0 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "maxDownloads must be greater than zero"})
			return
		}
		val := *req.MaxDownloads
		maxDownloads = &val
	}

	allowPreview := current.AllowPreview
	if req.AllowPreview != nil {
		allowPreview = *req.AllowPreview
	}
	allowDownload := current.AllowDownload
	if req.AllowDownload != nil {
		allowDownload = *req.AllowDownload
	}
	allowFolderBrowse := current.AllowFolderBrowse
	if req.AllowFolderBrowse != nil {
		allowFolderBrowse = *req.AllowFolderBrowse
	}
	isRevoked := current.IsRevoked
	if req.IsRevoked != nil {
		isRevoked = *req.IsRevoked
	}

	if !isRevoked {
		if err := h.enforceSharePolicyForCreate(r, allowPreview, allowDownload); err != nil {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": err.Error()})
			return
		}
	}

	var passwordHashParam any = nil
	if passwordHash != nil {
		passwordHashParam = *passwordHash
	}

	_, err = h.DB.Exec(r.Context(), `
		UPDATE share_links
		SET password_hash=$1, expires_at=$2, allow_preview=$3, allow_download=$4, allow_folder_browse=$5, max_downloads=$6, is_revoked=$7, updated_at=now()
		WHERE id=$8 AND owner_id=$9
	`, passwordHashParam, expiresAt, allowPreview, allowDownload, allowFolderBrowse, maxDownloads, isRevoked, shareID, user.ID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not update share"})
		return
	}

	h.insertAudit(r.Context(), &user.ID, "share.updated", "share", &shareID, clientIP(r), r.UserAgent(), map[string]any{
		"allowPreview":      allowPreview,
		"allowDownload":     allowDownload,
		"allowFolderBrowse": allowFolderBrowse,
		"isRevoked":         isRevoked,
	})

	updated, err := h.fetchOwnedShare(r, user.ID, shareID)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"id":                updated.ID,
		"targetType":        updated.TargetType,
		"targetId":          updated.TargetID,
		"expiresAt":         updated.ExpiresAt,
		"allowPreview":      updated.AllowPreview,
		"allowDownload":     updated.AllowDownload,
		"allowFolderBrowse": updated.AllowFolderBrowse,
		"maxDownloads":      updated.MaxDownloads,
		"downloadCount":     updated.DownloadCount,
		"isRevoked":         updated.IsRevoked,
		"createdAt":         updated.CreatedAt,
	})
}

func (h Handler) deleteShare(w http.ResponseWriter, r *http.Request) {
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

	cmd, err := h.DB.Exec(r.Context(), `DELETE FROM share_links WHERE id=$1 AND owner_id=$2`, shareID, user.ID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not delete share"})
		return
	}
	if cmd.RowsAffected() == 0 {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "share not found"})
		return
	}

	h.insertAudit(r.Context(), &user.ID, "share.deleted", "share", &shareID, clientIP(r), r.UserAgent(), map[string]any{})
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
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
	h.insertAudit(r.Context(), nil, "share.public.info_accessed", "share", &share.ID, clientIP(r), r.UserAgent(), map[string]any{
		"targetType": share.TargetType,
		"targetId":   share.TargetID,
	})

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
	if !h.enforceRateLimit(w, r, "share_password_"+chi.URLParam(r, "token"), h.Cfg.ShareRatePerMin) {
		return
	}

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
	h.insertAudit(r.Context(), nil, "share.public.password.ok", "share", &share.ID, clientIP(r), r.UserAgent(), map[string]any{})

	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "shareId": share.ID})
}

func (h Handler) publicShareItems(w http.ResponseWriter, r *http.Request) {
	if !h.enforceRateLimit(w, r, "share_access_"+chi.URLParam(r, "token"), h.Cfg.ShareRatePerMin) {
		return
	}

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
	h.insertAudit(r.Context(), nil, "share.public.items_accessed", "share", &share.ID, clientIP(r), r.UserAgent(), map[string]any{
		"parentId": r.URL.Query().Get("parentId"),
	})

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

func (h Handler) publicShareFilePreviewInfo(w http.ResponseWriter, r *http.Request) {
	if !h.enforceRateLimit(w, r, "share_access_"+chi.URLParam(r, "token"), h.Cfg.ShareRatePerMin) {
		return
	}

	password := getPublicSharePassword(r)
	share, err := h.resolvePublicShare(r, password, true)
	if err != nil {
		h.publicShareErr(w, err)
		return
	}
	sharingCfg, err := h.getSharingRuntimeSettings(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not load sharing settings"})
		return
	}
	if !sharingCfg.PublicPreviewEnabled {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "public preview is disabled by admin settings"})
		return
	}
	if !share.AllowPreview {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "preview is disabled for this share"})
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
	var hasReadyThumbnail bool
	var hasReadyPDF bool
	for _, p := range previews {
		if p.Status == "ready" && p.Type == "thumbnail" {
			hasReadyThumbnail = true
		}
		if p.Status == "ready" && p.Type == "pdf" {
			hasReadyPDF = true
		}
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

	token := chi.URLParam(r, "token")
	streamPreviewURL := "/api/public/shares/" + token + "/files/" + rec.ID + "/preview"
	textPreviewURL := "/api/public/shares/" + token + "/files/" + rec.ID + "/preview-content"
	thumbnailURL := streamPreviewURL + "?variant=thumbnail"
	pdfPreviewURL := streamPreviewURL + "?variant=pdf"

	if allowed, reason := previewAllowedByConfig(previewCfg, category, true); !allowed {
		supported = false
		method = "disabled"
		if reason == "" {
			reason = "public preview is disabled by admin settings"
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"fileId":              rec.ID,
			"name":                rec.Name,
			"mimeType":            rec.MimeType,
			"sizeBytes":           rec.SizeBytes,
			"category":            category,
			"method":              method,
			"supported":           supported,
			"textMaxBytes":        h.previewTextMaxBytes(r),
			"streamPreviewURL":    streamPreviewURL,
			"textPreviewURL":      textPreviewURL,
			"thumbnailURL":        thumbnailURL,
			"pdfPreviewURL":       pdfPreviewURL,
			"generatedPreviews":   previewItems,
			"thumbnailReady":      hasReadyThumbnail,
			"pdfReady":            hasReadyPDF,
			"needsGeneration":     previewNeedsGeneration(category, hasReadyThumbnail, hasReadyPDF),
			"recommendedJobTypes": recommendedPreviewJobTypes(category),
			"reason":              reason,
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"fileId":              rec.ID,
		"name":                rec.Name,
		"mimeType":            rec.MimeType,
		"sizeBytes":           rec.SizeBytes,
		"category":            category,
		"method":              method,
		"supported":           supported,
		"textMaxBytes":        h.previewTextMaxBytes(r),
		"streamPreviewURL":    streamPreviewURL,
		"textPreviewURL":      textPreviewURL,
		"thumbnailURL":        thumbnailURL,
		"pdfPreviewURL":       pdfPreviewURL,
		"generatedPreviews":   previewItems,
		"thumbnailReady":      hasReadyThumbnail,
		"pdfReady":            hasReadyPDF,
		"needsGeneration":     previewNeedsGeneration(category, hasReadyThumbnail, hasReadyPDF),
		"recommendedJobTypes": recommendedPreviewJobTypes(category),
	})
}

func (h Handler) publicShareFilePreviewContent(w http.ResponseWriter, r *http.Request) {
	if !h.enforceRateLimit(w, r, "share_access_"+chi.URLParam(r, "token"), h.Cfg.ShareRatePerMin) {
		return
	}

	password := getPublicSharePassword(r)
	share, err := h.resolvePublicShare(r, password, true)
	if err != nil {
		h.publicShareErr(w, err)
		return
	}
	sharingCfg, err := h.getSharingRuntimeSettings(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not load sharing settings"})
		return
	}
	if !sharingCfg.PublicPreviewEnabled {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "public preview is disabled by admin settings"})
		return
	}
	if !share.AllowPreview {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "preview is disabled for this share"})
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
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not load file metadata"})
		return
	}

	category, method, supported := detectPreviewMode(rec)
	previewCfg, cfgErr := h.getPreviewRuntimeSettings(r.Context())
	if cfgErr != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not load preview settings"})
		return
	}
	if allowed, reason := previewAllowedByConfig(previewCfg, category, true); !allowed {
		if reason == "" {
			reason = "public preview is disabled by admin settings"
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
	token := chi.URLParam(r, "token")
	downloadURL := "/api/public/shares/" + token + "/files/" + rec.ID + "/download"
	if isCSVPreviewCandidate(rec) {
		h.publicShareCSVPreviewContent(w, r, rec, limit, downloadURL)
		return
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
		"downloadURL": downloadURL,
	})
}

func (h Handler) publicShareCSVPreviewContent(w http.ResponseWriter, r *http.Request, rec fileRecord, limit int64, downloadURL string) {
	if limit < 2*1024*1024 {
		limit = 2 * 1024 * 1024
	}
	maxRows := h.previewCSVMaxRows(r)
	if maxRows <= 0 {
		maxRows = 500
	}

	stream, err := h.Storage.GetStream(r.Context(), rec.StorageKey)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "file data not found"})
		return
	}
	defer stream.Close()

	data, err := io.ReadAll(io.LimitReader(stream, limit+1))
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not read csv preview content"})
		return
	}

	truncatedByBytes := int64(len(data)) > limit
	if truncatedByBytes {
		data = data[:limit]
	}
	delimiter := detectCSVDelimiter(data)

	reader := csv.NewReader(strings.NewReader(string(data)))
	reader.Comma = delimiter
	reader.FieldsPerRecord = -1
	reader.LazyQuotes = true
	reader.TrimLeadingSpace = true

	rows := make([][]string, 0, maxRows)
	truncatedByRows := false
	for len(rows) < maxRows {
		record, err := reader.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			break
		}
		rows = append(rows, record)
	}
	if len(rows) >= maxRows {
		if _, err := reader.Read(); err == nil {
			truncatedByRows = true
		}
	}

	headers := []string{}
	previewRows := rows
	if len(rows) > 0 {
		headers = rows[0]
		if len(rows) > 1 {
			previewRows = rows[1:]
		} else {
			previewRows = [][]string{}
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"fileId":         rec.ID,
		"category":       "csv",
		"delimiter":      string(delimiter),
		"headers":        headers,
		"rows":           previewRows,
		"rawRows":        rows,
		"rowCount":       len(rows),
		"maxRows":        maxRows,
		"truncated":      truncatedByBytes || truncatedByRows,
		"sizeBytes":      rec.SizeBytes,
		"downloadURL":    downloadURL,
		"encoding":       "utf-8",
		"limitBytes":     limit,
		"truncatedBytes": truncatedByBytes,
		"truncatedRows":  truncatedByRows,
	})
}

func (h Handler) publicShareFileStream(w http.ResponseWriter, r *http.Request, inline bool) {
	if !h.enforceRateLimit(w, r, "share_access_"+chi.URLParam(r, "token"), h.Cfg.ShareRatePerMin) {
		return
	}

	password := getPublicSharePassword(r)
	share, err := h.resolvePublicShare(r, password, true)
	if err != nil {
		h.publicShareErr(w, err)
		return
	}

	sharingCfg, err := h.getSharingRuntimeSettings(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not load sharing settings"})
		return
	}
	if inline && !sharingCfg.PublicPreviewEnabled {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "public preview is disabled by admin settings"})
		return
	}
	if !inline && !sharingCfg.PublicDownloadEnable {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "public download is disabled by admin settings"})
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

	storageKey := rec.StorageKey
	lastModified := rec.UpdatedAt

	if inline {
		previewCfg, cfgErr := h.getPreviewRuntimeSettings(r.Context())
		if cfgErr != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not load preview settings"})
			return
		}
		category, method, _ := detectPreviewMode(rec)
		if allowed, reason := previewAllowedByConfig(previewCfg, category, true); !allowed {
			if reason == "" {
				reason = "public preview is disabled by admin settings"
			}
			writeJSON(w, http.StatusForbidden, map[string]string{"error": reason})
			return
		}

		variant := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("variant")))
		if variant == "" && method == "text_partial" && isRiskyInlinePreviewType(rec) {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "inline preview is blocked for active content; use preview-content endpoint"})
			return
		}
		if variant == "" && category == "office" {
			variant = "pdf"
		}
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
				rec.MimeType = previewRec.MimeType
			}
		}
	}

	if share.MaxDownloads != nil && share.DownloadCount >= *share.MaxDownloads {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "share download limit exceeded"})
		return
	}

	obj, err := h.Storage.Stat(r.Context(), storageKey)
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
	w.Header().Set("Last-Modified", lastModified.UTC().Format(http.TimeFormat))
	if inline {
		w.Header().Set("Content-Disposition", fmt.Sprintf("inline; filename=%q", rec.OriginalName))
		w.Header().Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'; sandbox")
	} else {
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", rec.OriginalName))
	}

	rangeHeader := strings.TrimSpace(r.Header.Get("Range"))
	if rangeHeader == "" {
		w.Header().Set("Content-Length", fmt.Sprintf("%d", obj.Size))
		stream, err := h.Storage.GetStream(r.Context(), storageKey)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not open file"})
			return
		}
		defer stream.Close()
		w.WriteHeader(http.StatusOK)
		_, _ = io.Copy(w, stream)
		if inline {
			h.insertAudit(r.Context(), nil, "share.public.file.preview", "share", &share.ID, clientIP(r), r.UserAgent(), map[string]any{
				"fileId": fileID,
			})
		} else {
			h.insertAudit(r.Context(), nil, "share.public.file.download", "share", &share.ID, clientIP(r), r.UserAgent(), map[string]any{
				"fileId": fileID,
			})
			h.bumpShareDownloadCount(r, share)
		}
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
	w.Header().Set("Content-Length", fmt.Sprintf("%d", length))
	w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, obj.Size))
	w.WriteHeader(http.StatusPartialContent)
	_, _ = io.Copy(w, stream)
	if inline {
		h.insertAudit(r.Context(), nil, "share.public.file.preview", "share", &share.ID, clientIP(r), r.UserAgent(), map[string]any{
			"fileId": fileID,
			"range":  true,
		})
	} else {
		h.insertAudit(r.Context(), nil, "share.public.file.download", "share", &share.ID, clientIP(r), r.UserAgent(), map[string]any{
			"fileId": fileID,
			"range":  true,
		})
		h.bumpShareDownloadCount(r, share)
	}
}

func (h Handler) publicShareFolderZip(w http.ResponseWriter, r *http.Request) {
	if !h.enforceRateLimit(w, r, "share_access_"+chi.URLParam(r, "token"), h.Cfg.ShareRatePerMin) {
		return
	}
	if !h.enforceRateLimitWithSubject(w, r, "zip_download_public", chi.URLParam(r, "token"), h.Cfg.ZipDownloadRatePerMin) {
		return
	}

	password := getPublicSharePassword(r)
	share, err := h.resolvePublicShare(r, password, true)
	if err != nil {
		h.publicShareErr(w, err)
		return
	}
	sharingCfg, err := h.getSharingRuntimeSettings(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not load sharing settings"})
		return
	}
	if !sharingCfg.PublicDownloadEnable {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "public download is disabled by admin settings"})
		return
	}
	if share.TargetType != "folder" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "share target is not a folder"})
		return
	}
	if !share.AllowDownload {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "download is disabled for this share"})
		return
	}

	folderID := chi.URLParam(r, "folderId")
	if _, err := uuid.Parse(folderID); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid folder id"})
		return
	}

	var inTree bool
	err = h.DB.QueryRow(r.Context(), `
		WITH RECURSIVE tree AS (
			SELECT id FROM folders WHERE id=$1 AND owner_id=$2 AND deleted_at IS NULL
			UNION ALL
			SELECT f.id FROM folders f JOIN tree t ON f.parent_id=t.id WHERE f.owner_id=$2 AND f.deleted_at IS NULL
		)
		SELECT EXISTS(SELECT 1 FROM tree WHERE id=$3)
	`, share.TargetID, share.OwnerID, folderID).Scan(&inTree)
	if err != nil || !inTree {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "folder is outside the shared folder boundary"})
		return
	}

	var folderName string
	if err := h.DB.QueryRow(r.Context(), `SELECT name FROM folders WHERE id=$1 AND owner_id=$2 AND deleted_at IS NULL`, folderID, share.OwnerID).Scan(&folderName); err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "folder not found"})
		return
	}

	h.streamFolderZip(w, r, folderID, share.OwnerID, normalizeNodeName(folderName)+".zip")
	h.insertAudit(r.Context(), nil, "share.public.folder.download_zip", "share", &share.ID, clientIP(r), r.UserAgent(), map[string]any{
		"folderId": folderID,
	})
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
	sharingCfg, err := h.getSharingRuntimeSettings(r.Context())
	if err != nil {
		return shareRecord{}, err
	}
	if !sharingCfg.Enabled {
		return shareRecord{}, fmt.Errorf("sharing_disabled")
	}

	rawToken := strings.TrimSpace(chi.URLParam(r, "token"))
	if rawToken == "" {
		return shareRecord{}, errors.New("invalid share token")
	}

	tokenHash := auth.HashToken(rawToken)
	var rec shareRecord
	var expiresAt sql.NullTime
	var passwordHash sql.NullString
	var maxDownloads sql.NullInt32
	err = h.DB.QueryRow(r.Context(), `
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
			h.insertAudit(r.Context(), nil, "share.public.password.failed", "share", &rec.ID, clientIP(r), r.UserAgent(), map[string]any{})
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
	case "sharing_disabled":
		code = http.StatusForbidden
		message = "public sharing is disabled by admin settings"
	}
	writeJSON(w, code, map[string]string{"error": message})
}

func (h Handler) enforceSharePolicyForCreate(r *http.Request, allowPreview bool, allowDownload bool) error {
	sharingCfg, err := h.getSharingRuntimeSettings(r.Context())
	if err != nil {
		return fmt.Errorf("could not load sharing settings")
	}
	if !sharingCfg.Enabled {
		return fmt.Errorf("public sharing is disabled by admin settings")
	}
	if allowDownload && !sharingCfg.PublicDownloadEnable {
		return fmt.Errorf("public download is disabled by admin settings")
	}

	previewCfg, err := h.getPreviewRuntimeSettings(r.Context())
	if err != nil {
		return fmt.Errorf("could not load preview settings")
	}
	if allowPreview {
		if !sharingCfg.PublicPreviewEnabled {
			return fmt.Errorf("public preview is disabled by admin settings")
		}
		if !previewCfg.Enabled || !previewCfg.PublicPreviewEnabled {
			return fmt.Errorf("preview is disabled by admin settings")
		}
	}
	return nil
}

func (h Handler) bumpShareDownloadCount(r *http.Request, share shareRecord) {
	_, _ = h.DB.Exec(r.Context(), `UPDATE share_links SET download_count = download_count + 1, updated_at=now() WHERE id=$1`, share.ID)
}

func (h Handler) fetchOwnedShare(r *http.Request, ownerID string, shareID string) (shareRecord, error) {
	var rec shareRecord
	var expiresAt sql.NullTime
	var passwordHash sql.NullString
	var maxDownloads sql.NullInt32
	err := h.DB.QueryRow(r.Context(), `
		SELECT id, owner_id, target_type, target_id, token_hash, password_hash, expires_at, allow_preview, allow_download, allow_folder_browse, max_downloads, download_count, is_revoked, created_at
		FROM share_links
		WHERE id=$1 AND owner_id=$2
	`, shareID, ownerID).Scan(
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
	return rec, nil
}
