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

func TestMigration251AddsExpiredFloorAndNonBlockingBuildMarks(t *testing.T) {
	content, err := FS.ReadFile("251_group_usage_rollup_nonblocking_build.sql")
	require.NoError(t, err)

	sql := string(content)
	require.Contains(t, sql, "SET LOCAL lock_timeout")
	require.Contains(t, sql, "ADD COLUMN IF NOT EXISTS expired_before TIMESTAMPTZ NOT NULL")
	require.Contains(t, sql, "ADD COLUMN IF NOT EXISTS building_until DATE")
	require.Contains(t, sql, "ADD COLUMN IF NOT EXISTS build_dirty_from DATE")
	require.Contains(t, sql, "CREATE OR REPLACE FUNCTION apply_group_usage_rollup_invalidation")
	require.Contains(t, sql, "CREATE OR REPLACE FUNCTION invalidate_group_usage_rollups_at")
	require.Contains(t, sql, "CREATE OR REPLACE FUNCTION invalidate_group_usage_rollup_state()")
	require.Contains(t, sql, "CREATE OR REPLACE FUNCTION invalidate_group_usage_rollup_state_after_insert()")
	require.Contains(t, sql, "created_at >= state_expired_before")
	// 源记录变更只对状态行持 KEY SHARE，与 usage_logs 写入互不阻塞。
	require.Contains(t, sql, "FOR KEY SHARE")
	require.NotContains(t, sql, "FOR UPDATE;")
}
