package openai

import "testing"

func TestIsCodexCLIRequest(t *testing.T) {
	tests := []struct {
		name string
		ua   string
		want bool
REDACTED{
		{name: "codex_cli_rs 前缀", ua: "codex_cli_rs/0.1.0", want: trueREDACTED,
		{name: "codex_vscode 前缀", ua: "codex_vscode/1.2.3", want: trueREDACTED,
		{name: "大小写混合", ua: "Codex_CLI_Rs/0.1.0", want: trueREDACTED,
		{name: "复合 UA 包含 codex", ua: "Mozilla/5.0 codex_cli_rs/0.1.0", want: trueREDACTED,
		{name: "空白包裹", ua: "  codex_vscode/1.2.3  ", want: trueREDACTED,
		{name: "非 codex", ua: "curl/8.0.1", want: falseREDACTED,
		{name: "空字符串", ua: "", want: falseREDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsCodexCLIRequest(tt.ua)
			if got != tt.want {
				t.Fatalf("IsCodexCLIRequest(%q) = %v, want %v", tt.ua, got, tt.want)
		REDACTED
	REDACTED)
REDACTED
REDACTED
