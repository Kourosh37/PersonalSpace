package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

type Config struct {
	HTTPAddr          string
	DBDSN             string
	AppName           string
	PublicBaseURL     string
	SessionCookieName string
	SessionTTL        time.Duration
	SessionSecure     bool
	StorageRoot       string
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

	cfg := Config{
		HTTPAddr:          getenv("BACKEND_HTTP_ADDR", ":8080"),
		DBDSN:             os.Getenv("DB_DSN"),
		AppName:           getenv("APP_NAME", "Space"),
		PublicBaseURL:     getenv("PUBLIC_BASE_URL", "http://localhost"),
		SessionCookieName: getenv("BACKEND_SESSION_COOKIE_NAME", "space_session"),
		SessionTTL:        time.Duration(ttlHours) * time.Hour,
		SessionSecure:     secureCookie,
		StorageRoot:       getenv("BACKEND_STORAGE_ROOT", "/data/storage"),
	}

	if cfg.DBDSN == "" {
		return Config{}, fmt.Errorf("DB_DSN is required")
	}

	return cfg, nil
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