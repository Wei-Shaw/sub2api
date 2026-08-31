package migrations

import (
	"strings"
	"testing"
)

func TestAPIKeyRotationMigrationAddsTimestamp(t *testing.T) {
	content, err := FS.ReadFile("232_api_keys_last_rotated_at.sql")
	if err != nil {
		t.Fatalf("read migration: %v", err)
	}
	sql := strings.ToLower(string(content))
	if !strings.Contains(sql, "add column if not exists last_rotated_at timestamptz") {
		t.Fatalf("migration does not add last_rotated_at: %s", sql)
	}
}
