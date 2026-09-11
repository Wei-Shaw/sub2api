package openai

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCodexUserAgentVersion(t *testing.T) {
	require.Equal(t, "0.146.0", CodexUserAgentVersion("codex_cli_rs/0.146.0 (Ubuntu 22.4.0; x86_64) xterm-256color"))
	require.Equal(t, "0.146.0", CodexUserAgentVersion("  codex_cli_rs/0.146.0  "))
	// 预发布后缀原样保留：出站 version 头必须与 UA 版本段逐字一致。
	require.Equal(t, "0.147.0-alpha.4", CodexUserAgentVersion("codex-tui/0.147.0-alpha.4 (Mac OS X 14.0; arm64) iTerm"))
	// 非 `{client}/{version}` 形态取不到版本段。
	require.Empty(t, CodexUserAgentVersion("curl 8.7.1"))
	require.Empty(t, CodexUserAgentVersion("/0.146.0"))
	require.Empty(t, CodexUserAgentVersion(""))
}

// 管理员在面板 / 账号上配置的 UA 填写于某个历史版本，逐字沿用会把出站身份钉死在陈旧版本上。
// 重建只动版本声明，OS / 架构 / 终端指纹必须原样保留——那是该配置项唯一不可替代的价值。
func TestSetCodexUserAgentVersion(t *testing.T) {
	require.Equal(t,
		"codex_cli_rs/0.146.0 (Ubuntu 22.4.0; x86_64) xterm-256color",
		SetCodexUserAgentVersion("codex_cli_rs/0.125.0 (Ubuntu 22.4.0; x86_64) xterm-256color", "0.146.0"),
	)
	require.Equal(t, "codex_cli_rs/0.146.0", SetCodexUserAgentVersion("codex_cli_rs/0.1.0", "0.146.0"))

	// 单版本模板策略要求同步重建首尾，不意外留下旧版本；
	// 这不是通过 UA 推断 embedded/remote 或制品来源。
	require.Equal(t,
		"cccc/0.146.0 (Ubuntu 22.4.0; x86_64) screen (codex-tui; 0.146.0)",
		SetCodexUserAgentVersion("cccc/0.142.0 (Ubuntu 22.4.0; x86_64) screen (codex-tui; 0.142.0)", "0.146.0"),
	)
	// Desktop 尾部是独立 frontend 版本；整个版本元组必须由调用方整体保留或替换。
	require.Empty(t, SetCodexUserAgentVersion(
		"Codex Desktop/0.153.4 (Mac OS 15.6.0; arm64) unknown (Codex Desktop; 26.901.41600)",
		"0.154.0",
	))
	// OS 括号组不是客户端标识，不得被误改。
	require.Equal(t,
		"codex_cli_rs/0.146.0 (Ubuntu 22.4.0; x86_64)",
		SetCodexUserAgentVersion("codex_cli_rs/0.125.0 (Ubuntu 22.4.0; x86_64)", "0.146.0"),
	)

	// 无法重建时返回空串，由调用方决定整体回退，绝不拼出畸形身份。
	require.Empty(t, SetCodexUserAgentVersion("not-a-codex-client", "0.146.0"))
	require.Empty(t, SetCodexUserAgentVersion("codex_cli_rs/", "0.146.0"))
	require.Empty(t, SetCodexUserAgentVersion("/0.1.0", "0.146.0"))
	require.Empty(t, SetCodexUserAgentVersion("codex_cli_rs/0.1.0", ""))
}

func TestParseCodexWireProfile(t *testing.T) {
	t.Run("Desktop 双版本画像", func(t *testing.T) {
		ua := "Codex Desktop/0.153.4 (Mac OS 15.6.0; arm64) unknown (Codex Desktop; 26.901.41600)"

		profile, ok := ParseCodexWireProfile(ua)

		require.True(t, ok)
		require.Equal(t, "Codex Desktop", profile.Originator)
		require.Equal(t, "0.153.4", profile.CoreVersion)
		require.Equal(t, "(Mac OS 15.6.0; arm64) unknown", profile.RuntimeDescriptor)
		require.Equal(t, &CodexClientInfo{Name: "Codex Desktop", Version: "26.901.41600"}, profile.ClientInfo)
		require.True(t, profile.HasDistinctClientVersion())
		require.Equal(t, ua, profile.UserAgent())
	})

	t.Run("TUI 同版本画像", func(t *testing.T) {
		ua := "codex-tui/0.154.0 (Ubuntu 22.4.0; x86_64) xterm-256color (codex-tui; 0.154.0)"

		profile, ok := ParseCodexWireProfile(ua)

		require.True(t, ok)
		require.False(t, profile.HasDistinctClientVersion())
		require.Equal(t, ua, profile.UserAgent())
	})

	t.Run("remote TUI 可携带不同的 Core 和 frontend 版本", func(t *testing.T) {
		const ua = "codex-tui/0.150.0 (Mac OS 15.6.0; arm64) unknown (codex-tui; 0.149.0)"
		profile, ok := ParseCodexWireProfile(ua)
		require.True(t, ok)
		require.True(t, profile.HasDistinctClientVersion())
		require.Equal(t, "0.150.0", profile.CoreVersion)
		require.Equal(t, &CodexClientInfo{Name: "codex-tui", Version: "0.149.0"}, profile.ClientInfo)
		require.Equal(t, ua, profile.UserAgent())
		require.Empty(t, SetCodexUserAgentVersion(ua, "0.200.1"), "dual-version tuples must not be partially rewritten")
	})

	t.Run("originator 与 clientInfo 可独立", func(t *testing.T) {
		ua := "Codex Desktop/0.153.4 (Mac OS 15.6.0; arm64) unknown (codex_vscode; 9.8.7)"

		profile, ok := ParseCodexWireProfile(ua)

		require.True(t, ok)
		require.Equal(t, "Codex Desktop", profile.Originator)
		require.Equal(t, &CodexClientInfo{Name: "codex_vscode", Version: "9.8.7"}, profile.ClientInfo)
		require.Equal(t, ua, profile.UserAgent())
	})
}
