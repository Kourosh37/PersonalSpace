package main

import (
	"context"
	"flag"
	"log/slog"
	"os"
	"strings"
	"time"

	"space/backend/internal/auth"
	"space/backend/internal/logging"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	logging.Configure("space-create-admin")

	var username string
	var password string
	flag.StringVar(&username, "username", "", "admin username")
	flag.StringVar(&password, "password", "", "admin password")
	flag.Parse()

	username = strings.TrimSpace(username)
	if username == "" || password == "" {
		slog.Error("username and password are required")
		os.Exit(1)
	}

	dsn := os.Getenv("DB_DSN")
	if dsn == "" {
		slog.Error("DB_DSN is required")
		os.Exit(1)
	}

	hash, err := auth.HashPassword(password)
	if err != nil {
		slog.Error("hash password", "error", err)
		os.Exit(1)
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		slog.Error("connect db", "error", err)
		os.Exit(1)
	}
	defer pool.Close()

	var exists bool
	if err := pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM users WHERE username=$1)`, username).Scan(&exists); err != nil {
		slog.Error("query user existence", "error", err)
		os.Exit(1)
	}
	if exists {
		slog.Error("user already exists", "username", username)
		os.Exit(1)
	}

	id := uuid.NewString()
	now := time.Now().UTC()
	_, err = pool.Exec(ctx, `
		INSERT INTO users (id, username, password_hash, role, is_active, used_storage_bytes, created_at, updated_at)
		VALUES ($1,$2,$3,'admin',true,0,$4,$5)
	`, id, username, hash, now, now)
	if err != nil {
		slog.Error("insert user", "error", err)
		os.Exit(1)
	}

	slog.Info("admin user created", "username", username, "user_id", id)
}
