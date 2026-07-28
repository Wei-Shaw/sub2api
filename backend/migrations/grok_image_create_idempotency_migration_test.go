package migrations

import (
	"strings"
	"testing"
)

func TestGrokImageCreateIdempotencyMigrationIsIndependentAndExpirable(t *testing.T) {
	body, err := FS.ReadFile("188_grok_image_create_idempotency.sql")
	if err != nil {
		t.Fatal(err)
	}
	sql := string(body)
	for _, required := range []string{
		"CREATE TABLE IF NOT EXISTS grok_image_create_idempotency",
		"PRIMARY KEY (user_id, api_key_id, group_id, endpoint, idempotency_key_hash)",
		"request_hash CHAR(64) NOT NULL",
		"upstream_idempotency_key VARCHAR(96) NOT NULL",
		"account_id BIGINT",
		"response_body BYTEA",
		"expires_at TIMESTAMPTZ NOT NULL",
		"idx_grok_image_create_idempotency_expiry",
		"status IN ('processing', 'completed')",
		"expired rows are eligible for bounded concurrent cleanup",
	} {
		if !strings.Contains(sql, required) {
			t.Fatalf("migration missing %q", required)
		}
	}
	if strings.Contains(sql, "INSERT INTO grok_video_request_owners") {
		t.Fatal("image idempotency migration must never write video owners")
	}
}
