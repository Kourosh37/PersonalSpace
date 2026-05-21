package httpapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"space/backend/internal/middleware"

	"github.com/go-chi/chi/v5"
)

type settingItem struct {
	Key         string          `json:"key"`
	Value       json.RawMessage `json:"value"`
	ValueType   string          `json:"valueType"`
	Description *string         `json:"description,omitempty"`
	IsPublic    bool            `json:"isPublic"`
	UpdatedAt   time.Time       `json:"updatedAt"`
}

type adminPatchSettingsRequest struct {
	Items []adminSettingPatchItem `json:"items"`
}

type adminSettingPatchItem struct {
	Key         string          `json:"key"`
	Value       json.RawMessage `json:"value"`
	ValueType   string          `json:"valueType"`
	Description *string         `json:"description"`
	IsPublic    *bool           `json:"isPublic"`
}

func (h Handler) registerAdminRoutes(admin chi.Router) {
	admin.Get("/settings", h.adminGetSettings)
	admin.Patch("/settings", h.adminPatchSettings)
	admin.Get("/settings/public", h.adminGetPublicSettings)
	admin.Get("/settings/upload", h.getUploadSettings)
	admin.Patch("/settings/upload", h.patchUploadSettings)
	admin.Get("/settings/sharing", h.adminGetSharingSettings)
	admin.Patch("/settings/sharing", h.adminPatchSharingSettings)
	admin.Get("/settings/preview", h.adminGetPreviewSettings)
	admin.Patch("/settings/preview", h.adminPatchPreviewSettings)
	admin.Get("/storage/summary", h.adminStorageSummary)
	admin.Post("/storage/recalculate", h.adminStorageRecalculate)
	admin.Post("/storage/cleanup-expired-uploads", h.adminCleanupExpiredUploads)
	admin.Post("/storage/cleanup-preview-cache", h.adminCleanupPreviewCache)
	admin.Get("/audit-logs", h.adminAuditLogs)
	admin.Get("/system/info", h.adminSystemInfo)
	h.registerAdminUserRoutes(admin)
}

func (h Handler) adminGetSettings(w http.ResponseWriter, r *http.Request) {
	items, err := h.querySettings(r, "")
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not load settings"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (h Handler) adminGetPublicSettings(w http.ResponseWriter, r *http.Request) {
	items, err := h.querySettings(r, "WHERE is_public = true")
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not load public settings"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (h Handler) querySettings(r *http.Request, whereClause string) ([]settingItem, error) {
	query := `
		SELECT key, value, value_type, description, is_public, updated_at
		FROM system_settings
	`
	if whereClause != "" {
		query += " " + whereClause
	}
	query += " ORDER BY key ASC"

	rows, err := h.DB.Query(r.Context(), query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]settingItem, 0, 64)
	for rows.Next() {
		var item settingItem
		if err := rows.Scan(&item.Key, &item.Value, &item.ValueType, &item.Description, &item.IsPublic, &item.UpdatedAt); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

func (h Handler) adminPatchSettings(w http.ResponseWriter, r *http.Request) {
	h.patchSettingsWithPrefix(w, r, "")
}

func (h Handler) adminGetSharingSettings(w http.ResponseWriter, r *http.Request) {
	items, err := h.querySettings(r, "WHERE key LIKE 'sharing.%'")
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not load sharing settings"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (h Handler) adminPatchSharingSettings(w http.ResponseWriter, r *http.Request) {
	h.patchSettingsWithPrefix(w, r, "sharing.")
}

func (h Handler) adminGetPreviewSettings(w http.ResponseWriter, r *http.Request) {
	items, err := h.querySettings(r, "WHERE key LIKE 'preview.%'")
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not load preview settings"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (h Handler) adminPatchPreviewSettings(w http.ResponseWriter, r *http.Request) {
	h.patchSettingsWithPrefix(w, r, "preview.")
}

func (h Handler) patchSettingsWithPrefix(w http.ResponseWriter, r *http.Request, prefix string) {
	user, ok := middleware.CurrentUser(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "authentication required"})
		return
	}

	var req adminPatchSettingsRequest
	if err := ReadJSON(r, &req); err != nil || len(req.Items) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	tx, err := h.DB.Begin(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not begin settings update"})
		return
	}
	defer tx.Rollback(r.Context())

	for _, item := range req.Items {
		key := strings.TrimSpace(item.Key)
		if key == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "setting key is required"})
			return
		}
		if prefix != "" && !strings.HasPrefix(key, prefix) {
			key = prefix + key
		}
		if len(item.Value) == 0 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("setting value is required for %s", key)})
			return
		}
		valueType := strings.TrimSpace(item.ValueType)
		if valueType == "" {
			valueType = "json"
		}
		isPublic := false
		if item.IsPublic != nil {
			isPublic = *item.IsPublic
		}

		_, err := tx.Exec(r.Context(), `
			INSERT INTO system_settings (id, key, value, value_type, description, is_public, created_at, updated_at)
			VALUES (gen_random_uuid(), $1, $2::jsonb, $3, $4, $5, now(), now())
			ON CONFLICT (key)
			DO UPDATE SET value=EXCLUDED.value, value_type=EXCLUDED.value_type, description=EXCLUDED.description, is_public=EXCLUDED.is_public, updated_at=now()
		`, key, []byte(item.Value), valueType, item.Description, isPublic)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("could not upsert setting %s", key)})
			return
		}
	}

	if err := tx.Commit(r.Context()); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not commit settings update"})
		return
	}

	h.insertAudit(r.Context(), &user.ID, "admin.settings.updated", "system_settings", nil, clientIP(r), r.UserAgent(), map[string]any{"count": len(req.Items), "prefix": prefix})
	items, err := h.querySettings(r, "")
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (h Handler) adminStorageSummary(w http.ResponseWriter, r *http.Request) {
	var fileCount int64
	var folderCount int64
	var totalBytes int64
	_ = h.DB.QueryRow(r.Context(), `SELECT COUNT(*) FROM files WHERE deleted_at IS NULL`).Scan(&fileCount)
	_ = h.DB.QueryRow(r.Context(), `SELECT COUNT(*) FROM folders WHERE deleted_at IS NULL`).Scan(&folderCount)
	_ = h.DB.QueryRow(r.Context(), `SELECT COALESCE(SUM(size_bytes),0) FROM files WHERE deleted_at IS NULL`).Scan(&totalBytes)

	var incompleteBytes int64
	_ = h.DB.QueryRow(r.Context(), `SELECT COALESCE(SUM(uploaded_bytes),0) FROM upload_sessions WHERE status IN ('initialized','uploading','paused')`).Scan(&incompleteBytes)

	writeJSON(w, http.StatusOK, map[string]any{
		"fileCount":             fileCount,
		"folderCount":           folderCount,
		"totalUsedStorageBytes": totalBytes,
		"incompleteUploadBytes": incompleteBytes,
		"storageRoot":           h.Cfg.StorageRoot,
	})
}

func (h Handler) adminStorageRecalculate(w http.ResponseWriter, r *http.Request) {
	_, err := h.DB.Exec(r.Context(), `
		UPDATE users u
		SET used_storage_bytes = COALESCE(f.total_bytes, 0), updated_at = now()
		FROM (
			SELECT owner_id, SUM(size_bytes) AS total_bytes
			FROM files
			WHERE deleted_at IS NULL
			GROUP BY owner_id
		) f
		WHERE u.id = f.owner_id
	`)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not recalculate storage usage"})
		return
	}

	_, _ = h.DB.Exec(r.Context(), `
		UPDATE users
		SET used_storage_bytes = 0, updated_at = now()
		WHERE id NOT IN (SELECT DISTINCT owner_id FROM files WHERE deleted_at IS NULL)
	`)

	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (h Handler) adminCleanupExpiredUploads(w http.ResponseWriter, r *http.Request) {
	rows, err := h.DB.Query(r.Context(), `
		SELECT id, storage_key_temp
		FROM upload_sessions
		WHERE status IN ('initialized','uploading','paused')
		  AND expires_at IS NOT NULL
		  AND expires_at < now()
	`)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not load expired uploads"})
		return
	}
	defer rows.Close()

	cleaned := 0
	for rows.Next() {
		var id string
		var key string
		if err := rows.Scan(&id, &key); err != nil {
			continue
		}
		_, _ = h.DB.Exec(r.Context(), `UPDATE upload_sessions SET status='expired', updated_at=now() WHERE id=$1`, id)
		_ = h.Storage.Delete(r.Context(), key)
		cleaned++
	}

	writeJSON(w, http.StatusOK, map[string]any{"cleaned": cleaned})
}

func (h Handler) adminCleanupPreviewCache(w http.ResponseWriter, r *http.Request) {
	previewsPath := filepath.Join(h.Cfg.StorageRoot, "previews")
	if err := os.RemoveAll(previewsPath); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not remove preview cache"})
		return
	}
	if err := os.MkdirAll(previewsPath, 0o755); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not recreate preview cache directory"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (h Handler) adminAuditLogs(w http.ResponseWriter, r *http.Request) {
	limit := 100
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 && parsed <= 500 {
			limit = parsed
		}
	}

	action := strings.TrimSpace(r.URL.Query().Get("action"))
	userID := strings.TrimSpace(r.URL.Query().Get("userId"))
	from := strings.TrimSpace(r.URL.Query().Get("from"))
	to := strings.TrimSpace(r.URL.Query().Get("to"))

	query := `
		SELECT id, user_id, action, target_type, target_id, ip_address, user_agent, metadata, created_at
		FROM audit_logs
		WHERE 1=1
	`
	args := make([]any, 0, 6)
	idx := 1
	if action != "" {
		query += fmt.Sprintf(" AND action = $%d", idx)
		args = append(args, action)
		idx++
	}
	if userID != "" {
		query += fmt.Sprintf(" AND user_id = $%d", idx)
		args = append(args, userID)
		idx++
	}
	if from != "" {
		query += fmt.Sprintf(" AND created_at >= $%d", idx)
		args = append(args, from)
		idx++
	}
	if to != "" {
		query += fmt.Sprintf(" AND created_at <= $%d", idx)
		args = append(args, to)
		idx++
	}
	query += fmt.Sprintf(" ORDER BY created_at DESC LIMIT $%d", idx)
	args = append(args, limit)

	rows, err := h.DB.Query(r.Context(), query, args...)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not load audit logs"})
		return
	}
	defer rows.Close()

	items := make([]map[string]any, 0, limit)
	for rows.Next() {
		var id string
		var uid *string
		var logAction string
		var targetType *string
		var targetID *string
		var ip *string
		var ua *string
		var metadata json.RawMessage
		var createdAt time.Time
		if err := rows.Scan(&id, &uid, &logAction, &targetType, &targetID, &ip, &ua, &metadata, &createdAt); err != nil {
			continue
		}
		items = append(items, map[string]any{
			"id":         id,
			"userId":     uid,
			"action":     logAction,
			"targetType": targetType,
			"targetId":   targetID,
			"ipAddress":  ip,
			"userAgent":  ua,
			"metadata":   metadata,
			"createdAt":  createdAt,
		})
	}

	writeJSON(w, http.StatusOK, map[string]any{"items": items, "limit": limit})
}

func (h Handler) adminSystemInfo(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"appName":       h.Cfg.AppName,
		"publicBaseURL": h.Cfg.PublicBaseURL,
		"httpAddr":      h.Cfg.HTTPAddr,
		"storageRoot":   h.Cfg.StorageRoot,
		"timeUTC":       time.Now().UTC(),
	})
}
