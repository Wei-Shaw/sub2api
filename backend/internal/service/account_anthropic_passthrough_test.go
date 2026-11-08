package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAccount_IsAnthropicAPIKeyPassthroughEnabled(t *testing.T) {
	t.Run("Anthropic API Key 开启", func(t *testing.T) {
		account := &Account{
			Platform: PlatformAnthropic,
			Type:     AccountTypeAPIKey,
			Extra: map[string]any{
				"anthropic_passthrough": true,
			},
		}
		require.True(t, account.IsAnthropicAPIKeyPassthroughEnabled())
	})

	t.Run("Anthropic API Key 关闭", func(t *testing.T) {
		account := &Account{
			Platform: PlatformAnthropic,
			Type:     AccountTypeAPIKey,
			Extra: map[string]any{
				"anthropic_passthrough": false,
			},
		}
		require.False(t, account.IsAnthropicAPIKeyPassthroughEnabled())
	})

	t.Run("字段类型非法默认关闭", func(t *testing.T) {
		account := &Account{
			Platform: PlatformAnthropic,
			Type:     AccountTypeAPIKey,
			Extra: map[string]any{
				"anthropic_passthrough": "true",
			},
		}
		require.False(t, account.IsAnthropicAPIKeyPassthroughEnabled())
	})

	t.Run("非 Anthropic API Key 账号始终关闭", func(t *testing.T) {
		oauth := &Account{
			Platform: PlatformAnthropic,
			Type:     AccountTypeOAuth,
			Extra: map[string]any{
				"anthropic_passthrough": true,
			},
		}
		require.False(t, oauth.IsAnthropicAPIKeyPassthroughEnabled())

		openai := &Account{
			Platform: PlatformOpenAI,
			Type:     AccountTypeAPIKey,
			Extra: map[string]any{
				"anthropic_passthrough": true,
			},
		}
		require.False(t, openai.IsAnthropicAPIKeyPassthroughEnabled())
	})
}

func TestAccount_GetAnthropicAPIKeyAuthScheme(t *testing.T) {
	tests := []struct {
		name    string
		account *Account
		want    string
	}{
		{
			name: "missing extra defaults to x-api-key",
			account: &Account{
				Platform: PlatformAnthropic,
				Type:     AccountTypeAPIKey,
			},
			want: AnthropicAPIKeyAuthSchemeXAPIKey,
		},
		{
			name: "explicit bearer",
			account: &Account{
				Platform: PlatformAnthropic,
				Type:     AccountTypeAPIKey,
				Extra: map[string]any{
					"anthropic_apikey_auth_scheme": AnthropicAPIKeyAuthSchemeAuthorizationBearer,
				},
			},
			want: AnthropicAPIKeyAuthSchemeAuthorizationBearer,
		},
		{
			name: "invalid value defaults to x-api-key",
			account: &Account{
				Platform: PlatformAnthropic,
				Type:     AccountTypeAPIKey,
				Extra: map[string]any{
					"anthropic_apikey_auth_scheme": "bearer",
				},
			},
			want: AnthropicAPIKeyAuthSchemeXAPIKey,
		},
		{
			name: "non Anthropic API key defaults to x-api-key",
			account: &Account{
				Platform: PlatformOpenAI,
				Type:     AccountTypeAPIKey,
				Extra: map[string]any{
					"anthropic_apikey_auth_scheme": AnthropicAPIKeyAuthSchemeAuthorizationBearer,
				},
			},
			want: AnthropicAPIKeyAuthSchemeXAPIKey,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, tt.account.GetAnthropicAPIKeyAuthScheme())
		})
	}
}

// 直通模式（passthrough）下，OpenAI 平台账号也必须能拿到 Anthropic messages 的
// base_url —— 因为该模式要让 /v1/messages 原样出站，而 nativeAnthropicTargetURL
// 的唯一来源就是本函数。若这里返回空，messages 会被静默降级成「转 Chat Completions」
// （2026-10-10 线上实测发现的缺口）。
func TestAccount_GetAnthropicProtocolBaseURL_PassthroughOpenAIAccount(t *testing.T) {
	const baseURL = "https://api.a6api.com"

	t.Run("passthrough 的 openai 账号返回其 base_url", func(t *testing.T) {
		account := &Account{
			Platform:    PlatformOpenAI,
			Type:        AccountTypeAPIKey,
			Credentials: map[string]any{"base_url": baseURL},
			Extra: map[string]any{
				"openai_responses_mode": "passthrough",
			},
		}
		require.Equal(t, baseURL, account.GetAnthropicProtocolBaseURL())
	})

	t.Run("非 passthrough 的 openai 账号仍返回空", func(t *testing.T) {
		account := &Account{
			Platform:    PlatformOpenAI,
			Type:        AccountTypeAPIKey,
			Credentials: map[string]any{"base_url": baseURL},
			Extra:       map[string]any{},
		}
		require.Empty(t, account.GetAnthropicProtocolBaseURL())
	})

	t.Run("mode=auto 的 openai 账号仍返回空", func(t *testing.T) {
		account := &Account{
			Platform:    PlatformOpenAI,
			Type:        AccountTypeAPIKey,
			Credentials: map[string]any{"base_url": baseURL},
			Extra: map[string]any{
				"openai_responses_mode":      "auto",
				"openai_responses_supported": false,
			},
		}
		require.Empty(t, account.GetAnthropicProtocolBaseURL())
	})
}
