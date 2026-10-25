//go:build integration

package repository

import (
	"context"
	"database/sql"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

const groupModelAllowlistRepairMigration = "236_group_model_allowlist_repair.sql"

// 236 是可重放的修复迁移：235 的重命名一旦被记账就不会重跑，数据库若回到旧结构
// （手工改回列名、按旧结构部分恢复）应用仍能启动，但所有关联 groups 的查询都会
// 报 column groups.model_allowlist does not exist（issue #6780）。
// MySQL 方言按 migrations.MySQLFS 的同名镜像迁移验证等价语义（临时存储过程版）。
func TestMigration236RenamesLegacyModelsListConfigColumn(t *testing.T) {
	tx := testTx(t)
	ctx := context.Background()

	normalizeMySQLGroupsAllowlistColumns(ctx, t, tx)
	_, err := tx.ExecContext(ctx, "ALTER TABLE "+groupsTableRef()+" RENAME COLUMN model_allowlist TO models_list_config")
	require.NoError(t, err)

	groupID, err := insertReturningID(ctx, tx, `
INSERT INTO `+groupsTableRef()+` (name, platform, rate_multiplier, status, models_list_config)
VALUES ('migration-236-rename', 'anthropic', 1, 'active', '{"enabled":true,"models":["claude-sonnet-5"]}')`)
	require.NoError(t, err)
	cleanupMySQLGroupRows(t, groupID)

	applyGroupModelAllowlistRepair(ctx, t, tx)

	// 重命名保留原数据，且新列恢复 NOT NULL DEFAULT '{}' 的形状。
	var allowlist string
	require.NoError(t, tx.QueryRowContext(ctx,
		"SELECT "+jsonTextExpr("model_allowlist")+" FROM "+groupsTableRef()+" WHERE id = "+ph(1), groupID).Scan(&allowlist))
	require.JSONEq(t, `{"enabled":true,"models":["claude-sonnet-5"]}`, allowlist)
	requireModelAllowlistColumnShape(ctx, t, tx)

	// 可重放：重复执行不报错也不改变结果。
	applyGroupModelAllowlistRepair(ctx, t, tx)
	require.NoError(t, tx.QueryRowContext(ctx,
		"SELECT "+jsonTextExpr("model_allowlist")+" FROM "+groupsTableRef()+" WHERE id = "+ph(1), groupID).Scan(&allowlist))
	require.JSONEq(t, `{"enabled":true,"models":["claude-sonnet-5"]}`, allowlist)
}

func TestMigration236BackfillsWhenBothColumnsExist(t *testing.T) {
	tx := testTx(t)
	ctx := context.Background()

	normalizeMySQLGroupsAllowlistColumns(ctx, t, tx)
	addColumn := "ALTER TABLE groups ADD COLUMN models_list_config JSONB NOT NULL DEFAULT '{}'::jsonb"
	if IsMySQL() {
		addColumn = "ALTER TABLE `groups` ADD COLUMN models_list_config JSON NOT NULL DEFAULT ('{}')"
	}
	_, err := tx.ExecContext(ctx, addColumn)
	require.NoError(t, err)

	// 新列仍是默认空值：旧列里的配置应该被补回来。
	staleID, err := insertReturningID(ctx, tx, `
INSERT INTO `+groupsTableRef()+` (name, platform, rate_multiplier, status, model_allowlist, models_list_config)
VALUES ('migration-236-backfill', 'anthropic', 1, 'active', '{}', '{"enabled":true,"models":["legacy-model"]}')`)
	require.NoError(t, err)
	cleanupMySQLGroupRows(t, staleID)

	// 新列已有配置：不能被旧列覆盖。
	currentID, err := insertReturningID(ctx, tx, `
INSERT INTO `+groupsTableRef()+` (name, platform, rate_multiplier, status, model_allowlist, models_list_config)
VALUES ('migration-236-keep', 'anthropic', 1, 'active', '{"enabled":true,"models":["current-model"]}', '{"enabled":true,"models":["legacy-model"]}')`)
	require.NoError(t, err)
	cleanupMySQLGroupRows(t, currentID)

	applyGroupModelAllowlistRepair(ctx, t, tx)

	var backfilled, kept string
	require.NoError(t, tx.QueryRowContext(ctx,
		"SELECT "+jsonTextExpr("model_allowlist")+" FROM "+groupsTableRef()+" WHERE id = "+ph(1), staleID).Scan(&backfilled))
	require.JSONEq(t, `{"enabled":true,"models":["legacy-model"]}`, backfilled)
	require.NoError(t, tx.QueryRowContext(ctx,
		"SELECT "+jsonTextExpr("model_allowlist")+" FROM "+groupsTableRef()+" WHERE id = "+ph(1), currentID).Scan(&kept))
	require.JSONEq(t, `{"enabled":true,"models":["current-model"]}`, kept)
}

func TestMigration236RecreatesMissingModelAllowlistColumn(t *testing.T) {
	tx := testTx(t)
	ctx := context.Background()

	normalizeMySQLGroupsAllowlistColumns(ctx, t, tx)
	_, err := tx.ExecContext(ctx, "ALTER TABLE "+groupsTableRef()+" DROP COLUMN model_allowlist")
	require.NoError(t, err)

	groupID, err := insertReturningID(ctx, tx, `
INSERT INTO `+groupsTableRef()+` (name, platform, rate_multiplier, status)
VALUES ('migration-236-recreate', 'anthropic', 1, 'active')`)
	require.NoError(t, err)
	cleanupMySQLGroupRows(t, groupID)

	applyGroupModelAllowlistRepair(ctx, t, tx)

	var allowlist string
	require.NoError(t, tx.QueryRowContext(ctx,
		"SELECT "+jsonTextExpr("model_allowlist")+" FROM "+groupsTableRef()+" WHERE id = "+ph(1), groupID).Scan(&allowlist))
	require.JSONEq(t, `{}`, allowlist)
	requireModelAllowlistColumnShape(ctx, t, tx)
}

func applyGroupModelAllowlistRepair(ctx context.Context, t *testing.T, tx *sql.Tx) {
	t.Helper()

	_, err := tx.ExecContext(ctx, readMigrationSQLForDialect(t, groupModelAllowlistRepairMigration))
	require.NoError(t, err)
}

func requireModelAllowlistColumnShape(ctx context.Context, t *testing.T, tx *sql.Tx) {
	t.Helper()

	schemaFilter := ""
	if IsMySQL() {
		schemaFilter = " AND table_schema = DATABASE()"
	}
	var isNullable, columnDefault string
	require.NoError(t, tx.QueryRowContext(ctx, `
SELECT is_nullable, COALESCE(column_default, '')
FROM information_schema.columns
WHERE table_name = 'groups' AND column_name = 'model_allowlist'`+schemaFilter).Scan(&isNullable, &columnDefault))
	require.Equal(t, "NO", isNullable)
	wantDefault := "'{}'::jsonb"
	if IsMySQL() {
		// MySQL 表达式默认值在 information_schema 中呈现为 ('{}')，
		// 且引号会带转义（如 _utf8mb4\'{}\'），先去掉反斜杠再断言。
		columnDefault = strings.ReplaceAll(columnDefault, `\`, "")
		wantDefault = "'{}'"
	}
	require.Contains(t, columnDefault, wantDefault)
}

// mysqlColumnExists 报告当前库中指定列是否存在（MySQL 专用，用于替代
// PG 的 ADD/DROP COLUMN IF EXISTS 可重入语义）。
func mysqlColumnExists(ctx context.Context, t *testing.T, db testSQLDB, table, column string) bool {
	t.Helper()

	var count int
	require.NoError(t, db.QueryRowContext(ctx, `
SELECT COUNT(*)
FROM information_schema.columns
WHERE table_schema = DATABASE() AND table_name = ? AND column_name = ?`, table, column).Scan(&count))
	return count > 0
}

// normalizeMySQLGroupsAllowlistColumns 在 MySQL 下把 groups 表恢复到 236 之前的
// 规范状态（model_allowlist 存在、models_list_config 不存在）。
// MySQL 的 ALTER 等 DDL 隐式提交，testTx 的回滚无法撤销上次运行残留的状态，
// PG 版测试则天然依赖每测试事务回滚获得的干净起点。
func normalizeMySQLGroupsAllowlistColumns(ctx context.Context, t *testing.T, tx *sql.Tx) {
	t.Helper()
	if !IsMySQL() {
		return
	}

	oldExists := mysqlColumnExists(ctx, t, tx, "groups", "models_list_config")
	newExists := mysqlColumnExists(ctx, t, tx, "groups", "model_allowlist")
	switch {
	case oldExists && !newExists:
		_, err := tx.ExecContext(ctx, "ALTER TABLE `groups` RENAME COLUMN models_list_config TO model_allowlist")
		require.NoError(t, err)
	case oldExists && newExists:
		_, err := tx.ExecContext(ctx, "ALTER TABLE `groups` DROP COLUMN models_list_config")
		require.NoError(t, err)
	}
}

// cleanupMySQLGroupRows 在 MySQL 下显式删除本测试提交的 groups 夹具行
// （DDL 隐式提交使行不再随 testTx 回滚）；PG 侧沿用事务回滚，行为不变。
func cleanupMySQLGroupRows(t *testing.T, groupIDs ...int64) {
	t.Helper()
	if !IsMySQL() {
		return
	}

	t.Cleanup(func() {
		ctx := context.Background()
		for _, id := range groupIDs {
			_, err := integrationDB.ExecContext(ctx, deleteByIDSQL("groups"), id)
			require.NoError(t, err)
		}
	})
}
