package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGetCodingPlanProvider_KimiOfficialURLs(t *testing.T) {
	for _, host := range []string{"api.kimi.com", "api.kimi.ai", "API.KIMI.AI", "api.kimi.ai:443"} {
		for _, path := range []string{"/coding", "/coding/", "/coding/v1", "/coding/v1/"} {
			t.Run(host+path, func(t *testing.T) {
				account := &Account{Platform: PlatformKimi, Type: AccountTypeAPIKey, Credentials: map[string]any{
					"account_mode": AccountModeCoding,
					"base_url":     "https://" + host + path,
				}}
				require.Equal(t, PlatformKimi, account.GetCodingPlanProvider())
				require.Equal(t, "https://"+host+"/coding/v1/usages", kimiQuotaURL(account.GetOpenAIBaseURL()))
				account.Credentials["account_mode"] = AccountModePayG
				require.Empty(t, account.GetCodingPlanProvider())
			})
		}
	}
}

func TestGetCodingPlanProvider_KimiRejectsUnsafeURLs(t *testing.T) {
	for _, baseURL := range []string{
		"https://api.kimi.ai.attacker.example/coding/v1",
		"https://api.kimi.com.attacker.example/coding/v1",
		"https://fake-api.kimi.ai/coding/v1",
		"https://relay.attacker.example/api.kimi.com/coding",
		"https://relay.attacker.example/?target=api.kimi.ai/coding/v1",
		"https://relay.attacker.example/#api.kimi.com/coding",
		"https://api.kimi.ai@relay.attacker.example/coding/v1",
		"https://user@api.kimi.ai/coding/v1",
		"https://api.kimi.ai/coding-other",
		"https://api.kimi.com/coding/v10",
		"https://api.kimi.ai/coding/v1/usages",
		"https://api.kimi.ai/v1?target=/coding/v1",
		"https://api.kimi.ai/coding/v1?target=attacker",
		"https://api.kimi.ai/coding/v1?",
		"https://api.kimi.ai/coding/v1#attacker",
		"https://api.kimi.ai/coding/v1#",
		"https://api.kimi.ai/%63oding/v1",
		"https://api.kimi.ai/coding%2fv1",
		"https://api.kimi.ai/coding/../coding/v1",
		"https://api.kimi.ai/CODING/v1",
		"https://api.kimi.ai:8443/coding/v1",
		"https://api.kimi.ai:/coding/v1",
		"http://api.kimi.ai/coding/v1",
		"//api.kimi.ai/coding/v1",
		"https://api.kimi.ai/%zz",
	} {
		t.Run(baseURL, func(t *testing.T) {
			account := &Account{Platform: PlatformKimi, Type: AccountTypeAPIKey, Credentials: map[string]any{
				"account_mode": AccountModeCoding,
				"base_url":     baseURL,
			}}
			require.Empty(t, account.GetCodingPlanProvider())
		})
	}
}

func TestGetCodingPlanProvider_UnrelatedProvidersUnchanged(t *testing.T) {
	for _, testCase := range []struct {
		platform string
		baseURL  string
	}{
		{PlatformZhipu, "https://open.bigmodel.cn/api/coding/paas/v4"},
		{PlatformZhipu, "https://api.z.ai/api/coding/paas/v4"},
		{PlatformMiniMax, "https://api.minimax.io/v1"},
		{PlatformMiniMax, "https://api.minimaxi.com/v1"},
		{PlatformMiniMax, "https://api.minimax.com/v1"},
	} {
		t.Run(testCase.baseURL, func(t *testing.T) {
			account := &Account{Platform: testCase.platform, Type: AccountTypeAPIKey, Credentials: map[string]any{
				"account_mode": AccountModeCoding,
				"base_url":     testCase.baseURL,
			}}
			require.Equal(t, testCase.platform, account.GetCodingPlanProvider())
		})
	}
}
