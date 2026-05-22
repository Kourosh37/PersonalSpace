package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"space/backend/internal/config"
	"space/backend/internal/db"
	"space/backend/internal/preview"
	"space/backend/internal/storage"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		slog.Error("load config", "error", err)
		os.Exit(1)
	}

	pollInterval := time.Duration(getEnvInt("PREVIEW_WORKER_POLL_INTERVAL_SECONDS", 3)) * time.Second
	maxAttempts := getEnvInt("PREVIEW_WORKER_MAX_ATTEMPTS", 3)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	pool, err := db.Connect(ctx, cfg.DBDSN)
	if err != nil {
		slog.Error("connect db", "error", err)
		os.Exit(1)
	}
	defer pool.Close()

	localStorage, err := storage.NewLocalStorage(cfg.StorageRoot)
	if err != nil {
		slog.Error("initialize storage", "error", err)
		os.Exit(1)
	}

	runner := preview.Runner{
		DB:           pool,
		Storage:      localStorage,
		PollInterval: pollInterval,
		MaxAttempts:  maxAttempts,
	}

	go runner.Start(ctx)

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh
	cancel()
	time.Sleep(200 * time.Millisecond)
}

func getEnvInt(key string, fallback int) int {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(raw)
	if err != nil || parsed <= 0 {
		return fallback
	}
	return parsed
}
