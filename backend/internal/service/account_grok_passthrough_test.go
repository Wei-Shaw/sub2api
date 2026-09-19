package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAccount_IsGrokPassthroughEnabled(t *testing.T) {
	t.Run("新字段开启", func(t *testing.T) {
		account := &Account{
			Platform: PlatformGrok,
			Type:     AccountTypeOAuth,
			Extra: map[string]any{
				"grok_passthrough": true,
			},
		}
		require.True(t, account.IsGrokPassthroughEnabled())
	})

	t.Run("非Grok账号始终关闭", func(t *testing.T) {
		account := &Account{
			Platform: PlatformOpenAI,
			Type:     AccountTypeOAuth,
			Extra: map[string]any{
				"grok_passthrough": true,
			},
		}
		require.False(t, account.IsGrokPassthroughEnabled())
	})

	t.Run("空额外配置默认关闭", func(t *testing.T) {
		account := &Account{
			Platform: PlatformGrok,
			Type:     AccountTypeAPIKey,
		}
		require.False(t, account.IsGrokPassthroughEnabled())
	})

	t.Run("非 bool 默认关闭", func(t *testing.T) {
		account := &Account{
			Platform: PlatformGrok,
			Type:     AccountTypeOAuth,
			Extra: map[string]any{
				"grok_passthrough": "true",
			},
		}
		require.False(t, account.IsGrokPassthroughEnabled())
	})

	t.Run("显式 false 关闭", func(t *testing.T) {
		account := &Account{
			Platform: PlatformGrok,
			Type:     AccountTypeAPIKey,
			Extra: map[string]any{
				"grok_passthrough": false,
			},
		}
		require.False(t, account.IsGrokPassthroughEnabled())
	})
}

func TestIsModelSupported_GrokPassthroughIgnoresLeftoverMapping(t *testing.T) {
	account := &Account{
		Platform: PlatformGrok,
		Type:     AccountTypeOAuth,
		Extra:    map[string]any{"grok_passthrough": true},
		Credentials: map[string]any{
			"model_mapping": map[string]any{"grok-4.5": "grok-4.5"},
		},
	}

	require.True(t, account.IsModelSupported("grok-4.6-new"), "透传应放行不在残留白名单中的新模型")
	require.True(t, account.IsModelSupported("grok-imagine-image"), "透传调度不因 mapping 排除模型")
}

func TestResolveOpenAIAccountUpstreamModelForRequest_GrokPassthroughKeepsRequestModel(t *testing.T) {
	account := &Account{
		Platform: PlatformGrok,
		Type:     AccountTypeOAuth,
		Extra:    map[string]any{"grok_passthrough": true},
		Credentials: map[string]any{
			"model_mapping": map[string]any{"grok-4.6": "grok-4.5"},
		},
	}

	require.Equal(t, "grok-4.6", resolveOpenAIAccountUpstreamModelForRequest(account, "grok-4.6", false))
	billing, upstream := resolveOpenAIForwardMappedModels(account, "grok-4.6", false)
	require.Equal(t, "grok-4.6", billing)
	require.Equal(t, "grok-4.6", upstream)
	require.Equal(t, "grok-4.6", canonicalOpenAIAccountSchedulingModel(account, "grok-4.6"))
}
