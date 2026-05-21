package httpapi

import (
	"net/http"
	"strings"

	"space/backend/internal/auth"
	"space/backend/internal/middleware"
)

type changeOwnPasswordRequest struct {
	CurrentPassword string `json:"currentPassword"`
	NewPassword     string `json:"newPassword"`
}

func (h Handler) changeOwnPassword(w http.ResponseWriter, r *http.Request) {
	user, ok := middleware.CurrentUser(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "authentication required"})
		return
	}

	var req changeOwnPasswordRequest
	if err := ReadJSON(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	req.CurrentPassword = strings.TrimSpace(req.CurrentPassword)
	req.NewPassword = strings.TrimSpace(req.NewPassword)
	if req.CurrentPassword == "" || req.NewPassword == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "currentPassword and newPassword are required"})
		return
	}
	if len(req.NewPassword) < 8 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "newPassword must be at least 8 characters"})
		return
	}

	var currentHash string
	err := h.DB.QueryRow(r.Context(), `SELECT password_hash FROM users WHERE id=$1 AND is_active=true`, user.ID).Scan(&currentHash)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid user session"})
		return
	}

	if !auth.VerifyPassword(currentHash, req.CurrentPassword) {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "current password is incorrect"})
		return
	}

	newHash, err := auth.HashPassword(req.NewPassword)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not hash new password"})
		return
	}

	_, err = h.DB.Exec(r.Context(), `UPDATE users SET password_hash=$1, updated_at=now() WHERE id=$2`, newHash, user.ID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not update password"})
		return
	}

	_, _ = h.DB.Exec(r.Context(), `DELETE FROM sessions WHERE user_id=$1`, user.ID)
	h.insertAudit(r.Context(), &user.ID, "auth.password.changed", "user", &user.ID, clientIP(r), r.UserAgent(), map[string]any{})

	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}