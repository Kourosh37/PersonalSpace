package httpapi

import (
	"encoding/json"
	"testing"
)

func TestValidateSharingSetting(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		key     string
		raw     string
		wantErr bool
	}{
		{name: "bool valid", key: "sharing.enabled", raw: "true", wantErr: false},
		{name: "bool invalid type", key: "sharing.enabled", raw: "\"true\"", wantErr: true},
		{name: "expiration valid", key: "sharing.default_expiration_hours", raw: "168", wantErr: false},
		{name: "expiration negative", key: "sharing.default_expiration_hours", raw: "-1", wantErr: true},
		{name: "password mode optional", key: "sharing.require_password_mode", raw: "\"optional\"", wantErr: false},
		{name: "password mode invalid", key: "sharing.require_password_mode", raw: "\"sometimes\"", wantErr: true},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := validateSharingSetting(tc.key, json.RawMessage(tc.raw))
			if tc.wantErr && err == nil {
				t.Fatalf("expected error, got nil")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("expected nil error, got %v", err)
			}
		})
	}
}

func TestValidatePreviewSetting(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		key     string
		raw     string
		wantErr bool
	}{
		{name: "preview enabled valid", key: "preview.enabled", raw: "true", wantErr: false},
		{name: "preview enabled invalid", key: "preview.enabled", raw: "1", wantErr: true},
		{name: "text max valid", key: "preview.text_max_bytes", raw: "1048576", wantErr: false},
		{name: "text max zero", key: "preview.text_max_bytes", raw: "0", wantErr: true},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := validatePreviewSetting(tc.key, json.RawMessage(tc.raw))
			if tc.wantErr && err == nil {
				t.Fatalf("expected error, got nil")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("expected nil error, got %v", err)
			}
		})
	}
}

func TestValidateScopedSetting(t *testing.T) {
	t.Parallel()

	if err := validateScopedSetting("sharing.", "sharing.enabled", json.RawMessage("true")); err != nil {
		t.Fatalf("expected nil error for sharing scope, got %v", err)
	}
	if err := validateScopedSetting("preview.", "preview.text_max_bytes", json.RawMessage("0")); err == nil {
		t.Fatalf("expected error for invalid preview scoped value")
	}
	if err := validateScopedSetting("unknown.", "x", json.RawMessage("{}")); err != nil {
		t.Fatalf("unknown scope should pass through, got %v", err)
	}
}
