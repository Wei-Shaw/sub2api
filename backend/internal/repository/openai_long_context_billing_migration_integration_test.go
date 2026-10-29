//go:build integration

package repository

import (
	"context"
	"testing"

	dbmigrations "github.com/Wei-Shaw/sub2api/migrations"
	"github.com/stretchr/testify/require"
)

func TestMigration175EnforcesOpenAILongContextBillingWriteInvariant(t *testing.T) {
	tx := testTx(t)
	ctx := context.Background()
	migrationSQL, err := dbmigrations.FS.ReadFile("175_default_openai_long_context_billing.sql")
	require.NoError(t, err)
	_, err = tx.ExecContext(ctx, `
DROP TRIGGER IF EXISTS accounts_propagate_openai_long_context_billing_extra ON accounts;
DROP TRIGGER IF EXISTS accounts_enforce_openai_long_context_billing_extra ON accounts;
`)
	require.NoError(t, err)

	var ordinaryID int64
	require.NoError(t, tx.QueryRowContext(ctx, `
INSERT INTO accounts (name, platform, type, extra)
VALUES ('migration-175-ordinary', 'openai', 'oauth', '{}'::jsonb)
RETURNING id
`).Scan(&ordinaryID))

	var parentID int64
	require.NoError(t, tx.QueryRowContext(ctx, `
INSERT INTO accounts (name, platform, type, extra)
VALUES ('migration-175-parent', 'openai', 'oauth', '{"openai_long_context_billing_enabled":false}'::jsonb)
RETURNING id
`).Scan(&parentID))

	var shadowID int64
	require.NoError(t, tx.QueryRowContext(ctx, `
INSERT INTO accounts (name, platform, type, extra, parent_account_id, quota_dimension)
VALUES ('migration-175-shadow', 'openai', 'oauth', '{}'::jsonb, $1, 'spark')
RETURNING id
`, parentID).Scan(&shadowID))

	var malformedLegacyID int64
	require.NoError(t, tx.QueryRowContext(ctx, `
INSERT INTO accounts (name, platform, type, extra)
VALUES ('migration-175-malformed-legacy', 'openai', 'oauth', '{"openai_long_context_billing_enabled":"false"}'::jsonb)
RETURNING id
`).Scan(&malformedLegacyID))

	_, err = tx.ExecContext(ctx, string(migrationSQL))
	require.NoError(t, err)
	_, err = tx.ExecContext(ctx, string(migrationSQL))
	require.NoError(t, err)

	var ordinaryEnabled bool
	require.NoError(t, tx.QueryRowContext(ctx, `
SELECT (extra->>'openai_long_context_billing_enabled')::boolean
FROM accounts
WHERE id = $1
`, ordinaryID).Scan(&ordinaryEnabled))
	require.False(t, ordinaryEnabled)

	var shadowEnabled bool
	require.NoError(t, tx.QueryRowContext(ctx, `
SELECT (extra->>'openai_long_context_billing_enabled')::boolean
FROM accounts
WHERE id = $1
`, shadowID).Scan(&shadowEnabled))
	require.False(t, shadowEnabled)

	var initialShadowOutboxEvents int
	require.NoError(t, tx.QueryRowContext(ctx, `
SELECT COUNT(*)
FROM scheduler_outbox
WHERE event_type = 'account_changed' AND account_id = $1
`, shadowID).Scan(&initialShadowOutboxEvents))
	require.Equal(t, 1, initialShadowOutboxEvents)

	var malformedLegacyEnabled bool
	require.NoError(t, tx.QueryRowContext(ctx, `
SELECT (extra->>'openai_long_context_billing_enabled')::boolean
FROM accounts
WHERE id = $1
`, malformedLegacyID).Scan(&malformedLegacyEnabled))
	require.False(t, malformedLegacyEnabled)
	_, err = tx.ExecContext(ctx, `
UPDATE accounts
SET extra = extra || '{"migration_175_unrelated_update":true}'::jsonb
WHERE id = $1
`, malformedLegacyID)
	require.NoError(t, err)

	_, err = tx.ExecContext(ctx, "TRUNCATE scheduler_outbox")
	require.NoError(t, err)
	_, err = tx.ExecContext(ctx, `
UPDATE accounts
SET extra = '{"legacy_writer_replaced_extra":true}'::jsonb
WHERE id = $1
`, parentID)
	require.NoError(t, err)
	var parentEnabled bool
	require.NoError(t, tx.QueryRowContext(ctx, `
SELECT (extra->>'openai_long_context_billing_enabled')::boolean
FROM accounts
WHERE id = $1
`, parentID).Scan(&parentEnabled))
	require.False(t, parentEnabled)
	require.NoError(t, tx.QueryRowContext(ctx, `
SELECT (extra->>'openai_long_context_billing_enabled')::boolean
FROM accounts
WHERE id = $1
`, shadowID).Scan(&shadowEnabled))
	require.False(t, shadowEnabled)
	var preservedOptOutEvents int
	require.NoError(t, tx.QueryRowContext(ctx, `
SELECT COUNT(*)
FROM scheduler_outbox
WHERE event_type = 'account_changed' AND account_id = $1
`, shadowID).Scan(&preservedOptOutEvents))
	require.Zero(t, preservedOptOutEvents)

	require.NoError(t, tx.QueryRowContext(ctx, `
INSERT INTO accounts (name, platform, type, extra)
VALUES ('migration-175-rolling-writer', 'openai', 'oauth', '{}'::jsonb)
RETURNING (extra->>'openai_long_context_billing_enabled')::boolean
`).Scan(&ordinaryEnabled))
	require.False(t, ordinaryEnabled)

	_, err = tx.ExecContext(ctx, "TRUNCATE scheduler_outbox")
	require.NoError(t, err)
	_, err = tx.ExecContext(ctx, `
UPDATE accounts
SET extra = jsonb_set(extra, '{openai_long_context_billing_enabled}', 'true'::jsonb, true)
WHERE id = $1
`, parentID)
	require.NoError(t, err)
	require.NoError(t, tx.QueryRowContext(ctx, `
SELECT (extra->>'openai_long_context_billing_enabled')::boolean
FROM accounts
WHERE id = $1
`, shadowID).Scan(&shadowEnabled))
	require.True(t, shadowEnabled)

	var shadowOutboxEvents int
	require.NoError(t, tx.QueryRowContext(ctx, `
SELECT COUNT(*)
FROM scheduler_outbox
WHERE event_type = 'account_changed' AND account_id = $1
`, shadowID).Scan(&shadowOutboxEvents))
	require.Equal(t, 1, shadowOutboxEvents)

	_, err = tx.ExecContext(ctx, `
INSERT INTO accounts (name, platform, type, extra)
VALUES ('migration-175-malformed', 'openai', 'oauth', '{"openai_long_context_billing_enabled":"false"}'::jsonb)
`)
	require.ErrorContains(t, err, "openai_long_context_billing_enabled must be a boolean")
}
