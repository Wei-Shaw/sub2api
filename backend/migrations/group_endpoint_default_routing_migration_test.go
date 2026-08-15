package migrations

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

const endpointDefaultRoutingMigration = "222_group_endpoint_default_routing.sql"

func TestMigration222AddsColumnIdempotently(t *testing.T) {
	content, err := FS.ReadFile(endpointDefaultRoutingMigration)
	require.NoError(t, err)

	sql := string(content)
	require.Contains(t, sql, "ADD COLUMN IF NOT EXISTS endpoint_default_routing_enabled BOOLEAN NOT NULL DEFAULT false",
		"迁移必须可重复执行：已跑过分支上旧编号迁移的环境会已经有这一列")
}

// TestMigration222PreservesEveryGuardFromMigration193 is the regression this
// migration is most likely to break: the invalidation trigger is replaced
// wholesale with CREATE OR REPLACE, so copying the wrong base body would
// silently drop the profit-control and peak-window guards that 193 added.
func TestMigration222PreservesEveryGuardFromMigration193(t *testing.T) {
	previous, err := FS.ReadFile("193_group_profit_control_auth_cache_invalidation.sql")
	require.NoError(t, err)
	current, err := FS.ReadFile(endpointDefaultRoutingMigration)
	require.NoError(t, err)

	previousGuards := extractInvalidationGuardColumns(string(previous))
	currentGuards := extractInvalidationGuardColumns(string(current))

	require.NotEmpty(t, previousGuards)
	for _, guard := range previousGuards {
		require.Contains(t, currentGuards, guard,
			"迁移 222 的触发器函数体必须保留 193 的每一条判定，缺失的是 %s", guard)
	}
	require.Contains(t, currentGuards, "endpoint_default_routing_enabled",
		"新字段必须加入触发器判定，否则带外改动会留下陈旧的认证快照")
	require.Len(t, currentGuards, len(previousGuards)+1,
		"222 只应新增 endpoint_default_routing_enabled 一条判定")
}

func TestMigration222ReplacesTheSharedInvalidationFunction(t *testing.T) {
	content, err := FS.ReadFile(endpointDefaultRoutingMigration)
	require.NoError(t, err)

	sql := string(content)
	require.Contains(t, sql, "CREATE OR REPLACE FUNCTION enqueue_group_auth_cache_invalidation()")
	require.Contains(t, sql, "INSERT INTO auth_cache_invalidation_outbox (cache_key)")
	require.Contains(t, sql, "WHERE k.group_id = target_group_id")
}

// extractInvalidationGuardColumns returns the column names compared in the
// trigger's "nothing relevant changed, skip invalidation" predicate.
func extractInvalidationGuardColumns(sql string) []string {
	var columns []string
	for _, line := range strings.Split(sql, "\n") {
		line = strings.TrimSpace(line)
		if !strings.Contains(line, "IS NOT DISTINCT FROM") {
			continue
		}
		prefix := "OLD."
		start := strings.Index(line, prefix)
		if start < 0 {
			continue
		}
		rest := line[start+len(prefix):]
		end := strings.Index(rest, " ")
		if end < 0 {
			continue
		}
		columns = append(columns, rest[:end])
	}
	return columns
}
