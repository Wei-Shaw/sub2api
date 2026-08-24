package migrations

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestDropPlatformEnumCheckConstraintsMigration 校验 229 号迁移移除
// user_platform_quotas.platform 与 composite_model_routes.target_platform
// 的枚举 CHECK 约束——平台合法性收敛到应用层（CN 注册表派生的
// service.IsAllowedQuotaPlatform / isConcreteRequestPlatform），
// 新增供应商不再需要约束迁移（157/224 两次同型事故的根因）。
func TestDropPlatformEnumCheckConstraintsMigration(t *testing.T) {
	content, err := FS.ReadFile("229_drop_platform_enum_check_constraints.sql")
	require.NoError(t, err)

	sql := strings.Join(strings.Fields(string(content)), " ")
	require.Contains(t, sql, "DROP CONSTRAINT IF EXISTS user_platform_quotas_platform_check")
	require.Contains(t, sql, "DROP CONSTRAINT IF EXISTS composite_model_routes_target_platform_check")
	// 不得再引入新的平台枚举约束。
	require.NotContains(t, sql, "ADD CONSTRAINT")
}
