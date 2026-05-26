//go:build unit

package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGetRectifierSettings_LegacyJSONUpgrade(t *testing.T) {
	t.Run("legacy json without advisor fields gets default pattern injected", func(t *testing.T) {
		legacyJSON := `{"enabled":true,"thinking_signature_enabled":true,"thinking_budget_enabled":true,"apikey_signature_enabled":false,"apikey_signature_patterns":[]}`
		svc := NewSettingService(&settingRepoStub{values: map[string]string{SettingKeyRectifierSettings: legacyJSON}}, nil)

		got, err := svc.GetRectifierSettings(context.Background())
		require.NoError(t, err)
		require.Equal(t, []string{DefaultAdvisorToolPattern}, got.AdvisorToolPatterns,
			"legacy JSON missing advisor_tool_patterns must inject default pattern")
		// 升级路径：advisor_tool_enabled 字段不存在 → 反序列化为 false（设计预期，不自动启用新功能）。
		require.False(t, got.AdvisorToolEnabled)
	})

	t.Run("user-cleared empty array is preserved (not overwritten by default)", func(t *testing.T) {
		// 用户故意清空 patterns 后保存的 JSON：advisor_tool_patterns: []
		clearedJSON := `{"enabled":true,"advisor_tool_enabled":true,"advisor_tool_patterns":[]}`
		svc := NewSettingService(&settingRepoStub{values: map[string]string{SettingKeyRectifierSettings: clearedJSON}}, nil)

		got, err := svc.GetRectifierSettings(context.Background())
		require.NoError(t, err)
		require.NotNil(t, got.AdvisorToolPatterns, "cleared patterns must remain non-nil empty slice")
		require.Empty(t, got.AdvisorToolPatterns, "user-cleared patterns must NOT be overwritten by default")
	})

	t.Run("explicit user patterns are preserved", func(t *testing.T) {
		userJSON := `{"enabled":true,"advisor_tool_enabled":true,"advisor_tool_patterns":["foo","bar"]}`
		svc := NewSettingService(&settingRepoStub{values: map[string]string{SettingKeyRectifierSettings: userJSON}}, nil)

		got, err := svc.GetRectifierSettings(context.Background())
		require.NoError(t, err)
		require.Equal(t, []string{"foo", "bar"}, got.AdvisorToolPatterns)
	})

	t.Run("setting not found falls back to defaults", func(t *testing.T) {
		svc := NewSettingService(&settingRepoStub{err: ErrSettingNotFound}, nil)

		got, err := svc.GetRectifierSettings(context.Background())
		require.NoError(t, err)
		// DefaultRectifierSettings 给出 enabled=true 与默认 advisor pattern。
		require.True(t, got.Enabled)
		require.True(t, got.AdvisorToolEnabled)
		require.Equal(t, []string{DefaultAdvisorToolPattern}, got.AdvisorToolPatterns)
	})

	t.Run("invalid json falls back to defaults", func(t *testing.T) {
		svc := NewSettingService(&settingRepoStub{values: map[string]string{SettingKeyRectifierSettings: `{not valid`}}, nil)

		got, err := svc.GetRectifierSettings(context.Background())
		require.NoError(t, err)
		require.True(t, got.AdvisorToolEnabled)
		require.Equal(t, []string{DefaultAdvisorToolPattern}, got.AdvisorToolPatterns)
	})
}

func TestShouldRectifyAdvisorToolError_GatewayService(t *testing.T) {
	body := []byte(`{"error":{"message":"Unexpected value(s) ` + "`advisor-tool-2026-03-01`" + ` for the ` + "`anthropic-beta`" + ` header."}}`)

	t.Run("nil setting service returns false (fail-closed)", func(t *testing.T) {
		gw := &GatewayService{}
		got := gw.shouldRectifyAdvisorToolError(context.Background(), PlatformAnthropic, body)
		require.False(t, got, "without a setting service we cannot read the switch — must be fail-closed")
	})

	t.Run("master switch off returns false", func(t *testing.T) {
		svc := NewSettingService(&settingRepoStub{values: map[string]string{SettingKeyRectifierSettings: `{"enabled":false,"advisor_tool_enabled":true}`}}, nil)
		gw := &GatewayService{settingService: svc}
		got := gw.shouldRectifyAdvisorToolError(context.Background(), PlatformAnthropic, body)
		require.False(t, got)
	})

	t.Run("subswitch off returns false", func(t *testing.T) {
		svc := NewSettingService(&settingRepoStub{values: map[string]string{SettingKeyRectifierSettings: `{"enabled":true,"advisor_tool_enabled":false}`}}, nil)
		gw := &GatewayService{settingService: svc}
		got := gw.shouldRectifyAdvisorToolError(context.Background(), PlatformAnthropic, body)
		require.False(t, got)
	})

	t.Run("both switches on returns true on built-in match", func(t *testing.T) {
		svc := NewSettingService(&settingRepoStub{values: map[string]string{SettingKeyRectifierSettings: `{"enabled":true,"advisor_tool_enabled":true}`}}, nil)
		gw := &GatewayService{settingService: svc}
		got := gw.shouldRectifyAdvisorToolError(context.Background(), PlatformAnthropic, body)
		require.True(t, got)
	})

	t.Run("custom pattern matches when builtin does not", func(t *testing.T) {
		svc := NewSettingService(&settingRepoStub{values: map[string]string{SettingKeyRectifierSettings: `{"enabled":true,"advisor_tool_enabled":true,"advisor_tool_patterns":["my-private-flag"]}`}}, nil)
		gw := &GatewayService{settingService: svc}
		body := []byte(`{"error":{"message":"unsupported beta my-private-flag in vendor"}}`)
		got := gw.shouldRectifyAdvisorToolError(context.Background(), PlatformAnthropic, body)
		require.True(t, got)
	})

	t.Run("non-anthropic platform returns false even when message would match", func(t *testing.T) {
		// advisor-tool-2026-03-01 是 Anthropic 特有 beta header，OpenAI / Antigravity / Gemini
		// 等其他平台不应进入 advisor 整流。即便上游响应文本碰巧含匹配关键词也必须返回 false——
		// 显式 platform guard 防止未来 OpenAI 类平台错误响应中混入"advisor-tool-2026-03-01"
		// 字面量时被误剥工具或 header。
		svc := NewSettingService(&settingRepoStub{values: map[string]string{SettingKeyRectifierSettings: `{"enabled":true,"advisor_tool_enabled":true}`}}, nil)
		gw := &GatewayService{settingService: svc}
		for _, platform := range []string{PlatformOpenAI, PlatformAntigravity, PlatformGemini, ""} {
			t.Run("platform="+platform, func(t *testing.T) {
				got := gw.shouldRectifyAdvisorToolError(context.Background(), platform, body)
				require.False(t, got, "non-anthropic platform must short-circuit advisor rectifier")
			})
		}
	})
}
