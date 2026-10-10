package dto

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

// ollama_cloud 平台账号无论合格与否都下发 ollama_cloud_usage state：不合格
// 时管理端要靠 EligibleReason 解释「为什么没有窗口/余额」。legacy 宿主平台
// （openai/kimi 等）无法与普通同平台账号区分，不合格时不挂 state。
func TestAccountFromServiceShallow_OllamaCloudUsageAttachment(t *testing.T) {
	t.Parallel()

	ollamaCloud := func() *service.Account {
		return &service.Account{
			ID:       1,
			Platform: service.PlatformOllamaCloud,
			Type:     service.AccountTypeAPIKey,
			Credentials: map[string]any{
				"base_url": "https://ollama.example.test", // 反代 → 不合格
				"api_key":  "key-1",
			},
		}
	}

	ineligible := AccountFromServiceShallow(ollamaCloud())
	require.NotNil(t, ineligible.OllamaCloudUsage)
	require.False(t, ineligible.OllamaCloudUsage.Eligible)
	require.Equal(t, "unsupported_base_url", ineligible.OllamaCloudUsage.EligibleReason)

	eligibleAccount := ollamaCloud()
	eligibleAccount.Credentials["base_url"] = "https://ollama.com"
	eligible := AccountFromServiceShallow(eligibleAccount)
	require.NotNil(t, eligible.OllamaCloudUsage)
	require.True(t, eligible.OllamaCloudUsage.Eligible)

	// legacy 宿主平台不合格账号：不挂 state（普通 openai 账号不受影响）。
	cnHost := &service.Account{
		ID:          2,
		Platform:    service.PlatformKimi,
		Type:        service.AccountTypeAPIKey,
		Credentials: map[string]any{"base_url": "https://api.kimi.com", "api_key": "key-2"},
	}
	require.Nil(t, AccountFromServiceShallow(cnHost).OllamaCloudUsage)

	// 完全无关平台（如 gemini）：照旧不挂 state。
	gemini := &service.Account{
		ID:          3,
		Platform:    service.PlatformGemini,
		Type:        service.AccountTypeOAuth,
		Credentials: map[string]any{},
	}
	require.Nil(t, AccountFromServiceShallow(gemini).OllamaCloudUsage)
}
