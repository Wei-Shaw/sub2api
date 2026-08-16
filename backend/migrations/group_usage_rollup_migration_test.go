//go:build unit

package migrations

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMigration236CreatesGroupUsageRollups(t *testing.T) {
	content, err := FS.ReadFile("236_group_usage_daily_rollups.sql")
	require.NoError(t, err)

	sql := string(content)
	require.Contains(t, sql, "CREATE TABLE IF NOT EXISTS usage_group_daily_rollups")
	require.Contains(t, sql, "actual_cost DECIMAL(20, 10)")
	require.Contains(t, sql, "PRIMARY KEY (bucket_date, group_id)")
	require.Contains(t, sql, "CREATE TABLE IF NOT EXISTS usage_group_rollup_state")
	require.Contains(t, sql, "CHECK (id = 1)")
	require.Contains(t, sql, "'1970-01-01 00:00:00+00'")
	require.Contains(t, sql, "ON CONFLICT (id) DO NOTHING")
}

func TestMigration236InvalidatesClosedBucketsWhenUsageLogsChange(t *testing.T) {
	content, err := FS.ReadFile("236_group_usage_daily_rollups.sql")
	require.NoError(t, err)

	sql := string(content)
	require.Contains(t, sql, "CREATE OR REPLACE FUNCTION invalidate_group_usage_rollup_state")
	require.Contains(t, sql, "SELECT closed_before")
	require.Contains(t, sql, "FOR UPDATE")
	require.Contains(t, sql, "FOR KEY SHARE")
	require.Contains(t, sql, "REFERENCING NEW TABLE AS inserted_usage_logs")
	require.Contains(t, sql, "closed_before = LEAST(closed_before, affected_date)")
	require.Contains(t, sql, "CREATE OR REPLACE TRIGGER usage_logs_group_rollup_invalidate_insert")
	require.Contains(t, sql, "CREATE OR REPLACE TRIGGER usage_logs_group_rollup_invalidate_delete")
	require.Contains(t, sql, "CREATE OR REPLACE TRIGGER usage_logs_group_rollup_invalidate_update")
	require.Contains(t, sql, "AFTER UPDATE OF created_at, group_id, actual_cost")
}

func TestMigration237TracksConfiguredTimezone(t *testing.T) {
	content, err := FS.ReadFile("237_group_usage_rollup_timezone.sql")
	require.NoError(t, err)

	sql := string(content)
	require.Contains(t, sql, "ADD COLUMN IF NOT EXISTS timezone_name TEXT")
	require.Contains(t, sql, "sub2api-managed-update: reviewed-compatible")
	require.Contains(t, sql, "SET timezone_name = 'Asia/Shanghai'")
	require.Contains(t, sql, "current_setting('TimeZone')")
	require.Contains(t, sql, "CREATE OR REPLACE FUNCTION invalidate_group_usage_rollup_state")
	require.Contains(t, sql, "CREATE OR REPLACE FUNCTION invalidate_group_usage_rollup_state_after_insert")
}

func TestMigration236MarksReviewedCompatibleTriggers(t *testing.T) {
	content, err := FS.ReadFile("236_group_usage_daily_rollups.sql")
	require.NoError(t, err)
	sql := strings.ToLower(string(content))
	require.Contains(t, sql, "sub2api-managed-update: reviewed-compatible")
	require.Contains(t, sql, "create or replace function invalidate_group_usage_rollup_state()")
	require.Contains(t, sql, "create or replace trigger usage_logs_group_rollup_invalidate_insert")
	require.NotContains(t, sql, "drop trigger")
}

func TestMigration238KeepsKeywordColumnRollbackCompatible(t *testing.T) {
	content, err := FS.ReadFile("238_prompt_audit_keywords.sql")
	require.NoError(t, err)
	sql := strings.ToLower(string(content))
	require.Contains(t, sql, "add column if not exists matched_keyword varchar(200)")
	require.NotContains(t, sql, "not null")
	require.Contains(t, sql, "sub2api-managed-update: reviewed-compatible")
	require.Contains(t, sql, "create or replace function sub2api_default_prompt_audit_matched_keyword()")
	require.NotContains(t, sql, "create index")
}

func TestMigration239CreatesKeywordIndexConcurrently(t *testing.T) {
	content, err := FS.ReadFile("239_prompt_audit_keywords_index_notx.sql")
	require.NoError(t, err)
	sql := strings.ToLower(string(content))
	require.Contains(t, sql, "create index concurrently if not exists idx_prompt_audit_events_matched_keyword")
}
