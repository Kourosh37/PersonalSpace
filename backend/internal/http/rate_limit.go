package httpapi

import (
	"fmt"
	"net/http"
	"strings"
	"time"
)

func (h Handler) enforceRateLimit(w http.ResponseWriter, r *http.Request, bucket string, perMinute int) bool {
	if h.RateLimiter == nil || perMinute <= 0 {
		return true
	}

	ip := strings.ReplaceAll(clientIP(r), ":", "_")
	key := fmt.Sprintf("ratelimit:%s:%s", bucket, ip)
	allowed, retryAfter, err := h.RateLimiter.Allow(r.Context(), key, int64(perMinute), time.Minute)
	if err != nil {
		return true
	}
	if !allowed {
		secs := int(retryAfter.Seconds())
		if secs < 1 {
			secs = 1
		}
		w.Header().Set("Retry-After", fmt.Sprintf("%d", secs))
		writeJSON(w, http.StatusTooManyRequests, map[string]string{"error": "too many requests, try again later"})
		return false
	}
	return true
}