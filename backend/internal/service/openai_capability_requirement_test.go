//go:build unit

package service

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/claude"
	"github.com/stretchr/testify/require"
)

func TestParseOpenAIAccountRequirement(t *testing.T) {
	forceFast := &Group{ID: 1, Platform: PlatformOpenAI, ForceOpenAIFast: true}
	tests := []struct {
		name          string
		body          string
		group         *Group
		wantRequested OpenAIAccountRequirement
		wantRouting   OpenAIAccountRequirement
	}{
		{name: "standard program is no requirement", body: `{"access_programs":{"cyber":"standard"}}`},
		{name: "daybreak blue", body: `{"access_programs":{"cyber":"daybreak_blue"}}`, wantRequested: OpenAIAccountRequirement{CyberProgram: "daybreak_blue"}, wantRouting: OpenAIAccountRequirement{CyberProgram: "daybreak_blue"}},
		{name: "daybreak red case insensitive", body: `{"access_programs":{"cyber":" Daybreak_Red "}}`, wantRequested: OpenAIAccountRequirement{CyberProgram: "daybreak_red"}, wantRouting: OpenAIAccountRequirement{CyberProgram: "daybreak_red"}},
		{name: "unknown program is left to upstream", body: `{"access_programs":{"cyber":"daybreak_green"}}`},
		{name: "fast alias", body: `{"service_tier":"fast"}`, wantRequested: OpenAIAccountRequirement{ServiceTier: "priority"}, wantRouting: OpenAIAccountRequirement{ServiceTier: "priority"}},
		{name: "priority", body: `{"service_tier":"priority"}`, wantRequested: OpenAIAccountRequirement{ServiceTier: "priority"}, wantRouting: OpenAIAccountRequirement{ServiceTier: "priority"}},
		{name: "ultrafast", body: `{"service_tier":"ultrafast"}`, wantRequested: OpenAIAccountRequirement{ServiceTier: "ultrafast"}, wantRouting: OpenAIAccountRequirement{ServiceTier: "ultrafast"}},
		{name: "flex", body: `{"service_tier":"flex"}`},
		{name: "auto", body: `{"service_tier":"auto"}`},
		{name: "default", body: `{"service_tier":"default"}`},
		{name: "scale", body: `{"service_tier":"scale"}`},
		{name: "absent", body: `{"model":"gpt-6-sol"}`},
		{name: "empty body", body: ``},
		{name: "invalid json", body: `{`},
		{
			name:          "both requirements",
			body:          `{"access_programs":{"cyber":"daybreak_blue"},"service_tier":"ultrafast"}`,
			wantRequested: OpenAIAccountRequirement{CyberProgram: "daybreak_blue", ServiceTier: "ultrafast"},
			wantRouting:   OpenAIAccountRequirement{CyberProgram: "daybreak_blue", ServiceTier: "ultrafast"},
		},
		{
			name:          "force fast folds routing tier to priority but keeps requested",
			body:          `{"access_programs":{"cyber":"daybreak_blue"},"service_tier":"ultrafast"}`,
			group:         forceFast,
			wantRequested: OpenAIAccountRequirement{CyberProgram: "daybreak_blue", ServiceTier: "ultrafast"},
			wantRouting:   OpenAIAccountRequirement{CyberProgram: "daybreak_blue", ServiceTier: "priority"},
		},
		{
			name:        "force fast with missing tier routes on priority",
			body:        `{"model":"gpt-6-sol"}`,
			group:       forceFast,
			wantRouting: OpenAIAccountRequirement{ServiceTier: "priority"},
		},
		{
			name:          "force fast ignored on non-openai platform",
			body:          `{"service_tier":"ultrafast"}`,
			group:         &Group{ID: 2, Platform: PlatformAnthropic, ForceOpenAIFast: true},
			wantRequested: OpenAIAccountRequirement{ServiceTier: "ultrafast"},
			wantRouting:   OpenAIAccountRequirement{ServiceTier: "ultrafast"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			requested, routing := ParseOpenAIAccountRequirement([]byte(tt.body), tt.group)
			require.Equal(t, tt.wantRequested, requested)
			require.Equal(t, tt.wantRouting, routing)
		})
	}
}

func TestParseOpenAIMessagesAccountRequirement(t *testing.T) {
	forceFast := &Group{ID: 1, Platform: PlatformOpenAI, ForceOpenAIFast: true}
	// 请求体 service_tier 在 Messages 路径不会到达上游：不构成需求。
	requested, routing := ParseOpenAIMessagesAccountRequirement([]byte(`{"service_tier":"ultrafast","access_programs":{"cyber":"daybreak_blue"}}`), "", nil)
	require.Equal(t, OpenAIAccountRequirement{CyberProgram: "daybreak_blue"}, requested)
	require.Equal(t, requested, routing)
	// anthropic-beta fast-mode 头注入 priority：构成 Fast 需求。
	requested, routing = ParseOpenAIMessagesAccountRequirement([]byte(`{"model":"gpt-6-sol"}`), "interleaved-thinking,"+claude.BetaFastMode, nil)
	require.Equal(t, OpenAIAccountRequirement{ServiceTier: "priority"}, requested)
	require.Equal(t, requested, routing)
	// 强制 Fast 分组：无头也按 priority 路由，原始需求保持为空。
	requested, routing = ParseOpenAIMessagesAccountRequirement([]byte(`{"model":"gpt-6-sol"}`), "", forceFast)
	require.Equal(t, OpenAIAccountRequirement{}, requested)
	require.Equal(t, OpenAIAccountRequirement{ServiceTier: "priority"}, routing)
}

func TestOpenAIAccountRequirement_SatisfiedBy(t *testing.T) {
	blueCaps, ok := DeriveCodexAccountCapabilities([]byte(codexCapabilitiesBlueAccountManifest))
	require.True(t, ok)
	standardCaps, ok := DeriveCodexAccountCapabilities([]byte(codexCapabilitiesStandardAccountManifest))
	require.True(t, ok)
	blueAccount := &Account{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Extra: map[string]any{CodexCapabilitiesExtraKey: blueCaps}}
	standardAccount := &Account{ID: 2, Platform: PlatformOpenAI, Type: AccountTypeSetupToken, Extra: map[string]any{CodexCapabilitiesExtraKey: standardCaps}}
	unprobedOAuth := &Account{ID: 3, Platform: PlatformOpenAI, Type: AccountTypeOAuth}
	apiKeyAccount := &Account{ID: 4, Platform: PlatformOpenAI, Type: AccountTypeAPIKey}
	grokAccount := &Account{ID: 5, Platform: PlatformGrok, Type: AccountTypeOAuth}

	blue := OpenAIAccountRequirement{CyberProgram: "daybreak_blue"}
	red := OpenAIAccountRequirement{CyberProgram: "daybreak_red"}
	ultrafast := OpenAIAccountRequirement{ServiceTier: "ultrafast"}
	fast := OpenAIAccountRequirement{ServiceTier: "priority"}
	both := OpenAIAccountRequirement{CyberProgram: "daybreak_blue", ServiceTier: "ultrafast"}

	tests := []struct {
		name       string
		req        OpenAIAccountRequirement
		account    *Account
		wantOK     bool
		wantReason string
	}{
		{name: "zero requirement passes unprobed", req: OpenAIAccountRequirement{}, account: unprobedOAuth, wantOK: true},
		{name: "blue account satisfies blue", req: blue, account: blueAccount, wantOK: true},
		{name: "blue account does not satisfy red", req: red, account: blueAccount, wantOK: false, wantReason: "daybreak_not_supported:daybreak_red"},
		{name: "standard account fails blue", req: blue, account: standardAccount, wantOK: false, wantReason: "daybreak_not_supported:daybreak_blue"},
		{name: "standard account fails ultrafast", req: ultrafast, account: standardAccount, wantOK: false, wantReason: "service_tier_not_supported:ultrafast"},
		{name: "standard setup_token satisfies fast", req: fast, account: standardAccount, wantOK: true},
		{name: "blue account satisfies both", req: both, account: blueAccount, wantOK: true},
		{name: "unprobed oauth fails fast", req: fast, account: unprobedOAuth, wantOK: false, wantReason: "service_tier_not_supported:priority"},
		{name: "unprobed oauth fails blue", req: blue, account: unprobedOAuth, wantOK: false, wantReason: "daybreak_not_supported:daybreak_blue"},
		{name: "api key always satisfies", req: both, account: apiKeyAccount, wantOK: true},
		{name: "grok always satisfies", req: both, account: grokAccount, wantOK: true},
		{name: "nil account satisfies", req: both, account: nil, wantOK: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ok, reason := tt.req.SatisfiedBy(tt.account)
			require.Equal(t, tt.wantOK, ok)
			require.Equal(t, tt.wantReason, reason)
		})
	}
	require.Equal(t, []string{"daybreak_blue", "ultrafast"}, both.Names())
	require.True(t, OpenAIAccountRequirement{}.IsZero())
	require.False(t, fast.IsZero())
}
