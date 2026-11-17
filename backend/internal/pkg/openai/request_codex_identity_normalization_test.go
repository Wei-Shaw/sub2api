package openai

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNormalizeCodexClientIdentityToCLI(t *testing.T) {
	tests := []struct {
		name           string
		originator     string
		userAgent      string
		wantOriginator string
		wantUserAgent  string
		wantChanged    bool
	}{
		{
			name:           "TUI 完整身份",
			originator:     CodexTUIOriginator,
			userAgent:      "codex-tui/0.163.0 (Windows 11; x86_64) WindowsTerminal (codex-tui; 0.163.0)",
			wantOriginator: CodexCLIOriginator,
			wantUserAgent:  "codex_cli_rs/0.163.0 (Windows 11; x86_64) WindowsTerminal",
			wantChanged:    true,
		},
		{
			name:           "TUI 只有系统指纹",
			originator:     CodexTUIOriginator,
			userAgent:      "codex-tui/0.163.0 (Windows 11; x86_64) WindowsTerminal",
			wantOriginator: CodexCLIOriginator,
			wantUserAgent:  "codex_cli_rs/0.163.0 (Windows 11; x86_64) WindowsTerminal",
			wantChanged:    true,
		},
		{
			name:           "CLI 身份保持不变",
			originator:     CodexCLIOriginator,
			userAgent:      "codex_cli_rs/0.163.0 (Windows 11; x86_64) WindowsTerminal",
			wantOriginator: CodexCLIOriginator,
			wantUserAgent:  "codex_cli_rs/0.163.0 (Windows 11; x86_64) WindowsTerminal",
			wantChanged:    false,
		},
		{
			name:           "其他官方客户端保持不变",
			originator:     "codex_vscode",
			userAgent:      "codex_vscode/0.163.0 (Windows 11; x86_64) vscode (codex_vscode; 0.163.0)",
			wantOriginator: "codex_vscode",
			wantUserAgent:  "codex_vscode/0.163.0 (Windows 11; x86_64) vscode (codex_vscode; 0.163.0)",
			wantChanged:    false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			originator, userAgent, changed := NormalizeCodexClientIdentityToCLI(test.originator, test.userAgent)
			require.Equal(t, test.wantOriginator, originator)
			require.Equal(t, test.wantUserAgent, userAgent)
			require.Equal(t, test.wantChanged, changed)
		})
	}
}

func TestNormalizeCodexClientIdentityToCLIIsPairedAndIdempotent(t *testing.T) {
	originator, userAgent, changed := NormalizeCodexClientIdentityToCLI(
		CodexTUIOriginator,
		"codex-tui/0.163.0 (Windows 11; x86_64) WindowsTerminal (codex-tui; 0.163.0)",
	)
	require.True(t, changed)

	pairedOriginator, pairedUserAgent, ok := PairCodexClientIdentity(userAgent)
	require.True(t, ok)
	require.Equal(t, originator, pairedOriginator)
	require.Equal(t, userAgent, pairedUserAgent)

	againOriginator, againUserAgent, changedAgain := NormalizeCodexClientIdentityToCLI(originator, userAgent)
	require.False(t, changedAgain)
	require.Equal(t, originator, againOriginator)
	require.Equal(t, userAgent, againUserAgent)
}
