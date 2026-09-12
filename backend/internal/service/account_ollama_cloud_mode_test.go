//go:build unit

package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGetOllamaCloudAccountMode(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name        string
		platform    string
		accountType string
		credentials map[string]any
		want        string
	}{
		{
			name:        "defaults to legacy when unset",
			platform:    PlatformOllamaCloud,
			accountType: AccountTypeAPIKey,
			credentials: nil,
			want:        AccountModeOllamaLegacy,
		},
		{
			name:        "explicit legacy",
			platform:    PlatformOllamaCloud,
			accountType: AccountTypeAPIKey,
			credentials: map[string]any{"account_mode": AccountModeOllamaLegacy},
			want:        AccountModeOllamaLegacy,
		},
		{
			name:        "explicit credits",
			platform:    PlatformOllamaCloud,
			accountType: AccountTypeAPIKey,
			credentials: map[string]any{"account_mode": AccountModeOllamaCredits},
			want:        AccountModeOllamaCredits,
		},
		{
			name:        "unknown value falls back to legacy",
			platform:    PlatformOllamaCloud,
			accountType: AccountTypeAPIKey,
			credentials: map[string]any{"account_mode": "bogus"},
			want:        AccountModeOllamaLegacy,
		},
		{
			name:        "oauth account returns empty",
			platform:    PlatformOllamaCloud,
			accountType: AccountTypeOAuth,
			credentials: map[string]any{"account_mode": AccountModeOllamaCredits},
			want:        "",
		},
		{
			name:        "non ollama platform returns empty",
			platform:    PlatformOpenAI,
			accountType: AccountTypeAPIKey,
			credentials: map[string]any{"account_mode": AccountModeOllamaCredits},
			want:        "",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			account := &Account{
				Platform:    tc.platform,
				Type:        tc.accountType,
				Credentials: tc.credentials,
			}
			require.Equal(t, tc.want, account.GetOllamaCloudAccountMode())
		})
	}
}

func TestGetAccountMode_IgnoresOllamaCloudModes(t *testing.T) {
	t.Parallel()

	// 防回归：GetAccountMode 是国产 payg/coding 的白名单解析器，
	// 不得被 ollama_cloud 的新 mode 值污染。
	for _, mode := range []string{AccountModeOllamaLegacy, AccountModeOllamaCredits} {
		account := &Account{
			Platform:    PlatformOllamaCloud,
			Type:        AccountTypeAPIKey,
			Credentials: map[string]any{"account_mode": mode},
		}
		require.Empty(t, account.GetAccountMode())
	}
}
