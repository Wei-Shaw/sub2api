//go:build integration

package repository

import (
	"context"
	"testing"

	dbmigrations "github.com/Wei-Shaw/sub2api/migrations"
	"github.com/stretchr/testify/require"
)

func TestMigration239ReasoningEffortMultipliers(t *testing.T) {
	tx := testTx(t)
	ctx := context.Background()
	_, err := tx.ExecContext(ctx, `ALTER TABLE channel_model_pricing DROP COLUMN reasoning_effort_multipliers`)
	require.NoError(t, err)
	_, err = tx.ExecContext(ctx, `ALTER TABLE channel_account_stats_model_pricing DROP COLUMN reasoning_effort_multipliers`)
	require.NoError(t, err)

	var channelID, configuredID, unsetID int64
	require.NoError(t, tx.QueryRowContext(ctx, `INSERT INTO channels (name) VALUES ('migration-reasoning-multipliers') RETURNING id`).Scan(&channelID))
	require.NoError(t, tx.QueryRowContext(ctx, `
INSERT INTO channel_model_pricing (channel_id, models, max_reasoning_effort_multiplier)
VALUES ($1, '["custom-model"]', 2.5) RETURNING id`, channelID).Scan(&configuredID))
	require.NoError(t, tx.QueryRowContext(ctx, `
INSERT INTO channel_model_pricing (channel_id, models)
VALUES ($1, '["claude-fable-5-1"]') RETURNING id`, channelID).Scan(&unsetID))

	migrationSQL, err := dbmigrations.FS.ReadFile("239_channel_reasoning_effort_multipliers.sql")
	require.NoError(t, err)
	_, err = tx.ExecContext(ctx, string(migrationSQL))
	require.NoError(t, err)
	var configured, unset string
	require.NoError(t, tx.QueryRowContext(ctx, `SELECT reasoning_effort_multipliers::text FROM channel_model_pricing WHERE id = $1`, configuredID).Scan(&configured))
	require.JSONEq(t, `{"max":2.5}`, configured)
	require.NoError(t, tx.QueryRowContext(ctx, `SELECT reasoning_effort_multipliers::text FROM channel_model_pricing WHERE id = $1`, unsetID).Scan(&unset))
	require.JSONEq(t, `{}`, unset)

	// Changing or clearing the generic configuration must survive a migration replay.
	for _, current := range []string{`{"high":1.5,"max":4}`, `{"low":0.75}`, `{}`} {
		_, err = tx.ExecContext(ctx, `UPDATE channel_model_pricing SET reasoning_effort_multipliers = $1 WHERE id = $2`, current, configuredID)
		require.NoError(t, err)
		_, err = tx.ExecContext(ctx, string(migrationSQL))
		require.NoError(t, err)
		require.NoError(t, tx.QueryRowContext(ctx, `SELECT reasoning_effort_multipliers::text FROM channel_model_pricing WHERE id = $1`, configuredID).Scan(&configured))
		require.JSONEq(t, current, configured)
	}

	// Account statistics use the same empty-by-default shape.
	var ruleID int64
	require.NoError(t, tx.QueryRowContext(ctx, `INSERT INTO channel_account_stats_pricing_rules (channel_id) VALUES ($1) RETURNING id`, channelID).Scan(&ruleID))
	var accountStatsDefault string
	require.NoError(t, tx.QueryRowContext(ctx, `INSERT INTO channel_account_stats_model_pricing (rule_id) VALUES ($1) RETURNING reasoning_effort_multipliers::text`, ruleID).Scan(&accountStatsDefault))
	require.JSONEq(t, `{}`, accountStatsDefault)
}

func TestMigration239GroupReasoningEffortMultipliers(t *testing.T) {
	tx := testTx(t)
	ctx := context.Background()
	var groupID int64
	require.NoError(t, tx.QueryRowContext(ctx, `
INSERT INTO groups (name, platform, model_pricing)
VALUES ('migration-group-reasoning', 'anthropic', '[
    {"models":["custom-model"],"input_price":2,"max_reasoning_effort_multiplier":2.5},
    {"models":["existing-map"],"max_reasoning_effort_multiplier":3,"reasoning_effort_multipliers":{"high":1.5,"max":4}},
    {"models":["cleared-map"],"max_reasoning_effort_multiplier":3,"reasoning_effort_multipliers":{}},
    {"models":["claude-fable-5-1"],"max_reasoning_effort_multiplier":null}
]'::jsonb) RETURNING id`).Scan(&groupID))

	migrationSQL, err := dbmigrations.FS.ReadFile("239_channel_reasoning_effort_multipliers.sql")
	require.NoError(t, err)
	for range 2 {
		_, err = tx.ExecContext(ctx, string(migrationSQL))
		require.NoError(t, err)
		var actual string
		require.NoError(t, tx.QueryRowContext(ctx, `SELECT model_pricing::text FROM groups WHERE id = $1`, groupID).Scan(&actual))
		require.JSONEq(t, `[
            {"models":["custom-model"],"input_price":2,"reasoning_effort_multipliers":{"max":2.5}},
            {"models":["existing-map"],"reasoning_effort_multipliers":{"high":1.5,"max":4}},
            {"models":["cleared-map"],"reasoning_effort_multipliers":{}},
            {"models":["claude-fable-5-1"]}
        ]`, actual)
	}
}
