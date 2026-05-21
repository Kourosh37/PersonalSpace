package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"space/backend/internal/config"
	"space/backend/internal/db"
	httpapi "space/backend/internal/http"
	"space/backend/internal/maintenance"
	"space/backend/internal/security"
	"space/backend/internal/storage"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		slog.Error("load config", "error", err)
		os.Exit(1)
	}

	appCtx, cancelApp := context.WithCancel(context.Background())
	defer cancelApp()

	ctx := appCtx
	pool, err := db.Connect(ctx, cfg.DBDSN)
	if err != nil {
		slog.Error("connect db", "error", err)
		os.Exit(1)
	}
	defer pool.Close()

	localStorage, err := storage.NewLocalStorage(cfg.StorageRoot)
	if err != nil {
		slog.Error("initialize local storage", "error", err)
		os.Exit(1)
	}

	var rateLimiter security.RateLimiter
	redisLimiter := security.NewRedisRateLimiter(cfg.RedisAddr)
	if err := redisLimiter.Ping(ctx); err != nil {
		slog.Warn("redis rate limiter ping failed, continuing without strict limiter", "error", err)
	} else {
		rateLimiter = redisLimiter
	}

	handler := httpapi.Handler{DB: pool, Cfg: cfg, Storage: localStorage, RateLimiter: rateLimiter}

	maintenanceRunner := maintenance.Runner{
		DB:      pool,
		Storage: localStorage,
		Every:   30 * time.Minute,
	}
	go maintenanceRunner.Start(appCtx)
	srv := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           handler.Router(),
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		slog.Info("http server listening", "addr", cfg.HTTPAddr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("http server failed", "error", err)
			os.Exit(1)
		}
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh
	cancelApp()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		slog.Error("graceful shutdown failed", "error", err)
	}
}
