package migrations

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMigration175DefaultsOrdinaryOpenAIAndInheritsForSparkShadows(t *testing.T) {
	content, err := FS.ReadFile("175_default_openai_long_context_billing.sql")
	require.NoError(t, err)

	sql := string(content)
	require.Contains(t, sql, "parent_account_id IS NULL")
	require.Contains(t, sql, "quota_dimension = 'spark'")
	require.Contains(t, sql, "parent.extra")
	require.Contains(t, sql, "jsonb_typeof")
	require.Contains(t, sql, "openai_long_context_billing_enabled")
}

func TestMigration175GuardsMixedVersionAccountWrites(t *testing.T) {
	content, err := FS.ReadFile("175_default_openai_long_context_billing.sql")
	require.NoError(t, err)

	sql := string(content)
	require.Contains(t, sql, "RETURNS TRIGGER")
	require.Contains(t, sql, "BEFORE INSERT OR UPDATE")
	require.Contains(t, sql, "CREATE TRIGGER")
	require.Contains(t, sql, "must be a boolean")
	require.Contains(t, sql, "INSERT INTO scheduler_outbox")
	require.Contains(t, sql, "'account_changed'")
	require.Contains(t, sql, "jsonb_typeof(extra->'openai_long_context_billing_enabled') IS DISTINCT FROM 'boolean'")
	require.Contains(t, sql, "WITH shadow_values AS")
	require.Contains(t, sql, "TG_OP = 'UPDATE'")
	require.Contains(t, sql, "OLD.extra->'openai_long_context_billing_enabled'")
}
