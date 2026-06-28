package openai

import "testing"

// 真实的 Claude Code codex 插件请求头：originator 与 UA 前缀同源于 clientInfo.name="Claude Code"。
const (
	testClaudeCodeOriginator = "Claude Code"
	testClaudeCodeUserAgent  = "Claude Code/0.5.0 (Macos 15.5; arm64) iTerm2.app (Claude Code; 1.0.4)"
)

func TestIsAllowedClientMatch(t *testing.T) {
	entry := AllowedClientEntry{Originator: "Claude Code", UAContains: []string{"Claude Code/"}}

	tests := []struct {
		name       string
		ua         string
		originator string
		want       bool
	}{
		{name: "真实签名命中", ua: testClaudeCodeUserAgent, originator: testClaudeCodeOriginator, want: true},
		{name: "大小写不敏感", ua: "claude code/0.5.0 (macos)", originator: "claude code", want: true},
		{name: "originator 两侧空白被裁剪", ua: testClaudeCodeUserAgent, originator: "  Claude Code  ", want: true},
		{name: "originator 非精确（带后缀）不命中", ua: testClaudeCodeUserAgent, originator: "Claude Code Extra", want: false},
		{name: "originator 为空不命中", ua: testClaudeCodeUserAgent, originator: "", want: false},
		{name: "originator 是官方 codex 不命中", ua: testClaudeCodeUserAgent, originator: "codex_cli_rs", want: false},
		{name: "UA 缺少 Claude Code/ 标记不命中", ua: "curl/8.0", originator: testClaudeCodeOriginator, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsAllowedClientMatch(tt.ua, tt.originator, entry); got != tt.want {
				t.Fatalf("IsAllowedClientMatch(%q, %q) = %v, want %v", tt.ua, tt.originator, got, tt.want)
			}
		})
	}
}

func TestIsAllowedClientMatch_EmptyOriginatorEntryNeverMatches(t *testing.T) {
	// registry 条目若没有配置 Originator，绝不放行，避免成为宽松后门。
	entry := AllowedClientEntry{Originator: "", UAContains: []string{"Claude Code/"}}
	if IsAllowedClientMatch(testClaudeCodeUserAgent, "", entry) {
		t.Fatal("空 Originator 的条目不应匹配任何请求")
	}
}

func TestIsAllowedClientMatch_EmptyUAContainsNeverMatches(t *testing.T) {
	// 预设必须声明 UA 特征，否则退化为仅凭可伪造的 originator 单因子匹配，绝不放行。
	entry := AllowedClientEntry{Originator: "Claude Code", UAContains: nil}
	if IsAllowedClientMatch(testClaudeCodeUserAgent, testClaudeCodeOriginator, entry) {
		t.Fatal("未声明 UA 特征的预设不应匹配，避免退化为单因子 originator 匹配")
	}
}

func TestIsAllowedClientMatch_WhitespaceUAMarkerNeverMatches(t *testing.T) {
	// 全空白 marker 归一化后为空，若被跳过则退化为仅 originator 单因子；
	// 任何空白 marker 视为非法预设配置，必须安全失败。
	entry := AllowedClientEntry{Originator: "Claude Code", UAContains: []string{"   "}}
	if IsAllowedClientMatch(testClaudeCodeUserAgent, testClaudeCodeOriginator, entry) {
		t.Fatal("UAContains 含全空白 marker 不应匹配，避免退化为单因子 originator 匹配")
	}
}

func TestIsAllowedClientMatch_MixedEmptyUAMarkerNeverMatches(t *testing.T) {
	// 即便 UAContains 含一个真实 marker，只要其中混入任何空白 marker 也视为非法配置；
	// 防止维护者只为对齐凑数而插入空字符串。
	entry := AllowedClientEntry{Originator: "Claude Code", UAContains: []string{"", "Claude Code/"}}
	if IsAllowedClientMatch(testClaudeCodeUserAgent, testClaudeCodeOriginator, entry) {
		t.Fatal("UAContains 混入空白 marker 不应匹配")
	}
}
