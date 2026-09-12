package repository

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestQuotaObservationWriteSanitizesAndKeepsPartialFields(t *testing.T) {
	query := "UPDATE accounts SET extra = $1 WHERE id = $2"
	stamp := "2026-09-12T08:00:00.123456Z"
	updated, args := quotaObservationWrite(query, []any{"{}", int64(1)}, map[string]any{
		"codex_usage_updated_at": stamp, "codex_5h_used_percent": 100.0,
		"access_token": "must-not-retain", "unrelated": "must-not-retain",
	})
	if !strings.Contains(updated, "INSERT INTO account_quota_observations") || !strings.HasSuffix(updated, query) {
		t.Fatal("journal insert and account update must share one statement")
	}
	if len(args) != 4 {
		t.Fatalf("unexpected args: %d", len(args))
	}
	wantAt, _ := time.Parse(time.RFC3339Nano, stamp)
	if !args[2].(time.Time).Equal(wantAt) {
		t.Fatal("observation timestamp was changed")
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(args[3].(string)), &payload); err != nil {
		t.Fatal(err)
	}
	if len(payload) != 2 || payload["codex_5h_used_percent"] != 100.0 {
		t.Fatalf("must only retain fields from this observation: %v", payload)
	}
	if _, exists := payload["codex_5h_reset_at"]; exists {
		t.Fatal("missing reset must not be filled from the previous snapshot")
	}
}

func TestQuotaObservationWriteIgnoresInvalidAndUnrelatedUpdates(t *testing.T) {
	for _, update := range []map[string]any{
		{"codex_usage_updated_at": "invalid", "codex_5h_used_percent": 10},
		{"name": "account"},
		{"codex_usage_updated_at": "2026-09-12T08:00:00Z"},
	} {
		query, args := quotaObservationWrite("UPDATE accounts", []any{"{}", int64(1)}, update)
		if query != "UPDATE accounts" || len(args) != 2 {
			t.Fatal("non-observations must not create journal entries")
		}
	}
}
