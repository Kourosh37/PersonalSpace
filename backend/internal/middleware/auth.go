package middleware

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"space/backend/internal/auth"
	"space/backend/internal/model"

	"github.com/jackc/pgx/v5/pgxpool"
)

type userCtxKey struct{}

type AuthMiddleware struct {
	DB                *pgxpool.Pool
	SessionCookieName string
}

func (m AuthMiddleware) RequireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, err := r.Cookie(m.SessionCookieName)
		if err != nil || raw.Value == "" {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "authentication required"})
			return
		}

		tokenHash := auth.HashToken(raw.Value)

		var user model.User
		err = m.DB.QueryRow(r.Context(), `
			SELECT u.id, u.username, u.role, u.is_active, u.storage_quota_bytes, u.used_storage_bytes, u.created_at, u.updated_at
			FROM sessions s
			JOIN users u ON u.id = s.user_id
			WHERE s.token_hash = $1 AND s.expires_at > $2 AND u.is_active = true
		`, tokenHash, time.Now().UTC()).Scan(
			&user.ID,
			&user.Username,
			&user.Role,
			&user.IsActive,
			&user.StorageQuota,
			&user.UsedStorageBytes,
			&user.CreatedAt,
			&user.UpdatedAt,
		)
		if err != nil {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid session"})
			return
		}

		ctx := context.WithValue(r.Context(), userCtxKey{}, user)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (m AuthMiddleware) RequireAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, ok := CurrentUser(r.Context())
		if !ok {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "authentication required"})
			return
		}
		if user.Role != "admin" {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "admin role required"})
			return
		}
		next.ServeHTTP(w, r)
	})
}

func CurrentUser(ctx context.Context) (model.User, bool) {
	user, ok := ctx.Value(userCtxKey{}).(model.User)
	return user, ok
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}