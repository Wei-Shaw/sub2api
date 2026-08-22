package openai

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPairCodexClientIdentity(t *testing.T) {
	tests := []struct {
		name           string
		ua             string
		wantOriginator string
		wantUA         string
		wantOK         bool
	}{
		{
			name:           "cli 首段直接配对",
			ua:             "codex_cli_rs/0.144.1 (Ubuntu 22.4.0; x86_64) xterm-256color",
			wantOriginator: "codex_cli_rs",
			wantUA:         "codex_cli_rs/0.144.1 (Ubuntu 22.4.0; x86_64) xterm-256color",
			wantOK:         true,
		},
		{
			name:           "tui 首段直接配对",
			ua:             "codex-tui/0.140.2 (Mac OS X 14.0; arm64) iTerm (codex-tui; 0.140.2)",
			wantOriginator: "codex-tui",
			wantUA:         "codex-tui/0.140.2 (Mac OS X 14.0; arm64) iTerm (codex-tui; 0.140.2)",
			wantOK:         true,
		},
		{
			name:           "Codex 家族前缀配对保留原大小写",
			ua:             "Codex Desktop/1.2.3",
			wantOriginator: "Codex Desktop",
			wantUA:         "Codex Desktop/1.2.3",
			wantOK:         true,
		},
		{
			name:           "originator override 用尾部 name 重写首段",
			ua:             "cccc/0.142.0 (Ubuntu 22.4.0; x86_64) screen (codex-tui; 0.142.0)",
			wantOriginator: "codex-tui",
			wantUA:         "codex-tui/0.142.0 (Ubuntu 22.4.0; x86_64) screen (codex-tui; 0.142.0)",
			wantOK:         true,
		},
		{
			name:           "override 尾部恢复保留 Codex 家族真实大小写",
			ua:             "cccc/1.2.3 (Ubuntu 22.4.0; x86_64) term (Codex Desktop; 1.2.3)",
			wantOriginator: "Codex Desktop",
			wantUA:         "Codex Desktop/1.2.3 (Ubuntu 22.4.0; x86_64) term (Codex Desktop; 1.2.3)",
			wantOK:         true,
		},
		{name: "含斜杠的尾部 name 拒绝配对（防自不一致身份）", ua: "foo/1.0 (Codex Desktop/2; 1.0)", wantOK: false},
		{
			name:           "精确集合大小写变体归一为规范小写",
			ua:             "CODEX_CLI_RS/1.0.0",
			wantOriginator: "codex_cli_rs",
			wantUA:         "codex_cli_rs/1.0.0",
			wantOK:         true,
		},
		{
			name:           "首段尾随空格重建为规范 UA",
			ua:             "codex-tui /1.0.0",
			wantOriginator: "codex-tui",
			wantUA:         "codex-tui/1.0.0",
			wantOK:         true,
		},
		{name: "家族前缀夹带不可打印字节拒绝", ua: "Codex \x01evil/1.0.0", wantOK: false},
		{name: "家族前缀夹带非 ASCII 字节拒绝", ua: "Codex \xc3\xa9vil/1.0.0", wantOK: false},
		{name: "超长首段拒绝", ua: "Codex " + strings.Repeat("a", 80) + "/1.0.0", wantOK: false},
		{name: "第三方 UA 不可配对", ua: "luna/1.0.0", wantOK: false},
		{name: "伪造前缀不可配对", ua: "codex_cli_rs_evil/1.0.0", wantOK: false},
		{name: "浏览器 UA 不可配对", ua: "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0 Safari/537.36", wantOK: false},
		{name: "无斜杠不可配对", ua: "curl", wantOK: false},
		{name: "空 UA 不可配对", ua: "", wantOK: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			originator, pairedUA, ok := PairCodexClientIdentity(tt.ua)
			require.Equal(t, tt.wantOK, ok)
			require.Equal(t, tt.wantOriginator, originator)
			require.Equal(t, tt.wantUA, pairedUA)
		})
	}
}
