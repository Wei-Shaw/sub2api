package migrations

import (
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestKiroPlatformMigrationExtendsBothChecks 是生产事故回归的另一半。
// 迁移 224 的头注释记载：平台进了 AllowedQuotaPlatforms 但 CHECK 没扩，
// BulkInsertInitial 单条多行 INSERT 整条中止，注册路径 fail-open 吞错，
// 新用户拿到零条配额行 = 无限额。
func TestKiroPlatformMigrationExtendsBothChecks(t *testing.T) {
	raw, err := os.ReadFile("234_kiro_platform.sql")
	require.NoError(t, err)
	sql := string(raw)

	require.Contains(t, sql, "user_platform_quotas_platform_check")
	require.Contains(t, sql, "composite_model_routes_target_platform_check")

	// 两个 CHECK 都必须列入 kiro。
	require.GreaterOrEqual(t, strings.Count(sql, "'kiro'"), 2,
		"两个 CHECK 约束都必须包含 'kiro'")

	// 可重入：必须先 DROP ... IF EXISTS。
	require.Equal(t, 2, strings.Count(sql, "DROP CONSTRAINT IF EXISTS"),
		"两个约束都要可重入")

	// 新约束必须是旧约束的超集，存量行才能瞬时校验通过。
	for _, existing := range []string{
		"'anthropic'", "'openai'", "'gemini'", "'antigravity'", "'grok'",
		"'kimi'", "'zhipu'", "'deepseek'",
	} {
		require.GreaterOrEqual(t, strings.Count(sql, existing), 2,
			"存量平台 %s 必须保留在两个约束里", existing)
	}
}
