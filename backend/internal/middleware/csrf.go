package middleware

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
)

type CSRFMiddleware struct {
	Disabled       bool
	SessionCookie  string
	AllowedOrigins map[string]struct{}
}

func NewCSRFMiddleware(disabled bool, sessionCookie string, baseURL string, extraOrigins []string) CSRFMiddleware {
	allowed := map[string]struct{}{}
	addAllowedOrigin(allowed, baseURL)
	for _, origin := range extraOrigins {
		addAllowedOrigin(allowed, origin)
	}
	return CSRFMiddleware{
		Disabled:       disabled,
		SessionCookie:  sessionCookie,
		AllowedOrigins: allowed,
	}
}

func (m CSRFMiddleware) Handler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if m.Disabled || isSafeMethod(r.Method) || !strings.HasPrefix(r.URL.Path, "/api/") {
			next.ServeHTTP(w, r)
			return
		}

		// CSRF risk exists when browser sends session cookies on state-changing requests.
		cookie, err := r.Cookie(m.SessionCookie)
		if err != nil || strings.TrimSpace(cookie.Value) == "" {
			next.ServeHTTP(w, r)
			return
		}

		origin := normalizeOrigin(r.Header.Get("Origin"))
		if origin == "" {
			origin = originFromReferer(r.Header.Get("Referer"))
		}
		if origin == "" {
			writeCSRFError(w, "missing origin")
			return
		}
		if _, ok := m.AllowedOrigins[origin]; !ok {
			writeCSRFError(w, "origin is not allowed")
			return
		}

		next.ServeHTTP(w, r)
	})
}

func isSafeMethod(method string) bool {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodOptions, http.MethodTrace:
		return true
	default:
		return false
	}
}

func normalizeOrigin(value string) string {
	value = strings.TrimSpace(value)
	if value == "" || value == "null" {
		return ""
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return ""
	}
	return strings.ToLower(parsed.Scheme + "://" + parsed.Host)
}

func originFromReferer(referer string) string {
	referer = strings.TrimSpace(referer)
	if referer == "" {
		return ""
	}
	parsed, err := url.Parse(referer)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return ""
	}
	return strings.ToLower(parsed.Scheme + "://" + parsed.Host)
}

func addAllowedOrigin(dst map[string]struct{}, raw string) {
	origin := normalizeOrigin(raw)
	if origin == "" {
		return
	}
	dst[origin] = struct{}{}
}

func writeCSRFError(w http.ResponseWriter, detail string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusForbidden)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"error":  "request blocked by csrf protection",
		"detail": detail,
	})
}
