package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// 类型容错（string/number 等）由 resolveAccountExtraBool 自身覆盖，这里只测
// IsForceCodexIdentityEnabled 特有的分支：默认值、开启、账号类型门禁。
func TestAccount_IsForceCodexIdentityEnabled(t *testing.T) {
	cases := []struct {
		name    string
		account *Account
		want    bool
	}{
		{
			name:    "字段缺失默认关闭",
			account: &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth},
			want:    false,
		},
		{
			name:    "开启",
			account: &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth, Extra: map[string]any{"force_codex_identity": true}},
			want:    true,
		},
		{
			name:    "非OAuth账号-APIKey始终关闭",
			account: &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Extra: map[string]any{"force_codex_identity": true}},
			want:    false,
		},
		{
			name:    "非OAuth账号-setup-token始终关闭",
			account: &Account{Platform: PlatformOpenAI, Type: AccountTypeSetupToken, Extra: map[string]any{"force_codex_identity": true}},
			want:    false,
		},
		{
			name:    "非OAuth账号-其他平台始终关闭",
			account: &Account{Platform: PlatformAnthropic, Type: AccountTypeOAuth, Extra: map[string]any{"force_codex_identity": true}},
			want:    false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, tc.account.IsForceCodexIdentityEnabled())
		})
	}

	t.Run("与codex_cli_only互相独立", func(t *testing.T) {
		account := &Account{
			Platform: PlatformOpenAI,
			Type:     AccountTypeOAuth,
			Extra:    map[string]any{"codex_cli_only": true, "force_codex_identity": false},
		}
		require.True(t, account.IsCodexCLIOnlyEnabled())
		require.False(t, account.IsForceCodexIdentityEnabled())

		account2 := &Account{
			Platform: PlatformOpenAI,
			Type:     AccountTypeOAuth,
			Extra:    map[string]any{"codex_cli_only": false, "force_codex_identity": true},
		}
		require.False(t, account2.IsCodexCLIOnlyEnabled())
		require.True(t, account2.IsForceCodexIdentityEnabled())
	})
}
