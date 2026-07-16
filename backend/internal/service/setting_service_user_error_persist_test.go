//go:build unit

package service

import (
	"context"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

// TestAllowUserViewErrorRequests_PersistsToDB 验证 buildSystemSettingsUpdates 会将
// AllowUserViewErrorRequests 写入 updates map（即最终落库），这是对 bug 的回归测试：
// 该字段曾因漏写而永远无法持久化。
func TestAllowUserViewErrorRequests_PersistsToDB(t *testing.T) {
	// bmUpdateRepoStub 已在 setting_service_backend_mode_test.go 中定义（同 package）。
	// 本测试不触及需要 GetValue 的设置项，getValueFn 设为 nil 即可，无需 stub。
	repo := &bmUpdateRepoStub{}
	svc := NewSettingService(repo, &config.Config{})

	err := svc.UpdateSettings(context.Background(), &SystemSettings{
		AllowUserViewErrorRequests: true,
	})
	require.NoError(t, err)

	// 断言 updates 中含有该 key，且值为 "true"
	val, ok := repo.updates[SettingKeyAllowUserViewErrorRequests]
	require.True(t, ok, "updates map 中应包含 SettingKeyAllowUserViewErrorRequests，但未找到（bug：buildSystemSettingsUpdates 漏写）")
	require.Equal(t, "true", val)
}
