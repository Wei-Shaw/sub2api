//go:build integration

package repository

import (
	"context"
	"testing"

	dbmigrations "github.com/Wei-Shaw/sub2api/migrations"
	"github.com/stretchr/testify/require"
)

func TestMigration234KeepsGroupPricingMixedWritersCompatible(t *testing.T) {
	tx := testTx(t)
	ctx := context.Background()
	migrationSQL, err := dbmigrations.FS.ReadFile("234_group_model_pricing.sql")
	require.NoError(t, err)

	// Reapply the complete migration twice. Operational replay must remain safe
	// even when the columns and trigger already exist.
	_, err = tx.ExecContext(ctx, string(migrationSQL))
	require.NoError(t, err)
	_, err = tx.ExecContext(ctx, string(migrationSQL))
	require.NoError(t, err)

	var legacyID int64
	var legacyEnabled bool
	require.NoError(t, tx.QueryRowContext(ctx, `
INSERT INTO groups (name, platform)
VALUES ('migration-234-legacy-writer', 'openai')
RETURNING id, long_context_pricing_enabled
`).Scan(&legacyID, &legacyEnabled))
	require.True(t, legacyEnabled, "an old binary which omits the new column must retain the historical enabled behavior")

	var modernID int64
	var modernEnabled bool
	require.NoError(t, tx.QueryRowContext(ctx, `
INSERT INTO groups (name, platform, long_context_pricing_enabled, model_pricing)
VALUES (
    'migration-234-modern-writer',
    'openai',
    FALSE,
    '[{"platform":"openai","models":["gpt-test"]}]'::jsonb
)
RETURNING id, long_context_pricing_enabled
`).Scan(&modernID, &modernEnabled))
	require.False(t, modernEnabled, "the trigger must not overwrite an explicit opt-out from the new binary")

	// Simulate the previous binary updating an unrelated field. Because the
	// trigger is scoped to UPDATE OF the new column, the opt-out must survive.
	_, err = tx.ExecContext(ctx, "UPDATE groups SET description = 'legacy update' WHERE id = $1", modernID)
	require.NoError(t, err)
	require.NoError(t, tx.QueryRowContext(ctx, `
SELECT long_context_pricing_enabled
FROM groups
WHERE id = $1
`, modernID).Scan(&modernEnabled))
	require.False(t, modernEnabled)

	// An explicit NULL is normalized to the historical default as documented by
	// the migration. This also proves the UPDATE OF trigger remains installed.
	_, err = tx.ExecContext(ctx, "UPDATE groups SET long_context_pricing_enabled = NULL WHERE id = $1", modernID)
	require.NoError(t, err)
	require.NoError(t, tx.QueryRowContext(ctx, `
SELECT long_context_pricing_enabled
FROM groups
WHERE id = $1
`, modernID).Scan(&modernEnabled))
	require.True(t, modernEnabled)

	var pricingJSON string
	require.NoError(t, tx.QueryRowContext(ctx, `
SELECT model_pricing::text
FROM groups
WHERE id = $1
`, modernID).Scan(&pricingJSON))
	require.JSONEq(t, `[{"platform":"openai","models":["gpt-test"]}]`, pricingJSON)
}
