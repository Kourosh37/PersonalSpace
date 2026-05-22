package db

import (
	"context"
	"fmt"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

func EnsureMigrationsApplied(ctx context.Context, pool *pgxpool.Pool, migrationsDir string) error {
	expected, err := listMigrationVersions(migrationsDir)
	if err != nil {
		return err
	}
	if len(expected) == 0 {
		return fmt.Errorf("no migration files found in %s", migrationsDir)
	}

	var hasMigrationsTable bool
	if err := pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM information_schema.tables
			WHERE table_schema='public' AND table_name='schema_migrations'
		)
	`).Scan(&hasMigrationsTable); err != nil {
		return fmt.Errorf("check schema_migrations table: %w", err)
	}
	if !hasMigrationsTable {
		return fmt.Errorf("schema_migrations table is missing; run migrations before starting the server")
	}

	rows, err := pool.Query(ctx, `SELECT version FROM schema_migrations`)
	if err != nil {
		return fmt.Errorf("read schema_migrations: %w", err)
	}
	defer rows.Close()

	applied := make(map[string]struct{}, len(expected))
	for rows.Next() {
		var version string
		if err := rows.Scan(&version); err != nil {
			return fmt.Errorf("scan schema_migrations: %w", err)
		}
		applied[version] = struct{}{}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate schema_migrations: %w", err)
	}

	pending := make([]string, 0, len(expected))
	for _, version := range expected {
		if _, ok := applied[version]; !ok {
			pending = append(pending, version)
		}
	}
	if len(pending) == 0 {
		return nil
	}

	if len(pending) > 8 {
		head := strings.Join(pending[:8], ", ")
		return fmt.Errorf("database schema is outdated (%d pending migrations). first pending: %s", len(pending), head)
	}
	return fmt.Errorf("database schema is outdated; pending migrations: %s", strings.Join(pending, ", "))
}

func listMigrationVersions(dir string) ([]string, error) {
	entries := make([]string, 0, 16)
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if strings.HasSuffix(d.Name(), ".sql") {
			entries = append(entries, d.Name())
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walk migrations dir %s: %w", dir, err)
	}
	sort.Strings(entries)
	return entries, nil
}
