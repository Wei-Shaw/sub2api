//go:build unit

package service

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

// advisorRepoStub 是只暴露 GetValue 的最小化 SettingRepository stub。
type advisorRepoStub struct {
	value string
	err   error
}

func (s *advisorRepoStub) Get(ctx context.Context, key string) (*Setting, error) {
	panic("unexpected Get call")
}
func (s *advisorRepoStub) GetValue(ctx context.Context, key string) (string, error) {
	if s.err != nil {
		return "", s.err
	}
	return s.value, nil
}
func (s *advisorRepoStub) Set(ctx context.Context, key, value string) error {
	panic("unexpected Set call")
}
func (s *advisorRepoStub) GetMultiple(ctx context.Context, keys []string) (map[string]string, error) {
	panic("unexpected GetMultiple call")
}
func (s *advisorRepoStub) SetMultiple(ctx context.Context, settings map[string]string) error {
	panic("unexpected SetMultiple call")
}
func (s *advisorRepoStub) GetAll(ctx context.Context) (map[string]string, error) {
	panic("unexpected GetAll call")
}
func (s *advisorRepoStub) Delete(ctx context.Context, key string) error {
	panic("unexpected Delete call")
}

func TestGetRectifierSettings_LegacyJSONUpgrade(t *testing.T) {
	t.Run("legacy json without advisor fields gets default pattern injected", func(t *testing.T) {
		legacyJSON := `{"enabled":true,"thinking_signature_enabled":true,"thinking_budget_enabled":true,"apikey_signature_enabled":false,"apikey_signature_patterns":[]}`
		svc := NewSettingService(&advisorRepoStub{value: legacyJSON}, nil)

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
		svc := NewSettingService(&advisorRepoStub{value: clearedJSON}, nil)

		got, err := svc.GetRectifierSettings(context.Background())
		require.NoError(t, err)
		require.NotNil(t, got.AdvisorToolPatterns, "cleared patterns must remain non-nil empty slice")
		require.Empty(t, got.AdvisorToolPatterns, "user-cleared patterns must NOT be overwritten by default")
	})

	t.Run("explicit user patterns are preserved", func(t *testing.T) {
		userJSON := `{"enabled":true,"advisor_tool_enabled":true,"advisor_tool_patterns":["foo","bar"]}`
		svc := NewSettingService(&advisorRepoStub{value: userJSON}, nil)

		got, err := svc.GetRectifierSettings(context.Background())
		require.NoError(t, err)
		require.Equal(t, []string{"foo", "bar"}, got.AdvisorToolPatterns)
	})

	t.Run("setting not found falls back to defaults", func(t *testing.T) {
		svc := NewSettingService(&advisorRepoStub{err: ErrSettingNotFound}, nil)

		got, err := svc.GetRectifierSettings(context.Background())
		require.NoError(t, err)
		// DefaultRectifierSettings 给出 enabled=true 与默认 advisor pattern。
		require.True(t, got.Enabled)
		require.True(t, got.AdvisorToolEnabled)
		require.Equal(t, []string{DefaultAdvisorToolPattern}, got.AdvisorToolPatterns)
	})

	t.Run("invalid json falls back to defaults", func(t *testing.T) {
		svc := NewSettingService(&advisorRepoStub{value: `{not valid`}, nil)

		got, err := svc.GetRectifierSettings(context.Background())
		require.NoError(t, err)
		require.True(t, got.AdvisorToolEnabled)
		require.Equal(t, []string{DefaultAdvisorToolPattern}, got.AdvisorToolPatterns)
	})
}

func TestIsAdvisorToolRectifierEnabled(t *testing.T) {
	cases := []struct {
		name string
		json string
		want bool
	}{
		{"both switches on", `{"enabled":true,"advisor_tool_enabled":true}`, true},
		{"master off blocks subswitch", `{"enabled":false,"advisor_tool_enabled":true}`, false},
		{"subswitch off blocks", `{"enabled":true,"advisor_tool_enabled":false}`, false},
		{"both off", `{"enabled":false,"advisor_tool_enabled":false}`, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc := NewSettingService(&advisorRepoStub{value: tc.json}, nil)
			got := svc.IsAdvisorToolRectifierEnabled(context.Background())
			require.Equal(t, tc.want, got)
		})
	}

	t.Run("repo error returns fail-open true", func(t *testing.T) {
		svc := NewSettingService(&advisorRepoStub{err: errors.New("db down")}, nil)
		// fail-open 与现有 IsBudgetRectifierEnabled / IsSignatureRectifierEnabled 行为一致：查询失败时默认启用。
		got := svc.IsAdvisorToolRectifierEnabled(context.Background())
		require.True(t, got)
	})
}

func TestShouldRectifyAdvisorToolError_GatewayService(t *testing.T) {
	body := []byte(`{"error":{"message":"Unexpected value(s) ` + "`advisor-tool-2026-03-01`" + ` for the ` + "`anthropic-beta`" + ` header."}}`)

	t.Run("nil setting service returns false (fail-closed)", func(t *testing.T) {
		gw := &GatewayService{}
		got := gw.shouldRectifyAdvisorToolError(context.Background(), body)
		require.False(t, got, "without a setting service we cannot read the switch — must be fail-closed")
	})

	t.Run("master switch off returns false", func(t *testing.T) {
		svc := NewSettingService(&advisorRepoStub{value: `{"enabled":false,"advisor_tool_enabled":true}`}, nil)
		gw := &GatewayService{settingService: svc}
		got := gw.shouldRectifyAdvisorToolError(context.Background(), body)
		require.False(t, got)
	})

	t.Run("subswitch off returns false", func(t *testing.T) {
		svc := NewSettingService(&advisorRepoStub{value: `{"enabled":true,"advisor_tool_enabled":false}`}, nil)
		gw := &GatewayService{settingService: svc}
		got := gw.shouldRectifyAdvisorToolError(context.Background(), body)
		require.False(t, got)
	})

	t.Run("both switches on returns true on built-in match", func(t *testing.T) {
		svc := NewSettingService(&advisorRepoStub{value: `{"enabled":true,"advisor_tool_enabled":true}`}, nil)
		gw := &GatewayService{settingService: svc}
		got := gw.shouldRectifyAdvisorToolError(context.Background(), body)
		require.True(t, got)
	})

	t.Run("custom pattern matches when builtin does not", func(t *testing.T) {
		svc := NewSettingService(&advisorRepoStub{
			value: `{"enabled":true,"advisor_tool_enabled":true,"advisor_tool_patterns":["my-private-flag"]}`,
		}, nil)
		gw := &GatewayService{settingService: svc}
		body := []byte(`{"error":{"message":"unsupported beta my-private-flag in vendor"}}`)
		got := gw.shouldRectifyAdvisorToolError(context.Background(), body)
		require.True(t, got)
	})
}
