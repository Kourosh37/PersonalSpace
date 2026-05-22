package observability

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/jackc/pgx/v5/pgxpool"
)

var uploadBytesTotal atomic.Int64
var previewJobFailuresTotal atomic.Int64

var httpMetrics = struct {
	sync.Mutex
	Requests map[string]*atomic.Int64
	Latency  map[string]*atomic.Int64
}{
	Requests: map[string]*atomic.Int64{},
	Latency:  map[string]*atomic.Int64{},
}

func AddUploadedBytes(bytes int64) {
	if bytes > 0 {
		uploadBytesTotal.Add(bytes)
	}
}

func IncPreviewJobFailure() {
	previewJobFailuresTotal.Add(1)
}

func RecordHTTPRequest(method string, route string, status int, durationMs int64) {
	if route == "" {
		route = "unknown"
	}
	statusClass := strconv.Itoa(status/100) + "xx"
	key := strings.Join([]string{method, route, statusClass}, "\xff")

	httpMetrics.Lock()
	requestCounter := httpMetrics.Requests[key]
	if requestCounter == nil {
		requestCounter = &atomic.Int64{}
		httpMetrics.Requests[key] = requestCounter
	}
	latencyCounter := httpMetrics.Latency[key]
	if latencyCounter == nil {
		latencyCounter = &atomic.Int64{}
		httpMetrics.Latency[key] = latencyCounter
	}
	httpMetrics.Unlock()

	requestCounter.Add(1)
	if durationMs > 0 {
		latencyCounter.Add(durationMs)
	}
}

func WritePrometheus(ctx context.Context, w http.ResponseWriter, db *pgxpool.Pool) {
	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")

	fmt.Fprintln(w, "# HELP space_uploaded_bytes_total Total successfully persisted upload bytes.")
	fmt.Fprintln(w, "# TYPE space_uploaded_bytes_total counter")
	fmt.Fprintf(w, "space_uploaded_bytes_total %d\n", uploadBytesTotal.Load())

	fmt.Fprintln(w, "# HELP space_preview_job_failures_total Total preview job processing failures.")
	fmt.Fprintln(w, "# TYPE space_preview_job_failures_total counter")
	fmt.Fprintf(w, "space_preview_job_failures_total %d\n", previewJobFailuresTotal.Load())

	writeHTTPMetrics(w)
	writeDBMetrics(ctx, w, db)
}

func writeHTTPMetrics(w http.ResponseWriter) {
	httpMetrics.Lock()
	requestKeys := make([]string, 0, len(httpMetrics.Requests))
	for key := range httpMetrics.Requests {
		requestKeys = append(requestKeys, key)
	}
	sort.Strings(requestKeys)
	httpMetrics.Unlock()

	fmt.Fprintln(w, "# HELP space_http_requests_total HTTP requests by route and status class.")
	fmt.Fprintln(w, "# TYPE space_http_requests_total counter")
	fmt.Fprintln(w, "# HELP space_http_request_duration_ms_total Aggregate HTTP request duration in milliseconds.")
	fmt.Fprintln(w, "# TYPE space_http_request_duration_ms_total counter")

	for _, key := range requestKeys {
		parts := strings.Split(key, "\xff")
		if len(parts) != 3 {
			continue
		}
		method, route, statusClass := parts[0], parts[1], parts[2]

		httpMetrics.Lock()
		requests := httpMetrics.Requests[key].Load()
		latency := httpMetrics.Latency[key].Load()
		httpMetrics.Unlock()

		labels := fmt.Sprintf(`method=%q,route=%q,status_class=%q`, method, route, statusClass)
		fmt.Fprintf(w, "space_http_requests_total{%s} %d\n", labels, requests)
		fmt.Fprintf(w, "space_http_request_duration_ms_total{%s} %d\n", labels, latency)
	}
}

func writeDBMetrics(ctx context.Context, w http.ResponseWriter, db *pgxpool.Pool) {
	if db == nil {
		return
	}

	var queuedPreviewJobs int64
	var processingPreviewJobs int64
	var failedPreviewJobs int64
	_ = db.QueryRow(ctx, `SELECT COUNT(*) FROM preview_jobs WHERE status='queued'`).Scan(&queuedPreviewJobs)
	_ = db.QueryRow(ctx, `SELECT COUNT(*) FROM preview_jobs WHERE status='processing'`).Scan(&processingPreviewJobs)
	_ = db.QueryRow(ctx, `SELECT COUNT(*) FROM preview_jobs WHERE status='failed'`).Scan(&failedPreviewJobs)

	fmt.Fprintln(w, "# HELP space_preview_jobs Number of preview jobs by status.")
	fmt.Fprintln(w, "# TYPE space_preview_jobs gauge")
	fmt.Fprintf(w, "space_preview_jobs{status=%q} %d\n", "queued", queuedPreviewJobs)
	fmt.Fprintf(w, "space_preview_jobs{status=%q} %d\n", "processing", processingPreviewJobs)
	fmt.Fprintf(w, "space_preview_jobs{status=%q} %d\n", "failed", failedPreviewJobs)

	var activeUploadSessions int64
	var activeUploadBytes int64
	_ = db.QueryRow(ctx, `
		SELECT COUNT(*), COALESCE(SUM(uploaded_bytes),0)
		FROM upload_sessions
		WHERE status IN ('initialized','uploading','paused')
	`).Scan(&activeUploadSessions, &activeUploadBytes)

	fmt.Fprintln(w, "# HELP space_active_upload_sessions Active resumable upload sessions.")
	fmt.Fprintln(w, "# TYPE space_active_upload_sessions gauge")
	fmt.Fprintf(w, "space_active_upload_sessions %d\n", activeUploadSessions)

	fmt.Fprintln(w, "# HELP space_active_upload_bytes Uploaded bytes currently held by active resumable sessions.")
	fmt.Fprintln(w, "# TYPE space_active_upload_bytes gauge")
	fmt.Fprintf(w, "space_active_upload_bytes %d\n", activeUploadBytes)
}
