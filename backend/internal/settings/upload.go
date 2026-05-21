package settings

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	UploadMaxFileSizeModeKey  = "upload.max_file_size_mode"
	UploadMaxFileSizeBytesKey = "upload.max_file_size_bytes"
)

type UploadSettings struct {
	Mode             string `json:"mode"`
	MaxFileSizeBytes *int64 `json:"maxFileSizeBytes"`
}

func GetUploadSettings(ctx context.Context, pool *pgxpool.Pool) (UploadSettings, error) {
	mode := "unlimited"
	var maxBytes *int64

	var rawMode []byte
	err := pool.QueryRow(ctx, `SELECT value FROM system_settings WHERE key=$1`, UploadMaxFileSizeModeKey).Scan(&rawMode)
	if err == nil {
		if err := json.Unmarshal(rawMode, &mode); err != nil {
			return UploadSettings{}, fmt.Errorf("decode mode: %w", err)
		}
	}

	var rawBytes []byte
	err = pool.QueryRow(ctx, `SELECT value FROM system_settings WHERE key=$1`, UploadMaxFileSizeBytesKey).Scan(&rawBytes)
	if err == nil {
		if string(rawBytes) != "null" {
			var parsed int64
			if err := json.Unmarshal(rawBytes, &parsed); err != nil {
				return UploadSettings{}, fmt.Errorf("decode max bytes: %w", err)
			}
			maxBytes = &parsed
		}
	}

	if mode != "unlimited" && mode != "custom" {
		mode = "unlimited"
		maxBytes = nil
	}

	if mode == "unlimited" {
		maxBytes = nil
	}

	return UploadSettings{Mode: mode, MaxFileSizeBytes: maxBytes}, nil
}

func ValidateUploadSettings(input UploadSettings) error {
	if input.Mode != "unlimited" && input.Mode != "custom" {
		return errors.New("mode must be unlimited or custom")
	}
	if input.Mode == "custom" {
		if input.MaxFileSizeBytes == nil || *input.MaxFileSizeBytes <= 0 {
			return errors.New("maxFileSizeBytes must be set and greater than zero in custom mode")
		}
	}
	return nil
}

func UpdateUploadSettings(ctx context.Context, pool *pgxpool.Pool, actorUserID string, input UploadSettings, ip string, ua string) error {
	if err := ValidateUploadSettings(input); err != nil {
		return err
	}

	modeBytes, _ := json.Marshal(input.Mode)
	maxBytesPayload := []byte("null")
	if input.Mode == "custom" && input.MaxFileSizeBytes != nil {
		payload, _ := json.Marshal(*input.MaxFileSizeBytes)
		maxBytesPayload = payload
	}

	metadata, _ := json.Marshal(map[string]any{
		"upload.max_file_size_mode":  input.Mode,
		"upload.max_file_size_bytes": input.MaxFileSizeBytes,
	})

	tx, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, upsertSettingSQL(), UploadMaxFileSizeModeKey, modeBytes, "string", "Upload max file size mode", false); err != nil {
		return fmt.Errorf("upsert mode: %w", err)
	}
	if _, err := tx.Exec(ctx, upsertSettingSQL(), UploadMaxFileSizeBytesKey, maxBytesPayload, "number|null", "Upload max file size in bytes", false); err != nil {
		return fmt.Errorf("upsert bytes: %w", err)
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO audit_logs (id, user_id, action, target_type, target_id, ip_address, user_agent, metadata, created_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
	`, uuid.NewString(), actorUserID, "admin.settings.upload.updated", "system_settings", nil, ip, ua, metadata, time.Now().UTC()); err != nil {
		return fmt.Errorf("insert audit log: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit tx: %w", err)
	}

	return nil
}

func upsertSettingSQL() string {
	return `
		INSERT INTO system_settings (id, key, value, value_type, description, is_public, created_at, updated_at)
		VALUES (gen_random_uuid(), $1, $2::jsonb, $3, $4, $5, now(), now())
		ON CONFLICT (key)
		DO UPDATE SET value = EXCLUDED.value, value_type = EXCLUDED.value_type, description = EXCLUDED.description, is_public = EXCLUDED.is_public, updated_at = now()
	`
}