package maintenance

import (
	"context"
	"log/slog"
	"time"

	"space/backend/internal/storage"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Runner struct {
	DB      *pgxpool.Pool
	Storage storage.Interface
	Every   time.Duration
}

func (r Runner) Start(ctx context.Context) {
	interval := r.Every
	if interval <= 0 {
		interval = 30 * time.Minute
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	r.runOnce(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			r.runOnce(ctx)
		}
	}
}

func (r Runner) runOnce(ctx context.Context) {
	if _, err := r.DB.Exec(ctx, `DELETE FROM sessions WHERE expires_at < now()`); err != nil {
		slog.Warn("maintenance cleanup sessions failed", "error", err)
	}

	rows, err := r.DB.Query(ctx, `
		SELECT id, storage_key_temp
		FROM upload_sessions
		WHERE status IN ('initialized','uploading','paused')
		  AND expires_at IS NOT NULL
		  AND expires_at < now()
	`)
	if err != nil {
		slog.Warn("maintenance query expired uploads failed", "error", err)
		return
	}
	defer rows.Close()

	for rows.Next() {
		var id string
		var tempKey string
		if err := rows.Scan(&id, &tempKey); err != nil {
			continue
		}
		_, _ = r.DB.Exec(ctx, `UPDATE upload_sessions SET status='expired', updated_at=now() WHERE id=$1`, id)
		_ = r.Storage.Delete(ctx, tempKey)
	}
}