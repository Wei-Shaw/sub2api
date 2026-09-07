//go:build unit

package service

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/stretchr/testify/require"
)

func TestPlatformKiroIsPromotedToDomain(t *testing.T) {
	require.Equal(t, "kiro", domain.PlatformKiro)
	require.Equal(t, domain.PlatformKiro, PlatformKiro,
		"service 侧常量必须由 domain 转出，不得再是本地字面量")
}

// TestKiroIsAllowedQuotaPlatform 是生产事故回归的一半。
// 另一半在 migrations/kiro_platform_migration_test.go —— 两者必须同时通过，
// 否则重现迁移 224 记载的「新用户零配额行 = 无限额」。
func TestKiroIsAllowedQuotaPlatform(t *testing.T) {
	require.True(t, IsAllowedQuotaPlatform(PlatformKiro))
	require.Contains(t, AllowedQuotaPlatforms, PlatformKiro)
}

// TestKiroStaysOutOfSchedulingThresholds 固化一个有意的排除：
// 阈值列表针对有原生 token 用量窗口的平台，Kiro 是 credits 制，
// 额度由 getUsageLimits + model_rate_limits["KiroCredits"] 管。
func TestKiroStaysOutOfSchedulingThresholds(t *testing.T) {
	require.NotContains(t, AllowedSchedulingThresholdPlatforms, PlatformKiro)
}
