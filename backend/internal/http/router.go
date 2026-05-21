package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"strings"
	"time"

	"space/backend/internal/auth"
	"space/backend/internal/config"
	"space/backend/internal/middleware"
	"space/backend/internal/settings"
	"space/backend/internal/storage"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Handler struct {
	DB      *pgxpool.Pool
	Cfg     config.Config
	Storage storage.Interface
}

func (h Handler) Router() http.Handler {
	r := chi.NewRouter()
	authMW := middleware.AuthMiddleware{DB: h.DB, SessionCookieName: h.Cfg.SessionCookieName}

	r.Get("/healthz", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})

	r.Route("/api", func(api chi.Router) {
		api.Route("/auth", func(ar chi.Router) {
			ar.Post("/login", h.login)
			ar.Post("/logout", h.logout)
			ar.With(authMW.RequireAuth).Get("/me", h.me)
			ar.With(authMW.RequireAuth).Post("/change-password", h.changeOwnPassword)
		})

		h.registerFolderRoutes(api, authMW)
		h.registerFileRoutes(api, authMW)
		h.registerUploadRoutes(api, authMW)
		h.registerShareRoutes(api, authMW)

		api.With(authMW.RequireAuth, authMW.RequireAdmin).Route("/admin", func(admin chi.Router) {
			admin.Get("/system/health", func(w http.ResponseWriter, r *http.Request) {
				writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
			})
			h.registerAdminRoutes(admin)
		})
	})

	return r
}

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func (h Handler) login(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	req.Username = strings.TrimSpace(req.Username)
	if req.Username == "" || req.Password == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "username and password are required"})
		return
	}

	ctx := r.Context()
	var userID string
	var username string
	var role string
	var passwordHash string
	err := h.DB.QueryRow(ctx, `
		SELECT id, username, role, password_hash
		FROM users
		WHERE username = $1 AND is_active = true
	`, req.Username).Scan(&userID, &username, &role, &passwordHash)
	if err != nil {
		h.insertAudit(ctx, nil, "auth.login.failed", "user", nil, clientIP(r), r.UserAgent(), map[string]any{"username": req.Username})
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid credentials"})
		return
	}

	if !auth.VerifyPassword(passwordHash, req.Password) {
		h.insertAudit(ctx, &userID, "auth.login.failed", "user", nil, clientIP(r), r.UserAgent(), map[string]any{"username": req.Username})
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid credentials"})
		return
	}

	rawToken, tokenHash, err := auth.NewSessionToken()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not create session"})
		return
	}

	expiresAt := time.Now().UTC().Add(h.Cfg.SessionTTL)
	_, err = h.DB.Exec(ctx, `
		INSERT INTO sessions (id, user_id, token_hash, expires_at, created_at, last_seen_at, ip_address, user_agent)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
	`, uuid.NewString(), userID, tokenHash, expiresAt, time.Now().UTC(), time.Now().UTC(), clientIP(r), r.UserAgent())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not store session"})
		return
	}

	h.insertAudit(ctx, &userID, "auth.login.success", "user", &userID, clientIP(r), r.UserAgent(), map[string]any{"username": username})

	http.SetCookie(w, &http.Cookie{
		Name:     h.Cfg.SessionCookieName,
		Value:    rawToken,
		Path:     "/",
		HttpOnly: true,
		Secure:   h.Cfg.SessionSecure,
		SameSite: http.SameSiteLaxMode,
		Expires:  expiresAt,
	})

	writeJSON(w, http.StatusOK, map[string]any{"user": map[string]string{"id": userID, "username": username, "role": role}})
}

func (h Handler) logout(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie(h.Cfg.SessionCookieName)
	if err == nil && cookie.Value != "" {
		tokenHash := auth.HashToken(cookie.Value)
		_, _ = h.DB.Exec(r.Context(), `DELETE FROM sessions WHERE token_hash=$1`, tokenHash)
	}

	http.SetCookie(w, &http.Cookie{
		Name:     h.Cfg.SessionCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   h.Cfg.SessionSecure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	})
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (h Handler) me(w http.ResponseWriter, r *http.Request) {
	user, ok := middleware.CurrentUser(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "authentication required"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"user": map[string]any{
			"id":               user.ID,
			"username":         user.Username,
			"role":             user.Role,
			"isActive":         user.IsActive,
			"usedStorageBytes": user.UsedStorageBytes,
		},
	})
}

func (h Handler) getUploadSettings(w http.ResponseWriter, r *http.Request) {
	current, err := settings.GetUploadSettings(r.Context(), h.DB)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not load settings"})
		return
	}
	writeJSON(w, http.StatusOK, current)
}

type patchUploadSettingsRequest struct {
	Mode             string `json:"mode"`
	MaxFileSizeBytes *int64 `json:"maxFileSizeBytes"`
}

func (h Handler) patchUploadSettings(w http.ResponseWriter, r *http.Request) {
	var req patchUploadSettingsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	user, ok := middleware.CurrentUser(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "authentication required"})
		return
	}

	payload := settings.UploadSettings{Mode: req.Mode, MaxFileSizeBytes: req.MaxFileSizeBytes}
	if err := settings.UpdateUploadSettings(r.Context(), h.DB, user.ID, payload, clientIP(r), r.UserAgent()); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	updated, err := settings.GetUploadSettings(r.Context(), h.DB)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "saved but could not reload settings"})
		return
	}

	writeJSON(w, http.StatusOK, updated)
}

func (h Handler) insertAudit(ctx context.Context, userID *string, action string, targetType string, targetID *string, ip string, ua string, metadata map[string]any) {
	data, _ := json.Marshal(metadata)
	_, _ = h.DB.Exec(ctx, `
		INSERT INTO audit_logs (id, user_id, action, target_type, target_id, ip_address, user_agent, metadata, created_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
	`, uuid.NewString(), userID, action, targetType, targetID, ip, ua, data, time.Now().UTC())
}

func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		parts := strings.Split(xff, ",")
		if len(parts) > 0 {
			return strings.TrimSpace(parts[0])
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func ReadJSON[T any](r *http.Request, dst *T) error {
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		return err
	}
	if decoder.More() {
		return errors.New("request body must contain a single json object")
	}
	return nil
}
