//go:build unit

package service

import (
	"context"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/stretchr/testify/require"
)

func TestGatewayService_isModelSupportedByAccount_AntigravityModelMapping(t *testing.T) {
	svc := &GatewayService{}

	// 使用 model_mapping 作为白名单（通配符匹配）
	account := &Account{
		Platform: PlatformAntigravity,
		Credentials: map[string]any{
			"model_mapping": map[string]any{
				"claude-*":   "claude-sonnet-4-5",
				"gemini-3-*": "gemini-3-flash",
			},
		},
	}

	// claude-* 通配符匹配
	require.True(t, svc.isModelSupportedByAccount(account, "claude-sonnet-4-5"))
	require.True(t, svc.isModelSupportedByAccount(account, "claude-haiku-4-5"))
	require.True(t, svc.isModelSupportedByAccount(account, "claude-opus-4-6"))

	// gemini-3-* 通配符匹配
	require.True(t, svc.isModelSupportedByAccount(account, "gemini-3-flash"))
	require.True(t, svc.isModelSupportedByAccount(account, "gemini-3-pro-high"))

	// gemini-2.5-* 不匹配（不在 model_mapping 中）
	require.False(t, svc.isModelSupportedByAccount(account, "gemini-2.5-flash"))
	require.False(t, svc.isModelSupportedByAccount(account, "gemini-2.5-pro"))

	// 其他平台模型不支持
	require.False(t, svc.isModelSupportedByAccount(account, "gpt-4"))

	// 空模型允许
	require.True(t, svc.isModelSupportedByAccount(account, ""))
}

func TestGatewayService_isModelSupportedByAccount_AntigravityNoMapping(t *testing.T) {
	svc := &GatewayService{}

	// 未配置 model_mapping 时，使用默认映射（domain.DefaultAntigravityModelMapping）
	// 只有默认映射中的模型才被支持
	account := &Account{
		Platform:    PlatformAntigravity,
		Credentials: map[string]any{},
	}

	// 默认映射中的模型应该被支持
	require.True(t, svc.isModelSupportedByAccount(account, "claude-sonnet-4-5"))
	require.True(t, svc.isModelSupportedByAccount(account, "gemini-3-flash"))
	require.True(t, svc.isModelSupportedByAccount(account, "gemini-2.5-pro"))
	require.True(t, svc.isModelSupportedByAccount(account, "claude-haiku-4-5"))

	// 不在默认映射中的模型不被支持
	require.False(t, svc.isModelSupportedByAccount(account, "claude-3-5-sonnet-20241022"))
	require.False(t, svc.isModelSupportedByAccount(account, "claude-unknown-model"))

	// 非 claude-/gemini- 前缀仍然不支持
	require.False(t, svc.isModelSupportedByAccount(account, "gpt-4"))
}

// TestGatewayService_isModelSupportedByAccountWithContext_ThinkingMode 测试 thinking 模式下的模型支持检查
// 验证调度时使用映射后的最终模型名（包括 thinking 后缀）来检查 model_mapping 支持
func TestGatewayService_isModelSupportedByAccountWithContext_ThinkingMode(t *testing.T) {
	svc := &GatewayService{}

	tests := []struct {
		name            string
		modelMapping    map[string]any
		requestedModel  string
		thinkingEnabled bool
		expected        bool
	}{
		// 场景 1: 只配置 claude-sonnet-4-5-thinking，请求 claude-sonnet-4-5 + thinking=true
		// mapAntigravityModel 找不到 claude-sonnet-4-5 的映射 → 返回 false
		{
			name: "thinking_enabled_no_base_mapping_returns_false",
			modelMapping: map[string]any{
				"claude-sonnet-4-5-thinking": "claude-sonnet-4-5-thinking",
			},
			requestedModel:  "claude-sonnet-4-5",
			thinkingEnabled: true,
			expected:        false,
		},
		// 场景 2: 只配置 claude-sonnet-4-5-thinking，请求 claude-sonnet-4-5 + thinking=false
		// mapAntigravityModel 找不到 claude-sonnet-4-5 的映射 → 返回 false
		{
			name: "thinking_disabled_no_base_mapping_returns_false",
			modelMapping: map[string]any{
				"claude-sonnet-4-5-thinking": "claude-sonnet-4-5-thinking",
			},
			requestedModel:  "claude-sonnet-4-5",
			thinkingEnabled: false,
			expected:        false,
		},
		// 场景 3: 配置 claude-sonnet-4-5（非 thinking），请求 claude-sonnet-4-5 + thinking=true
		// 最终模型名 = claude-sonnet-4-5-thinking，不在 mapping 中，应该不匹配
		{
			name: "thinking_enabled_no_match_non_thinking_mapping",
			modelMapping: map[string]any{
				"claude-sonnet-4-5": "claude-sonnet-4-5",
			},
			requestedModel:  "claude-sonnet-4-5",
			thinkingEnabled: true,
			expected:        false,
		},
		// 场景 4: 配置两种模型，请求 claude-sonnet-4-5 + thinking=true，应该匹配 thinking 版本
		{
			name: "both_models_thinking_enabled_matches_thinking",
			modelMapping: map[string]any{
				"claude-sonnet-4-5":          "claude-sonnet-4-5",
				"claude-sonnet-4-5-thinking": "claude-sonnet-4-5-thinking",
			},
			requestedModel:  "claude-sonnet-4-5",
			thinkingEnabled: true,
			expected:        true,
		},
		// 场景 5: 配置两种模型，请求 claude-sonnet-4-5 + thinking=false，应该匹配非 thinking 版本
		{
			name: "both_models_thinking_disabled_matches_non_thinking",
			modelMapping: map[string]any{
				"claude-sonnet-4-5":          "claude-sonnet-4-5",
				"claude-sonnet-4-5-thinking": "claude-sonnet-4-5-thinking",
			},
			requestedModel:  "claude-sonnet-4-5",
			thinkingEnabled: false,
			expected:        true,
		},
		// 场景 6: 通配符 claude-* 应该同时匹配 thinking 和非 thinking
		{
			name: "wildcard_matches_thinking",
			modelMapping: map[string]any{
				"claude-*": "claude-sonnet-4-5",
			},
			requestedModel:  "claude-sonnet-4-5",
			thinkingEnabled: true,
			expected:        true, // claude-sonnet-4-5-thinking 匹配 claude-*
		},
		// 场景 7: 只配置 thinking 变体但没有基础模型映射 → 返回 false
		// mapAntigravityModel 找不到 claude-opus-4-6 的映射
		{
			name: "opus_thinking_no_base_mapping_returns_false",
			modelMapping: map[string]any{
				"claude-opus-4-6-thinking": "claude-opus-4-6-thinking",
			},
			requestedModel:  "claude-opus-4-6",
			thinkingEnabled: true,
			expected:        false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			account := &Account{
				Platform: PlatformAntigravity,
				Credentials: map[string]any{
					"model_mapping": tt.modelMapping,
				},
			}

			ctx := context.WithValue(context.Background(), ctxkey.ThinkingEnabled, tt.thinkingEnabled)
			result := svc.isModelSupportedByAccountWithContext(ctx, account, tt.requestedModel)

			require.Equal(t, tt.expected, result,
				"isModelSupportedByAccountWithContext(ctx[thinking=%v], account, %q) = %v, want %v",
				tt.thinkingEnabled, tt.requestedModel, result, tt.expected)
		})
	}
}

// TestGatewayService_isModelSupportedByAccount_CustomMappingNotInDefault 测试自定义模型映射中
// 不在 DefaultAntigravityModelMapping 中的模型能通过调度
func TestGatewayService_isModelSupportedByAccount_CustomMappingNotInDefault(t *testing.T) {
	svc := &GatewayService{}

	// 自定义映射中包含不在默认映射中的模型
	account := &Account{
		Platform: PlatformAntigravity,
		Credentials: map[string]any{
			"model_mapping": map[string]any{
				"my-custom-model":   "actual-upstream-model",
				"gpt-4o":            "some-upstream-model",
				"llama-3-70b":       "llama-3-70b-upstream",
				"claude-sonnet-4-5": "claude-sonnet-4-5",
			},
		},
	}

	// 自定义模型应该通过（不在 DefaultAntigravityModelMapping 中也可以）
	require.True(t, svc.isModelSupportedByAccount(account, "my-custom-model"))
	require.True(t, svc.isModelSupportedByAccount(account, "gpt-4o"))
	require.True(t, svc.isModelSupportedByAccount(account, "llama-3-70b"))
	require.True(t, svc.isModelSupportedByAccount(account, "claude-sonnet-4-5"))

	// 不在自定义映射中的模型不通过
	require.False(t, svc.isModelSupportedByAccount(account, "gpt-3.5-turbo"))
	require.False(t, svc.isModelSupportedByAccount(account, "unknown-model"))

	// 空模型允许
	require.True(t, svc.isModelSupportedByAccount(account, ""))
}

// TestGatewayService_isModelSupportedByAccountWithContext_CustomMappingThinking
// 测试自定义映射 + thinking 模式的交互
func TestGatewayService_isModelSupportedByAccountWithContext_CustomMappingThinking(t *testing.T) {
	svc := &GatewayService{}

	// 自定义映射同时配置基础模型和 thinking 变体
	account := &Account{
		Platform: PlatformAntigravity,
		Credentials: map[string]any{
			"model_mapping": map[string]any{
				"claude-sonnet-4-5":          "claude-sonnet-4-5",
				"claude-sonnet-4-5-thinking": "claude-sonnet-4-5-thinking",
				"my-custom-model":            "upstream-model",
			},
		},
	}

	// thinking=true: claude-sonnet-4-5 → mapped=claude-sonnet-4-5 → +thinking → check IsModelSupported(claude-sonnet-4-5-thinking)=true
	ctx := context.WithValue(context.Background(), ctxkey.ThinkingEnabled, true)
	require.True(t, svc.isModelSupportedByAccountWithContext(ctx, account, "claude-sonnet-4-5"))

	// thinking=false: claude-sonnet-4-5 → mapped=claude-sonnet-4-5 → check IsModelSupported(claude-sonnet-4-5)=true
	ctx = context.WithValue(context.Background(), ctxkey.ThinkingEnabled, false)
	require.True(t, svc.isModelSupportedByAccountWithContext(ctx, account, "claude-sonnet-4-5"))

	// 自定义模型（非 claude）不受 thinking 后缀影响，mapped 成功即通过
	ctx = context.WithValue(context.Background(), ctxkey.ThinkingEnabled, true)
	require.True(t, svc.isModelSupportedByAccountWithContext(ctx, account, "my-custom-model"))
}
