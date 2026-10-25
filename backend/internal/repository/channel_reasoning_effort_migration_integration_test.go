//go:build integration

package repository

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMigration239ReasoningEffortMultipliers(t *testing.T) {
	tx := testTx(t)
	ctx := context.Background()
	_, err := tx.ExecContext(ctx, `ALTER TABLE channel_model_pricing DROP COLUMN reasoning_effort_multipliers`)
	require.NoError(t, err)
	_, err = tx.ExecContext(ctx, `ALTER TABLE channel_account_stats_model_pricing DROP COLUMN reasoning_effort_multipliers`)
	require.NoError(t, err)

	channelID, err := insertReturningID(ctx, tx, `INSERT INTO channels (name) VALUES ('migration-reasoning-multipliers')`)
	require.NoError(t, err)
	cleanupMySQLChannelRows(t, channelID)
	configuredID, err := insertReturningID(ctx, tx, `
INSERT INTO channel_model_pricing (channel_id, models, max_reasoning_effort_multiplier)
VALUES (`+ph(1)+`, '["custom-model"]', 2.5)`, channelID)
	require.NoError(t, err)
	unsetID, err := insertReturningID(ctx, tx, `
INSERT INTO channel_model_pricing (channel_id, models)
VALUES (`+ph(1)+`, '["claude-fable-5-1"]')`, channelID)
	require.NoError(t, err)

	_, err = tx.ExecContext(ctx, readMigrationSQLForDialect(t, "239_channel_reasoning_effort_multipliers.sql"))
	require.NoError(t, err)
	var configured, unset string
	require.NoError(t, tx.QueryRowContext(ctx,
		`SELECT `+jsonTextExpr("reasoning_effort_multipliers")+` FROM channel_model_pricing WHERE id = `+ph(1), configuredID).Scan(&configured))
	require.JSONEq(t, `{"max":2.5}`, configured)
	require.NoError(t, tx.QueryRowContext(ctx,
		`SELECT `+jsonTextExpr("reasoning_effort_multipliers")+` FROM channel_model_pricing WHERE id = `+ph(1), unsetID).Scan(&unset))
	require.JSONEq(t, `{}`, unset)

	// Changing or clearing the generic configuration must survive a migration replay.
	// （重放不变量由 PG 版迁移文件里的列存在性 DO 块承诺；MySQL 翻译版按
	// 迁移执行器单次执行保证去掉了守卫——重放整份文件会撞重复列，跳过。）
	if !IsMySQL() {
		for _, current := range []string{`{"high":1.5,"max":4}`, `{"low":0.75}`, `{}`} {
			_, err = tx.ExecContext(ctx, `UPDATE channel_model_pricing SET reasoning_effort_multipliers = $1 WHERE id = $2`, current, configuredID)
			require.NoError(t, err)
			_, err = tx.ExecContext(ctx, readMigrationSQLForDialect(t, "239_channel_reasoning_effort_multipliers.sql"))
			require.NoError(t, err)
			require.NoError(t, tx.QueryRowContext(ctx,
				`SELECT reasoning_effort_multipliers::text FROM channel_model_pricing WHERE id = $1`, configuredID).Scan(&configured))
			require.JSONEq(t, current, configured)
		}
	}

	// Account statistics use the same empty-by-default shape.
	ruleID, err := insertReturningID(ctx, tx, `INSERT INTO channel_account_stats_pricing_rules (channel_id) VALUES (`+ph(1)+`)`, channelID)
	require.NoError(t, err)
	var accountStatsDefault string
	if IsMySQL() {
		// MySQL 无 RETURNING 表达式：插入后按 LastInsertId 回查默认值。
		statsPricingID, insertErr := insertReturningID(ctx, tx,
			`INSERT INTO channel_account_stats_model_pricing (rule_id) VALUES (`+ph(1)+`)`, ruleID)
		require.NoError(t, insertErr)
		require.NoError(t, tx.QueryRowContext(ctx,
			`SELECT `+jsonTextExpr("reasoning_effort_multipliers")+` FROM channel_account_stats_model_pricing WHERE id = `+ph(1),
			statsPricingID).Scan(&accountStatsDefault))
	} else {
		require.NoError(t, tx.QueryRowContext(ctx,
			`INSERT INTO channel_account_stats_model_pricing (rule_id) VALUES ($1) RETURNING reasoning_effort_multipliers::text`,
			ruleID).Scan(&accountStatsDefault))
	}
	require.JSONEq(t, `{}`, accountStatsDefault)
}

func TestMigration239GroupReasoningEffortMultipliers(t *testing.T) {
	tx := testTx(t)
	ctx := context.Background()
	groupID, err := insertReturningID(ctx, tx, `
INSERT INTO `+groupsTableRef()+` (name, platform, model_pricing)
VALUES ('migration-group-reasoning', 'anthropic', '[
    {"models":["custom-model"],"input_price":2,"max_reasoning_effort_multiplier":2.5},
    {"models":["existing-map"],"max_reasoning_effort_multiplier":3,"reasoning_effort_multipliers":{"high":1.5,"max":4}},
    {"models":["cleared-map"],"max_reasoning_effort_multiplier":3,"reasoning_effort_multipliers":{}},
    {"models":["claude-fable-5-1"],"max_reasoning_effort_multiplier":null}
]')`)
	require.NoError(t, err)
	cleanupMySQLGroupRows(t, groupID)

	if IsMySQL() {
		// MySQL 翻译版迁移的 ADD COLUMN 无存在性守卫（依赖执行器单次执行保证），
		// 应用文件前把 239 引入的两列回到不存在状态，等价 PG 版 DO 块的起始条件。
		_, err = tx.ExecContext(ctx, `ALTER TABLE channel_model_pricing DROP COLUMN reasoning_effort_multipliers`)
		require.NoError(t, err)
		_, err = tx.ExecContext(ctx, `ALTER TABLE channel_account_stats_model_pricing DROP COLUMN reasoning_effort_multipliers`)
		require.NoError(t, err)
	}

	migrationSQL := readMigrationSQLForDialect(t, "239_channel_reasoning_effort_multipliers.sql")
	// PG 版整份文件可重放（DO 块守卫 + 存在性过滤）；MySQL 版仅应用一次。
	replays := 1
	if !IsMySQL() {
		replays = 2
	}
	for range replays {
		_, err = tx.ExecContext(ctx, migrationSQL)
		require.NoError(t, err)
		var actual string
		require.NoError(t, tx.QueryRowContext(ctx,
			`SELECT `+jsonTextExpr("model_pricing")+` FROM `+groupsTableRef()+` WHERE id = `+ph(1), groupID).Scan(&actual))
		require.JSONEq(t, `[
            {"models":["custom-model"],"input_price":2,"reasoning_effort_multipliers":{"max":2.5}},
            {"models":["existing-map"],"reasoning_effort_multipliers":{"high":1.5,"max":4}},
            {"models":["cleared-map"],"reasoning_effort_multipliers":{}},
            {"models":["claude-fable-5-1"]}
        ]`, actual)
	}
}

// cleanupMySQLChannelRows 在 MySQL 下显式删除本测试提交的 channel 夹具行
// （pricing / 规则子表经外键级联删除）；PG 侧沿用事务回滚，行为不变。
func cleanupMySQLChannelRows(t *testing.T, channelID int64) {
	t.Helper()
	if !IsMySQL() {
		return
	}

	t.Cleanup(func() {
		_, err := integrationDB.ExecContext(context.Background(), deleteByIDSQL("channels"), channelID)
		require.NoError(t, err)
	})
}
