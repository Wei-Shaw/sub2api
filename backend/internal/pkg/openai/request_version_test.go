package openai

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseCodexEngineVersion(t *testing.T) {
	cases := []struct {
		name    string
		ua      string
		wantVer string
		wantOK  bool
	}{
		{"cli", "codex_cli_rs/0.141.0 (Ubuntu 22.4.0; x86_64) xterm", "0.141.0", true},
		{"tui trailer", "codex-tui/0.140.2 (Mac OS X 14.0; arm64) iTerm (codex-tui; 0.140.2)", "0.140.2", true},
		{"cccc override prefix", "cccc/0.142.0 (Ubuntu 22.4.0; x86_64) screen (codex-tui; 0.142.0)", "0.142.0", true},
		{"desktop space prefix", "Codex Desktop/0.139.0 (Mac OS X 14; arm64) unknown", "0.139.0", true},
		{"alpha suffix keeps xyz", "codex_cli_rs/0.143.0-alpha.2 (Ubuntu; x86_64) x", "0.143.0", true},
		{"no slash", "curl 8.0", "", false},
		{"non numeric", "codex_cli_rs/abc (x)", "", false},
		{"empty", "", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ver, ok := ParseCodexEngineVersion(tc.ua)
			require.Equal(t, tc.wantOK, ok)
			require.Equal(t, tc.wantVer, ver)
		})
	}
}
