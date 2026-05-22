package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	HTTPAddr                 string
	DBDSN                    string
	RedisAddr                string
	AppName                  string
	PublicBaseURL            string
	SessionCookieName        string
	SessionTTL               time.Duration
	SessionSecure            bool
	SessionSameSite          string
	EnforceSecureCookies     bool
	StorageRoot              string
	LoginRatePerMin          int
	ShareRatePerMin          int
	UploadInitRatePerMin     int
	UploadCompleteRatePerMin int
	TusCreateRatePerMin      int
	PreviewJobRatePerMin     int
	ZipDownloadRatePerMin    int
	CSRFDisabled             bool
	AllowedOrigins           []string
}

func Load() (Config, error) {
	ttlHours, err := getenvInt("BACKEND_SESSION_TTL_HOURS", 168)
	if err != nil {
		return Config{}, fmt.Errorf("parse BACKEND_SESSION_TTL_HOURS: %w", err)
	}

	secureCookie, err := getenvBool("BACKEND_SESSION_SECURE", false)
	if err != nil {
		return Config{}, fmt.Errorf("parse BACKEND_SESSION_SECURE: %w", err)
	}
	enforceSecureCookies, err := getenvBool("BACKEND_ENFORCE_SECURE_COOKIES", true)
	if err != nil {
		return Config{}, fmt.Errorf("parse BACKEND_ENFORCE_SECURE_COOKIES: %w", err)
	}
	csrfDisabled, err := getenvBool("BACKEND_CSRF_DISABLED", false)
	if err != nil {
		return Config{}, fmt.Errorf("parse BACKEND_CSRF_DISABLED: %w", err)
	}
	sessionSameSite := strings.ToLower(strings.TrimSpace(getenv("BACKEND_SESSION_SAME_SITE", "lax")))
	switch sessionSameSite {
	case "lax", "strict", "none":
	default:
		return Config{}, fmt.Errorf("BACKEND_SESSION_SAME_SITE must be one of: lax, strict, none")
	}

	cfg := Config{
		HTTPAddr:                 getenv("BACKEND_HTTP_ADDR", ":8080"),
		DBDSN:                    os.Getenv("DB_DSN"),
		RedisAddr:                getenv("REDIS_ADDR", "redis:6379"),
		AppName:                  getenv("APP_NAME", "Space"),
		PublicBaseURL:            getenv("PUBLIC_BASE_URL", "http://localhost"),
		SessionCookieName:        getenv("BACKEND_SESSION_COOKIE_NAME", "space_session"),
		SessionTTL:               time.Duration(ttlHours) * time.Hour,
		SessionSecure:            secureCookie,
		SessionSameSite:          sessionSameSite,
		EnforceSecureCookies:     enforceSecureCookies,
		StorageRoot:              getenv("BACKEND_STORAGE_ROOT", "/data/storage"),
		LoginRatePerMin:          getenvIntMust("SECURITY_LOGIN_RATE_LIMIT_PER_MINUTE", 15),
		ShareRatePerMin:          getenvIntMust("SECURITY_SHARE_PASSWORD_RATE_LIMIT_PER_MINUTE", 20),
		UploadInitRatePerMin:     getenvIntMust("SECURITY_UPLOAD_INIT_RATE_LIMIT_PER_MINUTE", 60),
		UploadCompleteRatePerMin: getenvIntMust("SECURITY_UPLOAD_COMPLETE_RATE_LIMIT_PER_MINUTE", 60),
		TusCreateRatePerMin:      getenvIntMust("SECURITY_TUS_CREATE_RATE_LIMIT_PER_MINUTE", 60),
		PreviewJobRatePerMin:     getenvIntMust("SECURITY_PREVIEW_JOB_RATE_LIMIT_PER_MINUTE", 30),
		ZipDownloadRatePerMin:    getenvIntMust("SECURITY_ZIP_DOWNLOAD_RATE_LIMIT_PER_MINUTE", 20),
		CSRFDisabled:             csrfDisabled,
		AllowedOrigins:           parseCSV(getenv("BACKEND_ALLOWED_ORIGINS", "")),
	}

	if cfg.DBDSN == "" {
		return Config{}, fmt.Errorf("DB_DSN is required")
	}
	publicBaseLower := strings.ToLower(strings.TrimSpace(cfg.PublicBaseURL))
	if cfg.EnforceSecureCookies && strings.HasPrefix(publicBaseLower, "https://") && !cfg.SessionSecure {
		return Config{}, fmt.Errorf("BACKEND_SESSION_SECURE must be true when PUBLIC_BASE_URL is https and BACKEND_ENFORCE_SECURE_COOKIES=true")
	}
	if cfg.SessionSameSite == "none" && !cfg.SessionSecure {
		return Config{}, fmt.Errorf("BACKEND_SESSION_SAME_SITE=none requires BACKEND_SESSION_SECURE=true")
	}

	return cfg, nil
}

func parseCSV(value string) []string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	parts := strings.Split(value, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		item := strings.TrimSpace(part)
		if item == "" {
			continue
		}
		result = append(result, item)
	}
	return result
}

func getenvIntMust(key string, fallback int) int {
	value, err := getenvInt(key, fallback)
	if err != nil || value <= 0 {
		return fallback
	}
	return value
}

func getenv(key, fallback string) string {
	val := os.Getenv(key)
	if val == "" {
		return fallback
	}
	return val
}

func getenvInt(key string, fallback int) (int, error) {
	val := os.Getenv(key)
	if val == "" {
		return fallback, nil
	}
	parsed, err := strconv.Atoi(val)
	if err != nil {
		return 0, err
	}
	return parsed, nil
}

func getenvBool(key string, fallback bool) (bool, error) {
	val := os.Getenv(key)
	if val == "" {
		return fallback, nil
	}
	parsed, err := strconv.ParseBool(val)
	if err != nil {
		return false, err
	}
	return parsed, nil
}
