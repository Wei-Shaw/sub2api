package migrations

import (
	"strings"
	"testing"
)

func TestGrokVideoRequestOwnersMigrationIsScopedAndExpirable(t *testing.T) {
	body, err := FS.ReadFile("187_grok_video_request_owners.sql")
	if err != nil {
		t.Fatal(err)
	}
	sql := string(body)
	for _, required := range []string{
		"CREATE TABLE IF NOT EXISTS grok_video_request_owners",
		"PRIMARY KEY (request_id, user_id, api_key_id, group_id)",
		"account_id BIGINT NOT NULL",
		"expires_at TIMESTAMPTZ NOT NULL",
		"last_accessed_at TIMESTAMPTZ NOT NULL DEFAULT NOW()",
		"terminal_at TIMESTAMPTZ",
		"idx_grok_video_request_owners_expiry",
		"idx_grok_video_request_owners_terminal_cleanup",
		"CREATE TABLE IF NOT EXISTS grok_video_create_idempotency",
		"PRIMARY KEY (user_id, api_key_id, group_id, endpoint, idempotency_key_hash)",
		"request_hash CHAR(64) NOT NULL",
		"upstream_idempotency_key VARCHAR(96) NOT NULL",
		"idx_grok_video_create_idempotency_expiry",
		"status IN ('processing', 'completed')",
		"expired rows are eligible for bounded concurrent cleanup",
	} {
		if !strings.Contains(sql, required) {
			t.Fatalf("migration missing %q", required)
		}
	}
}
