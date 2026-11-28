package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSupportsDeepseekTools(t *testing.T) {
	official := &Account{
		Platform:    PlatformDeepseek,
		Type:        AccountTypeAPIKey,
		Credentials: map[string]any{"base_url": "https://api.deepseek.com"},
	}
	thirdParty := &Account{
		Platform:    PlatformDeepseek,
		Type:        AccountTypeAPIKey,
		Credentials: map[string]any{"base_url": "https://www.crazyhub.cn/v1"},
	}
	withExtra := func(account *Account, extra map[string]any) *Account {
		clone := *account
		clone.Extra = extra
		return &clone
	}

	cases := []struct {
		name    string
		account *Account
		want    bool
	}{
		{"official base supported", official, true},
		{"empty base defaults official", &Account{Platform: PlatformDeepseek, Type: AccountTypeAPIKey, Credentials: map[string]any{}}, true},
		{"third party base unsupported", thirdParty, false},
		{"adaptive relay with empty legacy base", &Account{Platform: PlatformDeepseek, Type: AccountTypeAPIKey,
			Credentials: map[string]any{"api_protocol": "adaptive", "api_base_urls": map[string]any{"chat_completions": "https://relay.test"}}}, false},
		{"probe true overrides third party", withExtra(thirdParty, map[string]any{deepseekToolsSupportedExtraKey: true}), true},
		{"official host ignores stale probe", withExtra(official, map[string]any{deepseekToolsSupportedExtraKey: false}), true},
		{"force_supported overrides third party", withExtra(thirdParty, map[string]any{deepseekToolsModeExtraKey: "force_supported"}), true},
		{"force_unsupported overrides official", withExtra(official, map[string]any{deepseekToolsModeExtraKey: "force_unsupported"}), false},
		{"non deepseek platform", &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Credentials: map[string]any{"base_url": "https://api.deepseek.com"}}, false},
		{"nil account", nil, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, tc.account.SupportsOpenAIEndpointCapability(OpenAIEndpointCapabilityDeepseekTools))
		})
	}
}
