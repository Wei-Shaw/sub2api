package migrations

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestChannelMonitorGroupIDMigration 钉住「监控 → 模型广场分组」改成真外键这件事的三个要点。
//
// 背景（真实事故）：在加这一列之前，「监控 → 分组」只能靠三级降级猜
// （account_id → account_groups ＞ group_name 同名 ＞ 监控名同名）。
// 两级同名匹配遇到「删掉旧分组、又建了同名新分组」时会稳定认领到已软删的那条，
// 使模型广场 Pro 的卡片显示「0 个模型」。生产实例：codex-官方0.3折 有 id=5（已软删）
// 与 id=37（活跃），监控认领到了 5。
//
// 本测试守住三件事，缺任何一件都会让同类问题复发：
//
//	① 列本身是真外键 —— 绑定不存在的分组被数据库直接拒绝；
//	② 硬删分组自动解绑（ON DELETE SET NULL）—— 但**软删除不会触发**，
//	   所以读取侧必须自己过滤 deleted_at / status，这一点由回填里的过滤体现；
//	③ 回填带「目标分组未删除且 active」约束 —— 少了它，上面那条被认错的监控
//	   会被原样固化进新列，等于把 bug 写进数据里。
func TestChannelMonitorGroupIDMigration(t *testing.T) {
	content, err := FS.ReadFile("249_add_channel_monitor_group_id.sql")
	require.NoError(t, err)

	sql := strings.Join(strings.Fields(string(content)), " ")

	// ① 真外键 + 索引：与 226 迁移的 account_id 同构（普通列、FK 由 SQL 管理、
	// 删除时置空而不是级联删监控）。
	require.Contains(t, sql, "ADD COLUMN IF NOT EXISTS group_id BIGINT REFERENCES groups(id) ON DELETE SET NULL")
	require.Contains(t, sql, "CREATE INDEX IF NOT EXISTS idx_channel_monitors_group_id ON channel_monitors (group_id)")

	// ③ 回填：三级降级**每一级**都要求候选分组未删除且 active。
	// 这是把「删旧建新留下的同名僵尸分组」挡在外面的关键，
	// 也正是 codex-官方0.3折 能从已删的 5 被修正到活跃的 37 的原因。
	require.Equal(t, 3, strings.Count(sql, "g.deleted_at IS NULL"),
		"三级降级的每一级都必须过滤已删除分组，否则同名僵尸分组仍会被认领")
	require.Equal(t, 3, strings.Count(sql, "g.status = 'active'"),
		"三级降级的每一级都必须要求候选分组处于 active")

	// ③ 续：回填必须幂等（只填尚未绑定的行），重跑不会覆盖管理员的手工修改。
	require.Contains(t,
		sql,
		"WHERE m.id = resolved.monitor_id AND m.group_id IS NULL AND resolved.group_id IS NOT NULL",
		"回填必须只作用于 group_id 仍为空的行，保证可重复执行且不覆盖既有绑定")

	// ② 守卫：不允许把 ON DELETE SET NULL 换成 CASCADE ——
	// 那会在硬删分组时连带删掉监控本身及其历史。
	require.NotContains(t, sql, "ON DELETE CASCADE")
}
