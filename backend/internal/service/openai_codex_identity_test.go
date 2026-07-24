package service

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

func requireOpenAICodexProbeHeaders(t *testing.T, h http.Header) {
	t.Helper()
	require.Equal(t, codexCLIUserAgent, h.Get("User-Agent"))
	require.Equal(t, "codex_cli_rs", h.Get("Originator"))
	require.Equal(t, codexCLIVersion, h.Get("Version"))
	require.Equal(t, "responses=experimental", h.Get("OpenAI-Beta"))
	require.NotEmpty(t, h.Get("X-Codex-Window-ID"))
}

func TestEnsureCodexIdentityHeaders(t *testing.T) {
	t.Run("补齐缺失身份头", func(t *testing.T) {
		h := make(http.Header)

		ensureCodexIdentityHeaders(h)
		enforceCodexIdentityHeaders(h)

		require.Equal(t, "codex_cli_rs", h.Get("originator"))
		require.Equal(t, codexCLIUserAgent, h.Get("user-agent"))
		require.Equal(t, codexCLIVersion, h.Get("version"))
		require.Equal(t, "responses=experimental", h.Get("OpenAI-Beta"))
	})

	t.Run("保留已有官方UA和合法version并重新配对", func(t *testing.T) {
		const tuiUA = "codex-tui/9.9.9 (Mac OS X 14.0; arm64) iTerm (codex-tui; 9.9.9)"
		h := make(http.Header)
		h.Set("user-agent", tuiUA)
		h.Set("version", "9.9.9")
		h.Set("OpenAI-Beta", "assistants=v2")

		ensureCodexIdentityHeaders(h)
		enforceCodexIdentityHeaders(h)

		require.Equal(t, "codex-tui", h.Get("originator"))
		require.Equal(t, tuiUA, h.Get("user-agent"))
		require.Equal(t, "9.9.9", h.Get("version"))
		require.Equal(t, "responses=experimental", h.Get("OpenAI-Beta"))
	})
}

func TestEnforceCodexIdentityHeaders(t *testing.T) {
	const tuiUA = "codex-tui/0.140.2 (Mac OS X 14.0; arm64) iTerm (codex-tui; 0.140.2)"

	tests := []struct {
		name           string
		originator     string
		userAgent      string
		version        string
		wantOriginator string
		wantUA         string
		wantVersion    string
	}{
		{
			name:           "错配 originator 按最终 UA 重配",
			originator:     "codex_cli_rs",
			userAgent:      tuiUA,
			wantOriginator: "codex-tui",
			wantUA:         tuiUA,
		},
		{
			name:           "官方配套身份原样保留",
			originator:     "codex-tui",
			userAgent:      tuiUA,
			wantOriginator: "codex-tui",
			wantUA:         tuiUA,
		},
		{
			name:           "第三方 UA 整体回退默认身份",
			originator:     "opencode",
			userAgent:      "luna/1.0.0",
			wantOriginator: "codex_cli_rs",
			wantUA:         codexCLIUserAgent,
		},
		{
			name:           "UA 缺失回退默认身份",
			originator:     "codex_vscode",
			wantOriginator: "codex_cli_rs",
			wantUA:         codexCLIUserAgent,
		},
		{
			name:           "originator override UA 首段被尾部真实身份重写",
			originator:     "cccc",
			userAgent:      "cccc/0.142.0 (Ubuntu 22.4.0; x86_64) screen (codex-tui; 0.142.0)",
			wantOriginator: "codex-tui",
			wantUA:         "codex-tui/0.142.0 (Ubuntu 22.4.0; x86_64) screen (codex-tui; 0.142.0)",
		},
		{
			name:           "低于门槛的 version 提升为内置版本",
			originator:     "codex_cli_rs",
			userAgent:      "codex_cli_rs/0.125.0",
			version:        "0.125.0",
			wantOriginator: "codex_cli_rs",
			wantUA:         "codex_cli_rs/0.125.0",
			wantVersion:    codexCLIVersion,
		},
		{
			name:           "达标 version 原样保留",
			originator:     "codex_cli_rs",
			userAgent:      "codex_cli_rs/0.145.0",
			version:        "0.145.0",
			wantOriginator: "codex_cli_rs",
			wantUA:         "codex_cli_rs/0.145.0",
			wantVersion:    "0.145.0",
		},
		{
			name:           "未携带 version 不注入",
			originator:     "codex_cli_rs",
			userAgent:      "codex_cli_rs/0.98.0",
			wantOriginator: "codex_cli_rs",
			wantUA:         "codex_cli_rs/0.98.0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := make(http.Header)
			if tt.originator != "" {
				h.Set("originator", tt.originator)
			}
			if tt.userAgent != "" {
				h.Set("user-agent", tt.userAgent)
			}
			if tt.version != "" {
				h.Set("version", tt.version)
			}

			enforceCodexIdentityHeaders(h)

			require.Equal(t, tt.wantOriginator, h.Get("originator"))
			require.Equal(t, tt.wantUA, h.Get("user-agent"))
			require.Equal(t, tt.wantVersion, h.Get("version"))
		})
	}
}

// enforce 本身仍只负责收口：缺少 originator 时必须保持 no-op，由需要恢复身份的
// 调用方先显式调用 ensureCodexIdentityHeaders。
func TestEnforceCodexIdentityHeaders_NoOriginatorIsNoop(t *testing.T) {
	h := make(http.Header)
	h.Set("user-agent", "third-party-client/1.0.0")

	enforceCodexIdentityHeaders(h)

	require.Empty(t, h.Get("originator"))
	require.Equal(t, "third-party-client/1.0.0", h.Get("user-agent"))
}
