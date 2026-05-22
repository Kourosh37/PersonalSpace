package httpapi

import (
	"encoding/json"
	"fmt"
)

func validateScopedSetting(prefix string, key string, raw json.RawMessage) error {
	switch prefix {
	case "sharing.":
		return validateSharingSetting(key, raw)
	case "preview.":
		return validatePreviewSetting(key, raw)
	default:
		return nil
	}
}

func validateSharingSetting(key string, raw json.RawMessage) error {
	switch key {
	case "sharing.enabled", "sharing.public_preview_enabled", "sharing.public_download_enabled", "sharing.allow_folder_sharing", "sharing.allow_permanent_links":
		var v bool
		if err := json.Unmarshal(raw, &v); err != nil {
			return fmt.Errorf("%s must be a boolean", key)
		}
		return nil
	case "sharing.default_expiration_hours":
		var v int64
		if err := json.Unmarshal(raw, &v); err != nil {
			return fmt.Errorf("%s must be an integer number", key)
		}
		if v < 0 {
			return fmt.Errorf("%s must be zero or greater", key)
		}
		return nil
	case "sharing.require_password_mode":
		var v string
		if err := json.Unmarshal(raw, &v); err != nil {
			return fmt.Errorf("%s must be a string", key)
		}
		if v != "optional" && v != "always" && v != "disabled" {
			return fmt.Errorf("%s must be optional, always, or disabled", key)
		}
		return nil
	default:
		return nil
	}
}

func validatePreviewSetting(key string, raw json.RawMessage) error {
	switch key {
	case "preview.enabled", "preview.public_preview_enabled", "preview.office_enabled", "preview.media_enabled", "preview.image_thumbnails_enabled":
		var v bool
		if err := json.Unmarshal(raw, &v); err != nil {
			return fmt.Errorf("%s must be a boolean", key)
		}
		return nil
	case "preview.text_max_bytes", "preview.csv_max_rows", "preview.office_conversion_timeout_seconds", "preview.max_auto_generate_size_bytes":
		var v int64
		if err := json.Unmarshal(raw, &v); err != nil {
			return fmt.Errorf("%s must be an integer number", key)
		}
		if v <= 0 {
			return fmt.Errorf("%s must be greater than zero", key)
		}
		return nil
	default:
		return nil
	}
}
