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
REDACTED{
		{"cli", "codex_cli_rs/0.141.0 (Ubuntu 22.4.0; x86_64) xterm", "0.141.0", trueREDACTED,
		{"tui trailer", "codex-tui/0.140.2 (Mac OS X 14.0; arm64) iTerm (codex-tui; 0.140.2)", "0.140.2", trueREDACTED,
		{"cccc override prefix", "cccc/0.142.0 (Ubuntu 22.4.0; x86_64) screen (codex-tui; 0.142.0)", "0.142.0", trueREDACTED,
		{"desktop space prefix", "Codex Desktop/0.139.0 (Mac OS X 14; arm64) unknown", "0.139.0", trueREDACTED,
		{"alpha suffix keeps xyz", "codex_cli_rs/0.143.0-alpha.2 (Ubuntu; x86_64) x", "0.143.0", trueREDACTED,
		{"no slash", "curl 8.0", "", falseREDACTED,
		{"non numeric", "codex_cli_rs/abc (x)", "", falseREDACTED,
		{"empty", "", "", falseREDACTED,
REDACTED
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ver, ok := ParseCodexEngineVersion(tc.ua)
			require.Equal(t, tc.wantOK, ok)
			require.Equal(t, tc.wantVer, ver)
	REDACTED)
REDACTED
REDACTED
