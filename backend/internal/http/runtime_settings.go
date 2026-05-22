package httpapi

import (
	"context"
	"encoding/json"

	"github.com/jackc/pgx/v5/pgxpool"
)

type sharingRuntimeSettings struct {
	Enabled              bool
	PublicPreviewEnabled bool
	PublicDownloadEnable bool
}

type previewRuntimeSettings struct {
	Enabled              bool
	PublicPreviewEnabled bool
	MediaEnabled         bool
	OfficeEnabled        bool
}

func loadBoolSettings(ctx context.Context, db *pgxpool.Pool, defaults map[string]bool) (map[string]bool, error) {
	keys := make([]string, 0, len(defaults))
	for key := range defaults {
		keys = append(keys, key)
	}

	rows, err := db.Query(ctx, `SELECT key, value FROM system_settings WHERE key = ANY($1)`, keys)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make(map[string]bool, len(defaults))
	for key, value := range defaults {
		out[key] = value
	}

	for rows.Next() {
		var key string
		var raw json.RawMessage
		if err := rows.Scan(&key, &raw); err != nil {
			return nil, err
		}
		var parsed bool
		if err := json.Unmarshal(raw, &parsed); err != nil {
			continue
		}
		out[key] = parsed
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return out, nil
}

func (h Handler) getSharingRuntimeSettings(ctx context.Context) (sharingRuntimeSettings, error) {
	values, err := loadBoolSettings(ctx, h.DB, map[string]bool{
		"sharing.enabled":                 true,
		"sharing.public_preview_enabled":  true,
		"sharing.public_download_enabled": true,
	})
	if err != nil {
		return sharingRuntimeSettings{}, err
	}

	return sharingRuntimeSettings{
		Enabled:              values["sharing.enabled"],
		PublicPreviewEnabled: values["sharing.public_preview_enabled"],
		PublicDownloadEnable: values["sharing.public_download_enabled"],
	}, nil
}

func (h Handler) getPreviewRuntimeSettings(ctx context.Context) (previewRuntimeSettings, error) {
	values, err := loadBoolSettings(ctx, h.DB, map[string]bool{
		"preview.enabled":                true,
		"preview.public_preview_enabled": true,
		"preview.media_enabled":          true,
		"preview.office_enabled":         false,
	})
	if err != nil {
		return previewRuntimeSettings{}, err
	}

	return previewRuntimeSettings{
		Enabled:              values["preview.enabled"],
		PublicPreviewEnabled: values["preview.public_preview_enabled"],
		MediaEnabled:         values["preview.media_enabled"],
		OfficeEnabled:        values["preview.office_enabled"],
	}, nil
}
