package httpapi

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"space/backend/internal/auth"
	"space/backend/internal/middleware"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type adminUserDTO struct {
	ID               string    `json:"id"`
	Username         string    `json:"username"`
	Role             string    `json:"role"`
	IsActive         bool      `json:"isActive"`
	StorageQuota     *int64    `json:"storageQuotaBytes,omitempty"`
	UsedStorageBytes int64     `json:"usedStorageBytes"`
	CreatedAt        time.Time `json:"createdAt"`
	UpdatedAt        time.Time `json:"updatedAt"`
}

func (h Handler) registerAdminUserRoutes(admin chi.Router) {
	admin.Get("/users", h.adminGetUsers)
	admin.Post("/users", h.adminCreateUser)
	admin.Get("/users/{id}", h.adminGetUser)
	admin.Patch("/users/{id}", h.adminPatchUser)
	admin.Delete("/users/{id}", h.adminDeleteUser)
	admin.Post("/users/{id}/change-password", h.adminChangeUserPassword)
}

func (h Handler) adminGetUsers(w http.ResponseWriter, r *http.Request) {
	rows, err := h.DB.Query(r.Context(), `
		SELECT id, username, role, is_active, storage_quota_bytes, used_storage_bytes, created_at, updated_at
		FROM users
		ORDER BY created_at DESC
	`)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not load users"})
		return
	}
	defer rows.Close()

	items := make([]adminUserDTO, 0, 64)
	for rows.Next() {
		var item adminUserDTO
		if err := rows.Scan(&item.ID, &item.Username, &item.Role, &item.IsActive, &item.StorageQuota, &item.UsedStorageBytes, &item.CreatedAt, &item.UpdatedAt); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not decode users"})
			return
		}
		items = append(items, item)
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

type adminCreateUserRequest struct {
	Username         string `json:"username"`
	Password         string `json:"password"`
	Role             string `json:"role"`
	StorageQuota     *int64 `json:"storageQuotaBytes"`
	IsActive         *bool  `json:"isActive"`
}

func (h Handler) adminCreateUser(w http.ResponseWriter, r *http.Request) {
	adminUser, ok := currentAdminUser(r)
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "authentication required"})
		return
	}

	var req adminCreateUserRequest
	if err := ReadJSON(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	username := strings.TrimSpace(req.Username)
	password := strings.TrimSpace(req.Password)
	role := strings.TrimSpace(req.Role)
	if username == "" || password == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "username and password are required"})
		return
	}
	if len(password) < 8 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "password must be at least 8 characters"})
		return
	}
	if role == "" {
		role = "user"
	}
	if role != "user" && role != "admin" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "role must be user or admin"})
		return
	}
	if req.StorageQuota != nil && *req.StorageQuota < 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "storageQuotaBytes cannot be negative"})
		return
	}

	isActive := true
	if req.IsActive != nil {
		isActive = *req.IsActive
	}

	hash, err := auth.HashPassword(password)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not hash password"})
		return
	}

	id := uuid.NewString()
	now := time.Now().UTC()
	_, err = h.DB.Exec(r.Context(), `
		INSERT INTO users (id, username, password_hash, role, is_active, storage_quota_bytes, used_storage_bytes, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,0,$7,$8)
	`, id, username, hash, role, isActive, req.StorageQuota, now, now)
	if err != nil {
		if isUniqueViolation(err) {
			writeJSON(w, http.StatusConflict, map[string]string{"error": "username already exists"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not create user"})
		return
	}

	h.insertAudit(r.Context(), &adminUser.ID, "admin.user.created", "user", &id, clientIP(r), r.UserAgent(), map[string]any{"username": username, "role": role})
	writeJSON(w, http.StatusCreated, map[string]any{"id": id, "username": username, "role": role, "isActive": isActive})
}

func (h Handler) adminGetUser(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if _, err := uuid.Parse(id); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid user id"})
		return
	}

	user, err := h.fetchUserByID(r, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "user not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not load user"})
		return
	}
	writeJSON(w, http.StatusOK, user)
}

type adminPatchUserRequest struct {
	Username     *string `json:"username"`
	Role         *string `json:"role"`
	IsActive     *bool   `json:"isActive"`
	StorageQuota *int64  `json:"storageQuotaBytes"`
}

func (h Handler) adminPatchUser(w http.ResponseWriter, r *http.Request) {
	adminUser, ok := currentAdminUser(r)
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "authentication required"})
		return
	}

	id := chi.URLParam(r, "id")
	if _, err := uuid.Parse(id); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid user id"})
		return
	}

	var req adminPatchUserRequest
	if err := ReadJSON(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	current, err := h.fetchUserByID(r, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "user not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not load user"})
		return
	}

	username := current.Username
	role := current.Role
	isActive := current.IsActive
	quota := current.StorageQuota

	if req.Username != nil {
		val := strings.TrimSpace(*req.Username)
		if val == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "username cannot be empty"})
			return
		}
		username = val
	}
	if req.Role != nil {
		val := strings.TrimSpace(*req.Role)
		if val != "user" && val != "admin" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "role must be user or admin"})
			return
		}
		role = val
	}
	if req.IsActive != nil {
		isActive = *req.IsActive
	}
	if req.StorageQuota != nil {
		if *req.StorageQuota < 0 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "storageQuotaBytes cannot be negative"})
			return
		}
		quota = req.StorageQuota
	}

	_, err = h.DB.Exec(r.Context(), `
		UPDATE users
		SET username=$1, role=$2, is_active=$3, storage_quota_bytes=$4, updated_at=now()
		WHERE id=$5
	`, username, role, isActive, quota, id)
	if err != nil {
		if isUniqueViolation(err) {
			writeJSON(w, http.StatusConflict, map[string]string{"error": "username already exists"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not update user"})
		return
	}

	if !isActive {
		_, _ = h.DB.Exec(r.Context(), `DELETE FROM sessions WHERE user_id=$1`, id)
	}

	h.insertAudit(r.Context(), &adminUser.ID, "admin.user.updated", "user", &id, clientIP(r), r.UserAgent(), map[string]any{"username": username, "role": role, "isActive": isActive})
	updated, _ := h.fetchUserByID(r, id)
	writeJSON(w, http.StatusOK, updated)
}

func (h Handler) adminDeleteUser(w http.ResponseWriter, r *http.Request) {
	adminUser, ok := currentAdminUser(r)
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "authentication required"})
		return
	}

	id := chi.URLParam(r, "id")
	if _, err := uuid.Parse(id); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid user id"})
		return
	}
	if id == adminUser.ID {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "admin cannot deactivate itself"})
		return
	}

	cmd, err := h.DB.Exec(r.Context(), `UPDATE users SET is_active=false, updated_at=now() WHERE id=$1`, id)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not deactivate user"})
		return
	}
	if cmd.RowsAffected() == 0 {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "user not found"})
		return
	}

	_, _ = h.DB.Exec(r.Context(), `DELETE FROM sessions WHERE user_id=$1`, id)
	h.insertAudit(r.Context(), &adminUser.ID, "admin.user.deactivated", "user", &id, clientIP(r), r.UserAgent(), map[string]any{})
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

type adminChangeUserPasswordRequest struct {
	NewPassword string `json:"newPassword"`
}

func (h Handler) adminChangeUserPassword(w http.ResponseWriter, r *http.Request) {
	adminUser, ok := currentAdminUser(r)
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "authentication required"})
		return
	}

	id := chi.URLParam(r, "id")
	if _, err := uuid.Parse(id); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid user id"})
		return
	}

	var req adminChangeUserPasswordRequest
	if err := ReadJSON(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	newPassword := strings.TrimSpace(req.NewPassword)
	if len(newPassword) < 8 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "newPassword must be at least 8 characters"})
		return
	}

	hash, err := auth.HashPassword(newPassword)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not hash password"})
		return
	}

	cmd, err := h.DB.Exec(r.Context(), `UPDATE users SET password_hash=$1, updated_at=now() WHERE id=$2`, hash, id)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not update password"})
		return
	}
	if cmd.RowsAffected() == 0 {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "user not found"})
		return
	}

	_, _ = h.DB.Exec(r.Context(), `DELETE FROM sessions WHERE user_id=$1`, id)
	h.insertAudit(r.Context(), &adminUser.ID, "admin.user.password.changed", "user", &id, clientIP(r), r.UserAgent(), map[string]any{})
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (h Handler) fetchUserByID(r *http.Request, id string) (adminUserDTO, error) {
	var user adminUserDTO
	err := h.DB.QueryRow(r.Context(), `
		SELECT id, username, role, is_active, storage_quota_bytes, used_storage_bytes, created_at, updated_at
		FROM users WHERE id=$1
	`, id).Scan(&user.ID, &user.Username, &user.Role, &user.IsActive, &user.StorageQuota, &user.UsedStorageBytes, &user.CreatedAt, &user.UpdatedAt)
	if err != nil {
		return adminUserDTO{}, err
	}
	return user, nil
}

func currentAdminUser(r *http.Request) (adminUserDTO, bool) {
	u, ok := middleware.CurrentUser(r.Context())
	if !ok {
		return adminUserDTO{}, false
	}
	return adminUserDTO{ID: u.ID, Username: u.Username, Role: u.Role, IsActive: u.IsActive, StorageQuota: u.StorageQuota, UsedStorageBytes: u.UsedStorageBytes, CreatedAt: u.CreatedAt, UpdatedAt: u.UpdatedAt}, true
}
