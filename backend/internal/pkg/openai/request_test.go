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

func TestIsCodexOfficialClientRequest(t *testing.T) {
	tests := []struct {
		name string
		ua   string
		want bool
REDACTED{
		{name: "codex_cli_rs 前缀", ua: "codex_cli_rs/0.98.0", want: trueREDACTED,
		{name: "codex_vscode 前缀", ua: "codex_vscode/1.0.0", want: trueREDACTED,
		{name: "codex_app 前缀", ua: "codex_app/0.1.0", want: trueREDACTED,
		{name: "codex_chatgpt_desktop 前缀", ua: "codex_chatgpt_desktop/1.0.0", want: trueREDACTED,
		{name: "codex_atlas 前缀", ua: "codex_atlas/1.0.0", want: trueREDACTED,
		{name: "codex_exec 前缀", ua: "codex_exec/0.1.0", want: trueREDACTED,
		{name: "codex_sdk_ts 前缀", ua: "codex_sdk_ts/0.1.0", want: trueREDACTED,
		{name: "Codex 桌面 UA", ua: "Codex Desktop/1.2.3", want: trueREDACTED,
		{name: "复合 UA 包含 codex_app", ua: "Mozilla/5.0 codex_app/0.1.0", want: trueREDACTED,
		{name: "大小写混合", ua: "Codex_VSCode/1.2.3", want: trueREDACTED,
		{name: "非 codex", ua: "curl/8.0.1", want: falseREDACTED,
		{name: "空字符串", ua: "", want: falseREDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsCodexOfficialClientRequest(tt.ua)
			if got != tt.want {
				t.Fatalf("IsCodexOfficialClientRequest(%q) = %v, want %v", tt.ua, got, tt.want)
		REDACTED
	REDACTED)
REDACTED
REDACTED

func TestIsCodexOfficialClientOriginator(t *testing.T) {
	tests := []struct {
		name       string
		originator string
		want       bool
REDACTED{
		{name: "codex_cli_rs", originator: "codex_cli_rs", want: trueREDACTED,
		{name: "codex_vscode", originator: "codex_vscode", want: trueREDACTED,
		{name: "codex_app", originator: "codex_app", want: trueREDACTED,
		{name: "codex_chatgpt_desktop", originator: "codex_chatgpt_desktop", want: trueREDACTED,
		{name: "codex_atlas", originator: "codex_atlas", want: trueREDACTED,
		{name: "codex_exec", originator: "codex_exec", want: trueREDACTED,
		{name: "codex_sdk_ts", originator: "codex_sdk_ts", want: trueREDACTED,
		{name: "Codex 前缀", originator: "Codex Desktop", want: trueREDACTED,
		{name: "空白包裹", originator: "  codex_vscode  ", want: trueREDACTED,
		{name: "非 codex", originator: "my_client", want: falseREDACTED,
		{name: "空字符串", originator: "", want: falseREDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsCodexOfficialClientOriginator(tt.originator)
			if got != tt.want {
				t.Fatalf("IsCodexOfficialClientOriginator(%q) = %v, want %v", tt.originator, got, tt.want)
		REDACTED
	REDACTED)
REDACTED
REDACTED
