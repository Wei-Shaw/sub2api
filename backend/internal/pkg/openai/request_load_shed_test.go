package openai

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestIsCodexLoadShedOriginator(t *testing.T) {
	require.True(t, IsCodexLoadShedOriginator("codex-tui"))
	require.True(t, IsCodexLoadShedOriginator("  CODEX-TUI  "))
	require.False(t, IsCodexLoadShedOriginator("codex_cli_rs"))
	require.False(t, IsCodexLoadShedOriginator("codex_vscode"))
	require.False(t, IsCodexLoadShedOriginator("Codex Desktop"))
	require.False(t, IsCodexLoadShedOriginator(""))
}

func TestNormalizeCodexClientIdentityToCLI(t *testing.T) {
	tests := []struct {
		name           string
		originator     string
		ua             string
		wantOriginator string
		wantUA         string
		wantChanged    bool
	}{
		{
			name:           "tui 完整 UA 改写首段并裁掉客户端标识组",
			originator:     "codex-tui",
			ua:             "codex-tui/0.144.1 (Ubuntu 22.4.0; x86_64) xterm-256color (codex-tui; 0.144.1)",
			wantOriginator: "codex_cli_rs",
			wantUA:         "codex_cli_rs/0.144.1 (Ubuntu 22.4.0; x86_64) xterm-256color",
			wantChanged:    true,
		},
		{
			name:           "无客户端标识组时仅改写首段",
			originator:     "codex-tui",
			ua:             "codex-tui/0.144.1 (Mac OS X 14.0; arm64)",
			wantOriginator: "codex_cli_rs",
			wantUA:         "codex_cli_rs/0.144.1 (Mac OS X 14.0; arm64)",
			wantChanged:    true,
		},
		{
			name:           "OS 括号组不是客户端标识不得被裁剪",
			originator:     "codex-tui",
			ua:             "codex-tui/0.144.1 (Ubuntu 22.4.0; x86_64)",
			wantOriginator: "codex_cli_rs",
			wantUA:         "codex_cli_rs/0.144.1 (Ubuntu 22.4.0; x86_64)",
			wantChanged:    true,
		},
		{
			name:           "缺少版本段时只替换 originator",
			originator:     "codex-tui",
			ua:             "codex-tui",
			wantOriginator: "codex_cli_rs",
			wantUA:         "codex-tui",
			wantChanged:    true,
		},
		{
			name:           "健康身份原样返回",
			originator:     "codex_cli_rs",
			ua:             "codex_cli_rs/0.144.1 (Ubuntu 22.4.0; x86_64) xterm-256color",
			wantOriginator: "codex_cli_rs",
			wantUA:         "codex_cli_rs/0.144.1 (Ubuntu 22.4.0; x86_64) xterm-256color",
			wantChanged:    false,
		},
		{
			name:           "其他官方身份不受影响",
			originator:     "codex_vscode",
			ua:             "codex_vscode/1.0.0 (Ubuntu 22.4.0; x86_64) vscode (codex_vscode; 1.0.0)",
			wantOriginator: "codex_vscode",
			wantUA:         "codex_vscode/1.0.0 (Ubuntu 22.4.0; x86_64) vscode (codex_vscode; 1.0.0)",
			wantChanged:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotOriginator, gotUA, changed := NormalizeCodexClientIdentityToCLI(tt.originator, tt.ua)

			require.Equal(t, tt.wantOriginator, gotOriginator)
			require.Equal(t, tt.wantUA, gotUA)
			require.Equal(t, tt.wantChanged, changed)
		})
	}
}

// 归一化后的身份必须仍然通过上游的 originator ↔ UA 首段配对校验（issue #3901），
// 且再次归一化保持幂等。
func TestNormalizeCodexClientIdentityToCLIStaysPaired(t *testing.T) {
	originator, ua, changed := NormalizeCodexClientIdentityToCLI(
		"codex-tui",
		"codex-tui/0.144.1 (Ubuntu 22.4.0; x86_64) xterm-256color (codex-tui; 0.144.1)",
	)
	require.True(t, changed)

	pairedOriginator, pairedUA, ok := PairCodexClientIdentity(ua)
	require.True(t, ok)
	require.Equal(t, originator, pairedOriginator)
	require.Equal(t, ua, pairedUA)

	againOriginator, againUA, againChanged := NormalizeCodexClientIdentityToCLI(originator, ua)
	require.False(t, againChanged)
	require.Equal(t, originator, againOriginator)
	require.Equal(t, ua, againUA)
}
