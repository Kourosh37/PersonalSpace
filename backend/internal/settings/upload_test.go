package settings

import "testing"

func TestValidateUploadSettings(t *testing.T) {
	t.Parallel()

	max := int64(1024)
	tests := []struct {
		name    string
		input   UploadSettings
		wantErr bool
	}{
		{name: "unlimited without max", input: UploadSettings{Mode: "unlimited"}, wantErr: false},
		{name: "custom with positive max", input: UploadSettings{Mode: "custom", MaxFileSizeBytes: &max}, wantErr: false},
		{name: "invalid mode", input: UploadSettings{Mode: "limited", MaxFileSizeBytes: &max}, wantErr: true},
		{name: "custom missing max", input: UploadSettings{Mode: "custom"}, wantErr: true},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := ValidateUploadSettings(tc.input)
			if tc.wantErr && err == nil {
				t.Fatalf("expected error, got nil")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("expected nil error, got %v", err)
			}
		})
	}
}

func TestUpsertSettingSQLContainsConflictClause(t *testing.T) {
	t.Parallel()

	sql := upsertSettingSQL()
	if sql == "" {
		t.Fatalf("expected SQL")
	}
	if !containsAll(sql, "INSERT INTO system_settings", "ON CONFLICT (key)", "DO UPDATE SET") {
		t.Fatalf("upsert SQL does not contain required conflict handling: %s", sql)
	}
}

func containsAll(value string, fragments ...string) bool {
	for _, fragment := range fragments {
		if !contains(value, fragment) {
			return false
		}
	}
	return true
}

func contains(value string, fragment string) bool {
	for i := 0; i+len(fragment) <= len(value); i++ {
		if value[i:i+len(fragment)] == fragment {
			return true
		}
	}
	return false
}
