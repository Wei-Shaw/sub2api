//go:build integration

package repository

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/Wei-Shaw/sub2api/migrations"
	"github.com/stretchr/testify/require"
)

func TestResetOpenAICompactProbeProtocolMigration_DataBehavior(t *testing.T) {
	tx := testTx(t)
	ctx := context.Background()
	probeExtra := `{
		"openai_compact_supported": true,
		"openai_compact_probe_version": 1,
		"openai_compact_checked_at": "2026-08-01T00:00:00Z",
		"openai_compact_last_status": 200,
		"openai_compact_last_error": "old compact",
		"openai_compact_probe_observed_at_unix_nano": 123,
		"openai_compact_mode": "force_on",
		"openai_responses_supported": false,
		"unrelated": "keep"
	}`

	insert := func(name, platform string, deleted bool) int64 {
		var id int64
		deletedExpr := "NULL"
		if deleted {
			deletedExpr = "NOW()"
		}
		err := tx.QueryRowContext(ctx,
			"INSERT INTO accounts (name, platform, type, credentials, extra, deleted_at) VALUES ($1, $2, 'oauth', '{}'::jsonb, $3::jsonb, "+deletedExpr+") RETURNING id",
			name, platform, probeExtra,
		).Scan(&id)
		require.NoError(t, err)
		return id
	}

	activeOpenAI := insert("migration-224-active-openai", "openai", false)
	deletedOpenAI := insert("migration-224-deleted-openai", "openai", true)
	otherPlatform := insert("migration-224-anthropic", "anthropic", false)

	content, err := migrations.FS.ReadFile("224_reset_openai_compact_probe_protocol.sql")
	require.NoError(t, err)
	_, err = tx.ExecContext(ctx, string(content))
	require.NoError(t, err)
	_, err = tx.ExecContext(ctx, string(content))
	require.NoError(t, err, "migration must remain idempotent")

	loadExtra := func(id int64) map[string]any {
		var raw []byte
		require.NoError(t, tx.QueryRowContext(ctx, "SELECT extra FROM accounts WHERE id = $1", id).Scan(&raw))
		var extra map[string]any
		require.NoError(t, json.Unmarshal(raw, &extra))
		return extra
	}
	assertReset := func(extra map[string]any) {
		for _, key := range []string{
			"openai_compact_supported",
			"openai_compact_probe_version",
			"openai_compact_checked_at",
			"openai_compact_last_status",
			"openai_compact_last_error",
			"openai_compact_probe_observed_at_unix_nano",
		} {
			require.NotContains(t, extra, key)
		}
		require.Equal(t, "force_on", extra["openai_compact_mode"])
		require.Equal(t, false, extra["openai_responses_supported"])
		require.Equal(t, "keep", extra["unrelated"])
	}

	assertReset(loadExtra(activeOpenAI))
	assertReset(loadExtra(deletedOpenAI))
	require.Contains(t, loadExtra(otherPlatform), "openai_compact_supported")
}
