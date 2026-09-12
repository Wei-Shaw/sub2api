package service

import "testing"

func TestClassifyUsageUserAgent(t *testing.T) {
	for _, tc := range []struct{ ua, client, version string }{
		{"", "__missing__", ""},
		{" \t ", "__missing__", ""},
		{"Mozilla/5.0 (Windows NT 10.0) Chrome/130.0", "__browser__", ""},
		{"codex_cli_rs/0.147.0-alpha.4 (Windows; x86_64)", "Codex", "0.147.0-alpha.4"},
		{"codex-tui/0.146.0", "Codex", "0.146.0"},
		{"Codex Desktop/1.2.3 (Windows)", "Codex", "1.2.3"},
		{"claude-cli/2.1.22 (external, cli)", "Claude Code", "2.1.22"},
		{" CURL/8.0.1 ", "curl", "8.0.1"},
		{"custom-agent/2.0-beta (Linux)", "custom-agent", "2.0-beta"},
		{"not codex_cli_rs/1.0", "__unknown__", ""},
		{"plain text", "__unknown__", ""},
		{"client/", "__unknown__", ""},
		{"client/ (Linux)", "__unknown__", ""},
	} {
		t.Run(tc.ua, func(t *testing.T) {
			client, version := classifyUsageUserAgent(tc.ua)
			if client != tc.client || version != tc.version {
				t.Fatalf("got (%q, %q), want (%q, %q)", client, version, tc.client, tc.version)
			}
		})
	}
}
