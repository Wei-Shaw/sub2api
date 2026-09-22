package migrations

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestChannelReasoningEffortMultipliersMigration(t *testing.T) {
	content, err := FS.ReadFile("239_channel_reasoning_effort_multipliers.sql")
	require.NoError(t, err)
	sql := strings.Join(strings.Fields(string(content)), " ")
	require.Contains(t, sql, "ALTER TABLE channel_model_pricing ADD COLUMN IF NOT EXISTS reasoning_effort_multipliers JSONB NOT NULL DEFAULT '{}'::jsonb")
	require.Contains(t, sql, "ALTER TABLE channel_account_stats_model_pricing ADD COLUMN IF NOT EXISTS reasoning_effort_multipliers JSONB NOT NULL DEFAULT '{}'::jsonb")
	require.Contains(t, sql, "WHERE max_reasoning_effort_multiplier IS NOT NULL")
	require.Contains(t, sql, "jsonb_build_object('max', max_reasoning_effort_multiplier)")
	require.Contains(t, sql, "attrelid = 'channel_model_pricing'::regclass AND attname = 'reasoning_effort_multipliers' AND NOT attisdropped")
	require.Less(t, strings.Index(sql, "UPDATE channel_model_pricing"), strings.Index(sql, "END IF;"))
	require.Contains(t, sql, "UPDATE groups AS g SET model_pricing")
	require.Contains(t, sql, "NOT (entry ? 'reasoning_effort_multipliers')")
	require.Contains(t, sql, "entry - 'max_reasoning_effort_multiplier'")
	require.Contains(t, sql, "END ORDER BY ordinal")
	require.NotContains(t, strings.ToLower(sql), "fable")
}
