package main

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"space/backend/internal/logging"

	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	logging.Configure("space-migrate")

	if len(os.Args) < 2 || os.Args[1] != "up" {
		slog.Error("usage: migrate up")
		os.Exit(1)
	}

	dsn := os.Getenv("DB_DSN")
	if dsn == "" {
		slog.Error("DB_DSN is required")
		os.Exit(1)
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		slog.Error("connect db", "error", err)
		os.Exit(1)
	}
	defer pool.Close()

	if err := runMigrations(ctx, pool, "/app/migrations"); err != nil {
		slog.Error("run migrations", "error", err)
		os.Exit(1)
	}

	slog.Info("migrations applied")
}

func runMigrations(ctx context.Context, pool *pgxpool.Pool, dir string) error {
	if _, err := pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version text primary key,
			applied_at timestamptz not null default now()
		)
	`); err != nil {
		return err
	}

	entries := make([]string, 0)
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if strings.HasSuffix(d.Name(), ".sql") {
			entries = append(entries, path)
		}
		return nil
	})
	if err != nil {
		return err
	}
	sort.Strings(entries)

	for _, path := range entries {
		version := filepath.Base(path)
		var exists bool
		err := pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE version=$1)`, version).Scan(&exists)
		if err != nil {
			return err
		}
		if exists {
			continue
		}

		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}

		tx, err := pool.Begin(ctx)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, string(body)); err != nil {
			tx.Rollback(ctx)
			return fmt.Errorf("apply %s: %w", version, err)
		}
		if _, err := tx.Exec(ctx, `INSERT INTO schema_migrations(version) VALUES ($1)`, version); err != nil {
			tx.Rollback(ctx)
			return fmt.Errorf("record migration %s: %w", version, err)
		}
		if err := tx.Commit(ctx); err != nil {
			return err
		}
		slog.Info("applied migration", "version", version)
	}

	if len(entries) == 0 {
		return errors.New("no migrations found")
	}

	return nil
}
