package openai

import "testing"

func TestIsCodexCLIRequest(t *testing.T) {
	tests := []struct {
		name string
		ua   string
		want bool
	}{
		{name: "codex_cli_rs 前缀", ua: "codex_cli_rs/0.1.0", want: true},
		{name: "codex_vscode 前缀", ua: "codex_vscode/1.2.3", want: true},
		{name: "大小写混合", ua: "Codex_CLI_Rs/0.1.0", want: true},
		{name: "复合 UA 包含 codex", ua: "Mozilla/5.0 codex_cli_rs/0.1.0", want: true},
		{name: "空白包裹", ua: "  codex_vscode/1.2.3  ", want: true},
		{name: "非 codex", ua: "curl/8.0.1", want: false},
		{name: "空字符串", ua: "", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsCodexCLIRequest(tt.ua)
			if got != tt.want {
				t.Fatalf("IsCodexCLIRequest(%q) = %v, want %v", tt.ua, got, tt.want)
			}
		})
	}
}

func TestCodexUATrailerName(t *testing.T) {
	tests := []struct {
		name string
		ua   string
		want string
	}{
		// 典型 cccc override 场景：前缀改写但尾部保留真实 clientInfo.name
		{name: "cccc override → codex-tui", ua: "cccc/0.141.0 (mac os 14.6.1; arm64) apple_terminal/453 (codex-tui; 0.141.0)", want: "codex-tui"},
		{name: "cccc override Ubuntu", ua: "cccc/0.139.0 (ubuntu 22.4.0; x86_64) screen (codex-tui; 0.139.0)", want: "codex-tui"},
		// 官方客户端自报名称
		{name: "Codex Desktop 自报(小写后)", ua: "codex desktop/0.142.0 (mac os 26.0.1; arm64) unknown (codex desktop; 26.616.71553)", want: "codex desktop"},
		{name: "codex-tui 自报", ua: "codex-tui/0.141.0 (mac os 15.5.0; arm64) ghostty/1.3.1 (codex-tui; 0.141.0)", want: "codex-tui"},
		{name: "codex_exec 自报", ua: "codex_exec/0.141.0 (mac os 14.7.3; arm64) apple_terminal (codex_exec; 0.141.0)", want: "codex_exec"},
		// 无括号/无尾部组
		{name: "curl 无括号", ua: "curl/8.0.1", want: ""},
		{name: "空字符串", ua: "", want: ""},
		// 非 codex 尾部
		{name: "非 codex 尾部不影响", ua: "evil/0.1.0 (linux; x86_64) bash (evil; 0.1.0)", want: "evil"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := codexUATrailerName(tt.ua)
			if got != tt.want {
				t.Fatalf("codexUATrailerName(%q) = %q, want %q", tt.ua, got, tt.want)
			}
		})
	}
}

func TestIsCodexOfficialClientRequest(t *testing.T) {
	tests := []struct {
		name string
		ua   string
		want bool
	}{
		{name: "codex_cli_rs 前缀", ua: "codex_cli_rs/0.98.0", want: true},
		{name: "codex_vscode 前缀", ua: "codex_vscode/1.0.0", want: true},
		{name: "codex_vscode_copilot 变体前缀", ua: "codex_vscode_copilot/0.140.0", want: true},
		{name: "codex_app 前缀", ua: "codex_app/0.1.0", want: true},
		{name: "codex_chatgpt_desktop 前缀", ua: "codex_chatgpt_desktop/1.0.0", want: true},
		{name: "codex_atlas 前缀", ua: "codex_atlas/1.0.0", want: true},
		{name: "codex_exec 前缀", ua: "codex_exec/0.1.0", want: true},
		{name: "codex_sdk_ts 前缀", ua: "codex_sdk_ts/0.1.0", want: true},
		{name: "Codex 桌面 UA", ua: "Codex Desktop/1.2.3", want: true},
		{name: "codex-tui 连字符前缀(真实流量占比最高)", ua: "codex-tui/0.141.0 (Mac OS 15.5.0; arm64) ghostty/1.3.1 (codex-tui; 0.141.0)", want: true},
		{name: "复合 UA 包含 codex_app", ua: "Mozilla/5.0 codex_app/0.1.0", want: true},
		{name: "大小写混合", ua: "Codex_VSCode/1.2.3", want: true},
		// UA 尾部兜底：cccc 是生产中 CODEX_INTERNAL_ORIGINATOR_OVERRIDE=cccc 的真实 codex-tui。
		// 审计 10GB/23天 中占非 codex 的 80.9%(494/611)、全 openai 流量的 5.3%——若不兜底会误杀。
		{name: "cccc override Mac → 尾部兜底放行", ua: "cccc/0.141.0 (Mac OS 14.6.1; arm64) Apple_Terminal/453 (codex-tui; 0.141.0)", want: true},
		{name: "cccc override Ubuntu → 尾部兜底放行", ua: "cccc/0.139.0 (Ubuntu 22.4.0; x86_64) screen (codex-tui; 0.139.0)", want: true},
		{name: "cccc override iTerm → 尾部兜底放行", ua: "cccc/0.137.0 (Mac OS 26.1.0; arm64) iTerm.app/3.4.22 (codex-tui; 0.137.0)", want: true},
		// 非 codex 尾部不应放行
		{name: "完全伪造尾部应拒", ua: "evil/0.1.0 (Linux; x86_64) bash (evil; 0.1.0)", want: false},
		{name: "非 codex", ua: "curl/8.0.1", want: false},
		{name: "空字符串", ua: "", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsCodexOfficialClientRequest(tt.ua)
			if got != tt.want {
				t.Fatalf("IsCodexOfficialClientRequest(%q) = %v, want %v", tt.ua, got, tt.want)
			}
		})
	}
}

func TestIsCodexOfficialClientOriginator(t *testing.T) {
	tests := []struct {
		name       string
		originator string
		want       bool
	}{
		{name: "codex_cli_rs", originator: "codex_cli_rs", want: true},
		{name: "codex_vscode", originator: "codex_vscode", want: true},
		{name: "codex_app", originator: "codex_app", want: true},
		{name: "codex_chatgpt_desktop", originator: "codex_chatgpt_desktop", want: true},
		{name: "codex_atlas", originator: "codex_atlas", want: true},
		{name: "codex_exec", originator: "codex_exec", want: true},
		{name: "codex_sdk_ts", originator: "codex_sdk_ts", want: true},
		{name: "Codex 前缀", originator: "Codex Desktop", want: true},
		{name: "codex-tui 连字符(真实流量占比最高)", originator: "codex-tui", want: true},
		{name: "空白包裹", originator: "  codex_vscode  ", want: true},
		{name: "伪造含 codex_ 子串应拒(L2 收紧)", originator: "evil-codex_cli", want: false},
		{name: "codex_ 混入中段应拒(L2 收紧)", originator: "my_codex_thing", want: false},
		{name: "非 codex", originator: "my_client", want: false},
		{name: "空字符串", originator: "", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsCodexOfficialClientOriginator(tt.originator)
			if got != tt.want {
				t.Fatalf("IsCodexOfficialClientOriginator(%q) = %v, want %v", tt.originator, got, tt.want)
			}
		})
	}
}

func TestIsCodexOfficialClientRequestStrict(t *testing.T) {
	tests := []struct {
		name string
		ua   string
		want bool
	}{
		// 前缀开头：与 lax 版一致放行
		{name: "codex_cli_rs 前缀开头", ua: "codex_cli_rs/0.141.0 (x)", want: true},
		{name: "codex_vscode 前缀开头", ua: "codex_vscode/1.0.0", want: true},
		{name: "codex_app 前缀开头", ua: "codex_app/2.1.0", want: true},
		{name: "Codex 家族前缀保留", ua: "Codex Desktop/1.2.3", want: true},
		{name: "大小写混合前缀开头", ua: "Codex_CLI_Rs/0.141.0", want: true},
		// UA 尾部兜底保留：cccc override 真实 codex-tui 仍放行
		{name: "cccc override 尾部兜底仍放行", ua: "cccc/0.141.0 (Mac OS 14.6.1; arm64) Apple_Terminal/453 (codex-tui; 0.141.0)", want: true},
		// N1 收紧：codex token 不在行首（子串）不再算官方——lax 版会因 Contains 误判 true
		{name: "浏览器前缀+中段 codex_app 收紧→拒", ua: "Mozilla/5.0 codex_app/0.141.0", want: false},
		{name: "中段 codex_cli_rs 收紧→拒", ua: "evilclient/1.0 codex_cli_rs/0.141.0", want: false},
		{name: "非 codex", ua: "curl/8.0.1", want: false},
		{name: "空字符串", ua: "", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsCodexOfficialClientRequestStrict(tt.ua)
			if got != tt.want {
				t.Fatalf("IsCodexOfficialClientRequestStrict(%q) = %v, want %v", tt.ua, got, tt.want)
			}
		})
	}
}

func TestIsCodexOfficialClientByHeaders(t *testing.T) {
	tests := []struct {
		name       string
		ua         string
		originator string
		want       bool
	}{
		{name: "仅 originator 命中 desktop", originator: "Codex Desktop", want: true},
		{name: "仅 originator 命中 vscode", originator: "codex_vscode", want: true},
		{name: "仅 ua 命中 desktop", ua: "Codex Desktop/1.2.3", want: true},
		{name: "仅 originator 命中 codex-tui", originator: "codex-tui", want: true},
		{name: "仅 ua 命中 codex-tui", ua: "codex-tui/0.141.0 (Mac OS 15.5.0; arm64) ghostty/1.3.1", want: true},
		// cccc：originator 不命中精确集，但 UA 尾部兜底恢复真实 codex-tui
		{name: "cccc override → UA 尾部兜底放行(审计 5.3% 误杀场景)", ua: "cccc/0.141.0 (Mac OS 14.6.1; arm64) Apple_Terminal/453 (codex-tui; 0.141.0)", originator: "cccc", want: true},
		{name: "ua 与 originator 都未命中", ua: "curl/8.0.1", originator: "my_client", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsCodexOfficialClientByHeaders(tt.ua, tt.originator)
			if got != tt.want {
				t.Fatalf("IsCodexOfficialClientByHeaders(%q, %q) = %v, want %v", tt.ua, tt.originator, got, tt.want)
			}
		})
	}
}
