package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"space/backend/internal/auth"
	"space/backend/internal/config"
	"space/backend/internal/storage"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestIntegrationAuthUploadShareAndPreview(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("INTEGRATION_DB_DSN"))
	if dsn == "" {
		t.Skip("INTEGRATION_DB_DSN is not set")
	}
	if os.Getenv("SPACE_TEST_ALLOW_DB_RESET") != "1" {
		t.Skip("SPACE_TEST_ALLOW_DB_RESET=1 is required because integration tests reset the target database")
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("connect integration db: %v", err)
	}
	defer pool.Close()

	resetIntegrationDB(t, ctx, pool)

	root := t.TempDir()
	localStorage, err := storage.NewLocalStorage(root)
	if err != nil {
		t.Fatalf("create local storage: %v", err)
	}

	handler := Handler{
		DB:      pool,
		Storage: localStorage,
		Cfg: config.Config{
			PublicBaseURL:            "http://example.test",
			SessionCookieName:        "space_session_test",
			SessionTTL:               time.Hour,
			SessionSameSite:          "lax",
			StorageRoot:              root,
			LoginRatePerMin:          1000,
			ShareRatePerMin:          1000,
			UploadInitRatePerMin:     1000,
			UploadCompleteRatePerMin: 1000,
			TusCreateRatePerMin:      1000,
			PreviewJobRatePerMin:     1000,
			ZipDownloadRatePerMin:    1000,
			CSRFDisabled:             true,
		},
	}
	server := httptest.NewServer(handler.Router())
	defer server.Close()

	userID := createIntegrationUser(t, ctx, pool, "admin", "password123", "admin", int64Ptr(3))
	sessionCookie := integrationLogin(t, server.URL, "admin", "password123")

	t.Run("auth session me", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodGet, server.URL+"/api/auth/me", nil)
		req.AddCookie(sessionCookie)
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("me request: %v", err)
		}
		defer res.Body.Close()
		if res.StatusCode != http.StatusOK {
			t.Fatalf("me status = %d", res.StatusCode)
		}
	})

	t.Run("multipart upload enforces quota and persists successful file", func(t *testing.T) {
		res := postMultipartFile(t, server.URL+"/api/files/upload", sessionCookie, "too-large.txt", []byte("1234"))
		if res.StatusCode != http.StatusBadRequest {
			t.Fatalf("quota rejection status = %d", res.StatusCode)
		}
		res.Body.Close()

		res = postMultipartFile(t, server.URL+"/api/files/upload", sessionCookie, "ok.txt", []byte("12"))
		if res.StatusCode != http.StatusCreated {
			body, _ := io.ReadAll(res.Body)
			t.Fatalf("upload status = %d body=%s", res.StatusCode, string(body))
		}
		res.Body.Close()

		var used int64
		if err := pool.QueryRow(ctx, `SELECT used_storage_bytes FROM users WHERE id=$1`, userID).Scan(&used); err != nil {
			t.Fatalf("read used storage: %v", err)
		}
		if used != 2 {
			t.Fatalf("used_storage_bytes = %d, want 2", used)
		}
	})

	fileID := createIntegrationFile(t, ctx, pool, localStorage, userID, "share-target.txt", "files/test/share-target.txt")

	t.Run("share policy blocks disabled public downloads", func(t *testing.T) {
		_, err := pool.Exec(ctx, `UPDATE system_settings SET value='false'::jsonb WHERE key='sharing.public_download_enabled'`)
		if err != nil {
			t.Fatalf("disable public downloads: %v", err)
		}

		body := strings.NewReader(`{"targetType":"file","targetId":"` + fileID + `","allowPreview":false,"allowDownload":true}`)
		req, _ := http.NewRequest(http.MethodPost, server.URL+"/api/shares", body)
		req.Header.Set("Content-Type", "application/json")
		req.AddCookie(sessionCookie)
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("create share: %v", err)
		}
		defer res.Body.Close()
		if res.StatusCode != http.StatusForbidden {
			t.Fatalf("share policy status = %d", res.StatusCode)
		}
	})

	t.Run("preview job endpoint persists queued job", func(t *testing.T) {
		body := strings.NewReader(`{"jobType":"metadata"}`)
		req, _ := http.NewRequest(http.MethodPost, server.URL+"/api/files/"+fileID+"/preview-jobs", body)
		req.Header.Set("Content-Type", "application/json")
		req.AddCookie(sessionCookie)
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("create preview job: %v", err)
		}
		defer res.Body.Close()
		if res.StatusCode != http.StatusAccepted {
			payload, _ := io.ReadAll(res.Body)
			t.Fatalf("preview job status = %d body=%s", res.StatusCode, string(payload))
		}

		var count int
		if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM preview_jobs WHERE file_id=$1 AND job_type='metadata' AND status='queued'`, fileID).Scan(&count); err != nil {
			t.Fatalf("count preview jobs: %v", err)
		}
		if count != 1 {
			t.Fatalf("queued preview jobs = %d, want 1", count)
		}
	})
}

func resetIntegrationDB(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()
	if _, err := pool.Exec(ctx, `DROP SCHEMA public CASCADE; CREATE SCHEMA public;`); err != nil {
		t.Fatalf("reset schema: %v", err)
	}

	migrationsDir := filepath.Join("..", "..", "migrations")
	entries, err := os.ReadDir(migrationsDir)
	if err != nil {
		t.Fatalf("read migrations: %v", err)
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".sql") {
			names = append(names, entry.Name())
		}
	}
	sort.Strings(names)
	for _, name := range names {
		body, err := os.ReadFile(filepath.Join(migrationsDir, name))
		if err != nil {
			t.Fatalf("read migration %s: %v", name, err)
		}
		if _, err := pool.Exec(ctx, string(body)); err != nil {
			t.Fatalf("apply migration %s: %v", name, err)
		}
	}
}

func createIntegrationUser(t *testing.T, ctx context.Context, pool *pgxpool.Pool, username string, password string, role string, quota *int64) string {
	t.Helper()
	hash, err := auth.HashPassword(password)
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	id := uuid.NewString()
	_, err = pool.Exec(ctx, `
		INSERT INTO users (id, username, password_hash, role, is_active, storage_quota_bytes, used_storage_bytes, created_at, updated_at)
		VALUES ($1,$2,$3,$4,true,$5,0,now(),now())
	`, id, username, hash, role, quota)
	if err != nil {
		t.Fatalf("insert user: %v", err)
	}
	return id
}

func integrationLogin(t *testing.T, baseURL string, username string, password string) *http.Cookie {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"username": username, "password": password})
	res, err := http.Post(baseURL+"/api/auth/login", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("login request: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		payload, _ := io.ReadAll(res.Body)
		t.Fatalf("login status = %d body=%s", res.StatusCode, string(payload))
	}
	for _, cookie := range res.Cookies() {
		if cookie.Name == "space_session_test" {
			return cookie
		}
	}
	t.Fatalf("session cookie not set")
	return nil
}

func postMultipartFile(t *testing.T, url string, cookie *http.Cookie, filename string, data []byte) *http.Response {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", filename)
	if err != nil {
		t.Fatalf("create multipart file: %v", err)
	}
	if _, err := part.Write(data); err != nil {
		t.Fatalf("write multipart file: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}

	req, _ := http.NewRequest(http.MethodPost, url, &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.AddCookie(cookie)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("multipart request: %v", err)
	}
	return res
}

func createIntegrationFile(t *testing.T, ctx context.Context, pool *pgxpool.Pool, store storage.Interface, ownerID string, name string, key string) string {
	t.Helper()
	if err := store.PutStream(ctx, key, strings.NewReader("preview me")); err != nil {
		t.Fatalf("put storage object: %v", err)
	}
	id := uuid.NewString()
	_, err := pool.Exec(ctx, `
		INSERT INTO files (id, owner_id, folder_id, name, original_name, storage_key, size_bytes, mime_type, extension, checksum_sha256, status, created_at, updated_at)
		VALUES ($1,$2,NULL,$3,$3,$4,10,'text/plain','txt',NULL,'ready',now(),now())
	`, id, ownerID, name, key)
	if err != nil {
		t.Fatalf("insert file: %v", err)
	}
	return id
}
