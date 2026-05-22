package observability

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestWritePrometheusIncludesRuntimeMetrics(t *testing.T) {
	AddUploadedBytes(123)
	IncPreviewJobFailure()
	RecordHTTPRequest("GET", "/healthz", 200, 12)

	rec := httptest.NewRecorder()
	WritePrometheus(t.Context(), rec, nil)
	body := rec.Body.String()

	for _, expected := range []string{
		"space_uploaded_bytes_total",
		"space_preview_job_failures_total",
		`space_http_requests_total{method="GET",route="/healthz",status_class="2xx"}`,
		"space_http_request_duration_ms_total",
	} {
		if !strings.Contains(body, expected) {
			t.Fatalf("metrics output missing %q:\n%s", expected, body)
		}
	}
}
