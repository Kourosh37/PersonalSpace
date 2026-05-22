package httpapi

import (
	"encoding/base64"
	"testing"
	"time"
)

func strPtr(value string) *string {
	return &value
}

func int64Ptr(value int64) *int64 {
	return &value
}

func TestParseTusMetadata(t *testing.T) {
	t.Parallel()

	raw := "filename " + base64.StdEncoding.EncodeToString([]byte("report.pdf")) +
		",folderid " + base64.RawStdEncoding.EncodeToString([]byte("folder-1")) +
		",broken !!!"

	got := parseTusMetadata(raw)
	if got["filename"] != "report.pdf" {
		t.Fatalf("filename mismatch: %q", got["filename"])
	}
	if got["folderid"] != "folder-1" {
		t.Fatalf("folderid mismatch: %q", got["folderid"])
	}
	if _, ok := got["broken"]; ok {
		t.Fatalf("invalid metadata segment should be ignored")
	}
}

func TestPreviewModeAndPolicy(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		rec           fileRecord
		wantCategory  string
		wantMethod    string
		wantSupported bool
	}{
		{name: "image mime", rec: fileRecord{MimeType: strPtr("image/png")}, wantCategory: "image", wantMethod: "stream", wantSupported: true},
		{name: "pdf mime", rec: fileRecord{MimeType: strPtr("application/pdf")}, wantCategory: "pdf", wantMethod: "stream", wantSupported: true},
		{name: "office extension", rec: fileRecord{Extension: strPtr("docx")}, wantCategory: "office", wantMethod: "async_generated", wantSupported: true},
		{name: "text extension", rec: fileRecord{Extension: strPtr("md")}, wantCategory: "text", wantMethod: "text_partial", wantSupported: true},
		{name: "unknown binary", rec: fileRecord{Extension: strPtr("bin")}, wantCategory: "binary", wantMethod: "unsupported", wantSupported: false},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			category, method, supported := detectPreviewMode(tc.rec)
			if category != tc.wantCategory || method != tc.wantMethod || supported != tc.wantSupported {
				t.Fatalf("got (%s, %s, %v), want (%s, %s, %v)", category, method, supported, tc.wantCategory, tc.wantMethod, tc.wantSupported)
			}
		})
	}

	cfg := previewRuntimeSettings{Enabled: true, PublicPreviewEnabled: false, OfficeEnabled: false, MediaEnabled: false}
	if allowed, _ := previewAllowedByConfig(cfg, "text", false); !allowed {
		t.Fatalf("private text preview should be allowed")
	}
	if allowed, reason := previewAllowedByConfig(cfg, "text", true); allowed || reason == "" {
		t.Fatalf("public preview should be denied with reason")
	}
	if allowed, reason := previewAllowedByConfig(cfg, "video", false); allowed || reason == "" {
		t.Fatalf("media preview should be denied with reason")
	}
}

func TestRiskyInlinePreviewType(t *testing.T) {
	t.Parallel()

	if !isRiskyInlinePreviewType(fileRecord{MimeType: strPtr("text/html")}) {
		t.Fatalf("html mime should be risky")
	}
	if !isRiskyInlinePreviewType(fileRecord{Extension: strPtr("svg")}) {
		t.Fatalf("svg extension should be risky")
	}
	if isRiskyInlinePreviewType(fileRecord{MimeType: strPtr("text/plain"), Extension: strPtr("txt")}) {
		t.Fatalf("plain text should not be risky")
	}
}

func TestFolderHelpers(t *testing.T) {
	t.Parallel()

	if got := normalizeNodeName(` ../bad/name\ `); got != "..badname" {
		t.Fatalf("normalizeNodeName mismatch: %q", got)
	}
	if !matchesSearch("Quarterly Report", "report") {
		t.Fatalf("expected case-insensitive search match")
	}
}

func TestSortBrowserItems(t *testing.T) {
	t.Parallel()

	items := []browserItem{
		{ID: "file-small", Name: "b.txt", Type: itemTypeFile, SizeBytes: int64Ptr(1), ModifiedAt: time.Unix(2, 0)},
		{ID: "folder", Name: "z-folder", Type: itemTypeFolder, ModifiedAt: time.Unix(1, 0)},
		{ID: "file-large", Name: "a.txt", Type: itemTypeFile, SizeBytes: int64Ptr(10), ModifiedAt: time.Unix(3, 0)},
	}

	sortBrowserItems(items, "size", "desc")
	if items[0].Type != itemTypeFolder {
		t.Fatalf("folders should remain first, got %s", items[0].Type)
	}
	if items[1].ID != "file-large" || items[2].ID != "file-small" {
		t.Fatalf("files not sorted by size desc: %+v", items)
	}
}

func TestSanitizeRateLimitSegment(t *testing.T) {
	t.Parallel()

	if got := sanitizeRateLimitSegment(" User:Name / Path\\Part "); got != "user_name___path_part" {
		t.Fatalf("unexpected sanitized segment: %q", got)
	}
	if got := sanitizeRateLimitSegment("   "); got != "" {
		t.Fatalf("blank segment should stay blank: %q", got)
	}
}
