//go:build unit

package migrations

import (
	"strings"
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

func TestMigration227AddsRetentionBarrier(t *testing.T) {
	content, err := FS.ReadFile("227_group_usage_rollup_archival.sql")
	require.NoError(t, err)

	sql := string(content)
	require.Contains(t, sql, "CREATE OR REPLACE FUNCTION invalidate_group_usage_rollup_state")
	require.Contains(t, sql, "CREATE OR REPLACE FUNCTION invalidate_group_usage_rollup_state_after_insert")
	// 归档屏障：两个失效函数都必须在回退水位前跳过已归档区间。
	require.Equal(t, 2, strings.Count(sql, "affected_date < archived_before"))
	require.Contains(t, sql, "(retained_from AT TIME ZONE configured_timezone)::date")
	// 屏障判定不加锁，避免批量归档删除逐行争抢状态行。
	require.Contains(t, sql, "SELECT (retained_from AT TIME ZONE configured_timezone)::date")
	// 存量修复：把被清理拉坏的水位收回到屏障之后。
	require.Contains(t, sql, "closed_before < (retained_from AT TIME ZONE timezone_name)::date")
}
