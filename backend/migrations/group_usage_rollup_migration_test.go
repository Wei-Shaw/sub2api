//go:build unit

package migrations

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMigration222CreatesGroupUsageRollups(t *testing.T) {
	content, err := FS.ReadFile("222_group_usage_daily_rollups.sql")
	require.NoError(t, err)

	sql := string(content)
	require.Contains(t, sql, "CREATE TABLE IF NOT EXISTS usage_group_daily_rollups")
	require.Contains(t, sql, "actual_cost DECIMAL(20, 10)")
	require.Contains(t, sql, "PRIMARY KEY (bucket_date, group_id)")
	require.Contains(t, sql, "CREATE TABLE IF NOT EXISTS usage_group_rollup_state")
	require.Contains(t, sql, "CHECK (id = 1)")
	require.Contains(t, sql, "TIMESTAMPTZ '1970-01-01 00:00:00+00'")
	require.Contains(t, sql, "ON CONFLICT (id) DO NOTHING")
}

func TestMigration222InvalidatesClosedBucketsWhenUsageLogsChange(t *testing.T) {
	content, err := FS.ReadFile("222_group_usage_daily_rollups.sql")
	require.NoError(t, err)

	sql := string(content)
	require.Contains(t, sql, "CREATE OR REPLACE FUNCTION invalidate_group_usage_rollup_state")
	require.Contains(t, sql, "SELECT closed_before")
	require.Contains(t, sql, "FOR UPDATE")
	require.Contains(t, sql, "FOR KEY SHARE")
	require.Contains(t, sql, "REFERENCING NEW TABLE AS inserted_usage_logs")
	require.Contains(t, sql, "closed_before = LEAST(closed_before, affected_date)")
	require.Contains(t, sql, "CREATE TRIGGER usage_logs_group_rollup_invalidate_insert")
	require.Contains(t, sql, "CREATE TRIGGER usage_logs_group_rollup_invalidate_delete")
	require.Contains(t, sql, "CREATE TRIGGER usage_logs_group_rollup_invalidate_update")
	require.Contains(t, sql, "AFTER UPDATE OF created_at, group_id, actual_cost")
}

func TestMigration223TracksConfiguredTimezone(t *testing.T) {
	content, err := FS.ReadFile("223_group_usage_rollup_timezone.sql")
	require.NoError(t, err)

	sql := string(content)
	require.Contains(t, sql, "ADD COLUMN IF NOT EXISTS timezone_name TEXT")
	require.Contains(t, sql, "DEFAULT 'Asia/Shanghai'")
	require.Contains(t, sql, "current_setting('TimeZone')")
	require.Contains(t, sql, "CREATE OR REPLACE FUNCTION invalidate_group_usage_rollup_state")
	require.Contains(t, sql, "CREATE OR REPLACE FUNCTION invalidate_group_usage_rollup_state_after_insert")
}

func TestMigration231AddsRetentionBarrier(t *testing.T) {
	content, err := FS.ReadFile("231_group_usage_rollup_archival.sql")
	require.NoError(t, err)

	sql := string(content)
	require.Contains(t, sql, "CREATE OR REPLACE FUNCTION invalidate_group_usage_rollup_state")
	require.Contains(t, sql, "CREATE OR REPLACE FUNCTION invalidate_group_usage_rollup_state_after_insert")
	// UPDATE 的旧值与新值必须分别按归档屏障过滤，跨屏障移动仍会失效保留侧。
	require.Contains(t, sql, "old_date >= archived_before")
	require.Contains(t, sql, "new_date >= archived_before")
	require.Contains(t, sql, "(retained_from AT TIME ZONE configured_timezone)::date")
	// 屏障判定不加锁，避免批量归档删除逐行争抢状态行。
	require.Contains(t, sql, "SELECT (retained_from AT TIME ZONE configured_timezone)::date")
	// 批量 INSERT 必须先排除归档行，再从保留侧求最早失效日期。
	require.Contains(t, sql, "::date >= archived_before")
	// 存量修复：把被清理拉坏的水位收回到屏障之后。
	require.Contains(t, sql, "closed_before < (retained_from AT TIME ZONE timezone_name)::date")
	// group 与 api_key 共用水位，触发器必须覆盖只有 API key 的日志变化。
	require.Contains(t, sql, "(group_id IS NOT NULL OR api_key_id IS NOT NULL)")
	require.Contains(t, sql, "WHEN (OLD.group_id IS NOT NULL OR OLD.api_key_id IS NOT NULL)")
	require.Contains(t, sql, "AFTER UPDATE OF created_at, group_id, api_key_id, actual_cost")
	require.Contains(t, sql, "OLD.api_key_id IS DISTINCT FROM NEW.api_key_id")
}

func TestMigration232CreatesAPIKeyRollupsAndRewindsForBackfill(t *testing.T) {
	content, err := FS.ReadFile("232_usage_apikey_daily_rollups.sql")
	require.NoError(t, err)

	sql := string(content)
	require.Contains(t, sql, "CREATE TABLE IF NOT EXISTS usage_apikey_daily_rollups")
	require.Contains(t, sql, "PRIMARY KEY (bucket_date, api_key_id)")
	require.Contains(t, sql, "idx_usage_apikey_daily_rollups_key_date")
	require.Contains(t, sql, "SET closed_before = LEAST")
	require.Contains(t, sql, "(retained_from AT TIME ZONE timezone_name)::date")
}
