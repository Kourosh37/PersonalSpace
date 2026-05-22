package middleware

import (
	"log/slog"
	"net/http"
	"time"

	"space/backend/internal/observability"

	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
)

func StructuredRequestLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := chimiddleware.NewWrapResponseWriter(w, r.ProtoMajor)

		next.ServeHTTP(rec, r)

		status := rec.Status()
		if status == 0 {
			status = http.StatusOK
		}
		routePattern := chi.RouteContext(r.Context()).RoutePattern()
		durationMs := time.Since(start).Milliseconds()
		observability.RecordHTTPRequest(r.Method, routePattern, status, durationMs)

		level := slog.LevelInfo
		if status >= 500 {
			level = slog.LevelError
		} else if status >= 400 {
			level = slog.LevelWarn
		}

		slog.LogAttrs(
			r.Context(),
			level,
			"http_request",
			slog.String("request_id", chimiddleware.GetReqID(r.Context())),
			slog.String("method", r.Method),
			slog.String("path", r.URL.Path),
			slog.String("query", r.URL.RawQuery),
			slog.Int("status", status),
			slog.Int("bytes", rec.BytesWritten()),
			slog.Int64("duration_ms", durationMs),
			slog.String("remote_addr", r.RemoteAddr),
			slog.String("user_agent", r.UserAgent()),
		)
	})
}
