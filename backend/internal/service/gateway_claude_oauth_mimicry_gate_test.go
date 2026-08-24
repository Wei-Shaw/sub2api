package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// Kiro 账号（上游是 AWS CodeWhisperer）必须绕开 Claude Code 伪装链路。
// 伪装会把客户端真正的 instructions 从 system 摘走降级成 user 消息，
// 导致 Codex CLI 等客户端的工具调用契约失效（模型报"没有工具调用权限"）。
func TestShouldMimicClaudeCodeForAccount(t *testing.T) {
	tests := []struct {
		name            string
		account         *Account
		isClaudeCode    bool
		wantShouldMimic bool
	}{
		{
			name:            "Anthropic OAuth + 非 CC 客户端 → 伪装",
			account:         &Account{Platform: PlatformAnthropic, Type: AccountTypeOAuth},
			wantShouldMimic: true,
		},
		{
			name:            "Anthropic SetupToken + 非 CC 客户端 → 伪装",
			account:         &Account{Platform: PlatformAnthropic, Type: AccountTypeSetupToken},
			wantShouldMimic: true,
		},
		{
			name:            "Anthropic OAuth + CC 客户端 → 不伪装",
			account:         &Account{Platform: PlatformAnthropic, Type: AccountTypeOAuth},
			isClaudeCode:    true,
			wantShouldMimic: false,
		},
		{
			name:            "Anthropic APIKey → 不伪装",
			account:         &Account{Platform: PlatformAnthropic, Type: AccountTypeAPIKey},
			wantShouldMimic: false,
		},
		{
			name:            "Kiro OAuth → 不伪装（回归防护：Codex 工具调用）",
			account:         &Account{Platform: PlatformKiro, Type: AccountTypeOAuth},
			wantShouldMimic: false,
		},
		{
			name:            "Kiro SetupToken → 不伪装",
			account:         &Account{Platform: PlatformKiro, Type: AccountTypeSetupToken},
			wantShouldMimic: false,
		},
		{
			name:            "Kiro APIKey（带 base_url 中转）→ 不伪装",
			account:         &Account{Platform: PlatformKiro, Type: AccountTypeAPIKey},
			wantShouldMimic: false,
		},
		{
			name:            "nil 账号 → 不伪装",
			account:         nil,
			wantShouldMimic: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.wantShouldMimic,
				shouldMimicClaudeCodeForAccount(tt.account, tt.isClaudeCode))
		})
	}
}
