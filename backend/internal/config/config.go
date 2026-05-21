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
	RedisAddr         string
	AppName           string
	PublicBaseURL     string
	SessionCookieName string
	SessionTTL        time.Duration
	SessionSecure     bool
	StorageRoot       string
	LoginRatePerMin   int
	ShareRatePerMin   int
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
		RedisAddr:         getenv("REDIS_ADDR", "redis:6379"),
		AppName:           getenv("APP_NAME", "Space"),
		PublicBaseURL:     getenv("PUBLIC_BASE_URL", "http://localhost"),
		SessionCookieName: getenv("BACKEND_SESSION_COOKIE_NAME", "space_session"),
		SessionTTL:        time.Duration(ttlHours) * time.Hour,
		SessionSecure:     secureCookie,
		StorageRoot:       getenv("BACKEND_STORAGE_ROOT", "/data/storage"),
		LoginRatePerMin:   getenvIntMust("SECURITY_LOGIN_RATE_LIMIT_PER_MINUTE", 15),
		ShareRatePerMin:   getenvIntMust("SECURITY_SHARE_PASSWORD_RATE_LIMIT_PER_MINUTE", 20),
	}

	if cfg.DBDSN == "" {
		return Config{}, fmt.Errorf("DB_DSN is required")
	}

	return cfg, nil
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
