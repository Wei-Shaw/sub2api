package service

import (
	"net/http"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/openai"
	"github.com/stretchr/testify/require"
)

func requireOpenAICodexProbeHeaders(t *testing.T, h http.Header) {
	t.Helper()
	require.Equal(t, codexCLIUserAgent, h.Get("User-Agent"))
	require.Equal(t, openai.CodexDefaultOriginator, h.Get("Originator"))
	require.Equal(t, codexCLIVersion, h.Get("Version"))
	require.Equal(t, "responses=experimental", h.Get("OpenAI-Beta"))
	require.NotEmpty(t, h.Get("X-Codex-Window-ID"))
}

// 强制统一出口：无论客户端自报什么身份，OAuth 出站的 User-Agent / originator / version
// 一律是网关规范身份。上游在容量紧张时按客户端身份分优先级降载，统一出口确保没有请求
// 带着第三方或陈旧身份出站。
func TestEnsureCodexIdentityHeaders(t *testing.T) {
	t.Run("补齐缺失身份头", func(t *testing.T) {
		h := make(http.Header)

		ensureCodexIdentityHeaders(h)
		enforceCodexIdentityHeaders(h)

		require.Equal(t, openai.CodexDefaultOriginator, h.Get("originator"))
		require.Equal(t, codexCLIUserAgent, h.Get("user-agent"))
		require.Equal(t, codexCLIVersion, h.Get("version"))
		require.Equal(t, "responses=experimental", h.Get("OpenAI-Beta"))
	})

	t.Run("官方非 CLI 客户端身份同样被统一", func(t *testing.T) {
		h := make(http.Header)
		h.Set("user-agent", "codex_vscode/9.9.9 (Mac OS X 14.0; arm64) vscode (codex_vscode; 9.9.9)")
		h.Set("version", "9.9.9")
		h.Set("OpenAI-Beta", "assistants=v2")

		ensureCodexIdentityHeaders(h)
		enforceCodexIdentityHeaders(h)

		require.Equal(t, openai.CodexDefaultOriginator, h.Get("originator"))
		require.Equal(t, codexCLIUserAgent, h.Get("user-agent"))
		require.Equal(t, codexCLIVersion, h.Get("version"))
		require.Equal(t, "responses=experimental", h.Get("OpenAI-Beta"))
	})
}

func TestEnforceCodexIdentityHeaders(t *testing.T) {
	tests := []struct {
		name       string
		originator string
		userAgent  string
		version    string
	}{
		{
			name:       "TUI 身份",
			originator: "codex-tui",
			userAgent:  "codex-tui/0.140.2 (Mac OS X 14.0; arm64) iTerm (codex-tui; 0.140.2)",
		},
		{
			name:       "错配 originator",
			originator: "codex_cli_rs",
			userAgent:  "codex-tui/0.140.2 (Mac OS X 14.0; arm64) iTerm (codex-tui; 0.140.2)",
		},
		{
			name:       "官方 vscode 身份",
			originator: "codex_vscode",
			userAgent:  "codex_vscode/1.2.3 (Ubuntu 22.4.0; x86_64) vscode (codex_vscode; 1.2.3)",
		},
		{
			name:       "第三方客户端身份",
			originator: "opencode",
			userAgent:  "luna/1.0.0",
			version:    "2.1.0",
		},
		{
			name:       "浏览器型 UA（原浏览器兜底已被统一出口吸收）",
			originator: "codex_cli_rs",
			userAgent:  "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36",
		},
		{
			name:       "UA 缺失",
			originator: "codex_vscode",
		},
		{
			name:       "originator override 的真实 TUI 客户端",
			originator: "cccc",
			userAgent:  "cccc/0.142.0 (Ubuntu 22.4.0; x86_64) screen (codex-tui; 0.142.0)",
		},
		{
			name:       "陈旧客户端版本",
			originator: "codex_cli_rs",
			userAgent:  "codex_cli_rs/0.125.0",
			version:    "0.125.0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := make(http.Header)
			h.Set("originator", tt.originator)
			if tt.userAgent != "" {
				h.Set("user-agent", tt.userAgent)
			}
			if tt.version != "" {
				h.Set("version", tt.version)
			}

			enforceCodexIdentityHeaders(h)

			require.Equal(t, openai.CodexDefaultOriginator, h.Get("originator"))
			require.Equal(t, codexCLIUserAgent, h.Get("user-agent"))
			require.Equal(t, codexCLIVersion, h.Get("version"))
		})
	}
}

// 账号级自定义 UA 是管理员的显式配置，仍然生效；但它只贡献客户端名与 OS / 架构 / 终端指纹，
// originator 与版本段一律由规范身份重建，不允许出现自相矛盾或陈旧的身份。
func TestEnforceCodexIdentityHeadersWithAccountOverrideUA(t *testing.T) {
	t.Run("官方形态覆写 UA 保留指纹但重建版本段", func(t *testing.T) {
		h := make(http.Header)
		h.Set("originator", "codex-tui")
		h.Set("user-agent", "luna/1.0.0")
		h.Set("version", "2.1.0")

		enforceCodexIdentityHeadersWithUA(h, "codex_vscode/0.150.0 (Ubuntu 22.4.0; x86_64) vscode")

		require.Equal(t, "codex_vscode", h.Get("originator"))
		require.Equal(t, "codex_vscode/"+codexCLIVersion+" (Ubuntu 22.4.0; x86_64) vscode", h.Get("user-agent"))
		require.Equal(t, codexCLIVersion, h.Get("version"))
	})

	t.Run("非官方形态覆写 UA 回退规范身份", func(t *testing.T) {
		h := make(http.Header)
		h.Set("originator", "codex_cli_rs")

		enforceCodexIdentityHeadersWithUA(h, "luna/1.0.0")

		require.Equal(t, openai.CodexDefaultOriginator, h.Get("originator"))
		require.Equal(t, codexCLIUserAgent, h.Get("user-agent"))
		require.Equal(t, codexCLIVersion, h.Get("version"))
	})

	// 回归：覆写 UA 填写于某个历史版本时，其版本段必须被重建而不是逐字沿用——
	// 否则这条配置会绕过版本自动同步，把出站身份永久钉死在陈旧版本上，
	// 稳定落在上游优先降载的那一侧（UA 与 version 头也不再同源）。
	t.Run("陈旧覆写 UA 的版本段被重建", func(t *testing.T) {
		h := make(http.Header)
		h.Set("originator", "codex_cli_rs")

		enforceCodexIdentityHeadersWithUA(h, "codex_cli_rs/0.125.0 (Ubuntu 22.4.0; x86_64) xterm-256color")

		require.Equal(t, "codex_cli_rs", h.Get("originator"))
		require.Equal(t, "codex_cli_rs/"+codexCLIVersion+" (Ubuntu 22.4.0; x86_64) xterm-256color", h.Get("user-agent"))
		require.Equal(t, codexCLIVersion, h.Get("version"))
		require.NotContains(t, h.Get("user-agent"), "0.125.0")
	})

	// 陈旧覆写 UA 同样跟随自动同步到的新版本，无需管理员重新编辑那条 UA。
	t.Run("陈旧覆写 UA 跟随同步版本", func(t *testing.T) {
		SetCodexCanonicalUserAgentResolver(func() string {
			return "codex_cli_rs/0.200.1" + codexCLIUserAgentSuffix
		})
		t.Cleanup(func() { SetCodexCanonicalUserAgentResolver(nil) })

		h := make(http.Header)
		h.Set("originator", "codex_cli_rs")

		enforceCodexIdentityHeadersWithUA(h, "codex-tui/0.125.0 (Mac OS X 14.0; arm64) iTerm")

		require.Equal(t, "codex-tui", h.Get("originator"))
		require.Equal(t, "codex-tui/0.200.1 (Mac OS X 14.0; arm64) iTerm", h.Get("user-agent"))
		require.Equal(t, "0.200.1", h.Get("version"))
	})
}

// 规范身份跟随注入的解析器（后台面板 UA / 自动同步版本号），无需重启或发版。
//
// 不得给本用例加 t.Parallel()：它改写进程级解析器。
func TestEnforceCodexIdentityHeadersFollowsCanonicalResolver(t *testing.T) {
	SetCodexCanonicalUserAgentResolver(func() string {
		return "codex_cli_rs/0.200.1 (Ubuntu 22.4.0; x86_64) xterm-256color"
	})
	t.Cleanup(func() { SetCodexCanonicalUserAgentResolver(nil) })

	h := make(http.Header)
	h.Set("originator", "codex-tui")
	h.Set("user-agent", "codex-tui/0.140.2 (Mac OS X 14.0; arm64) iTerm (codex-tui; 0.140.2)")

	enforceCodexIdentityHeaders(h)

	require.Equal(t, "codex_cli_rs", h.Get("originator"))
	require.Equal(t, "codex_cli_rs/0.200.1 (Ubuntu 22.4.0; x86_64) xterm-256color", h.Get("user-agent"))
	require.Equal(t, "0.200.1", h.Get("version"))
}

// 解析器返回非法值（配置被写坏、同步到异常内容）时必须回退到内置身份，
// 绝不能把不可控内容拼进出站身份。
//
// 不得给本用例加 t.Parallel()：它改写进程级解析器。
func TestEnforceCodexIdentityHeadersRejectsInvalidCanonicalUA(t *testing.T) {
	SetCodexCanonicalUserAgentResolver(func() string { return "not-a-codex-client" })
	t.Cleanup(func() { SetCodexCanonicalUserAgentResolver(nil) })

	h := make(http.Header)
	h.Set("originator", "codex_cli_rs")

	enforceCodexIdentityHeaders(h)

	require.Equal(t, openai.CodexDefaultOriginator, h.Get("originator"))
	require.Equal(t, codexCLIUserAgent, h.Get("user-agent"))
	require.Equal(t, codexCLIVersion, h.Get("version"))
}

// 开关是进程级快照，零值 Config（测试 / 工具手工构造，不经 viper）必须落在「强制统一开启」
// 一侧，否则任意一处零值构造都会静默关掉全局保护。
//
// 不得给本文件的开关类用例加 t.Parallel()：它们改写进程级状态。
func TestCodexIdentityEnforcementZeroValueConfigKeepsItEnabled(t *testing.T) {
	var cfg config.Config
	require.False(t, cfg.Gateway.DisableCodexIdentityEnforcement,
		"零值必须表示强制统一开启；若改为正向命名，零值会静默关闭保护")

	SetCodexIdentityEnforcementEnabled(!cfg.Gateway.DisableCodexIdentityEnforcement)
	t.Cleanup(func() { SetCodexIdentityEnforcementEnabled(true) })

	h := make(http.Header)
	h.Set("originator", "codex-tui")
	h.Set("user-agent", "codex-tui/0.140.2 (Mac OS X 14.0; arm64) iTerm (codex-tui; 0.140.2)")

	enforceCodexIdentityHeaders(h)

	require.Equal(t, openai.CodexDefaultOriginator, h.Get("originator"))
	require.Equal(t, codexCLIUserAgent, h.Get("user-agent"))
}

// 关闭强制统一后退回配对语义：客户端真实身份逐字保留，仅保证 originator 与 UA 首段配套
// （issue #3901），供上游策略变动时回滚。
func TestEnforceCodexIdentityHeaders_EnforcementDisabled(t *testing.T) {
	const tuiUA = "codex-tui/0.145.2 (Mac OS X 14.0; arm64) iTerm (codex-tui; 0.145.2)"

	SetCodexIdentityEnforcementEnabled(false)
	t.Cleanup(func() { SetCodexIdentityEnforcementEnabled(true) })

	h := make(http.Header)
	h.Set("originator", "codex-tui")
	h.Set("user-agent", tuiUA)
	h.Set("version", "0.145.2")

	enforceCodexIdentityHeaders(h)

	require.Equal(t, "codex-tui", h.Get("originator"))
	require.Equal(t, tuiUA, h.Get("user-agent"))
	require.Equal(t, "0.145.2", h.Get("version"))
}

// 关闭强制统一后，第三方 UA 仍整体回退为规范身份并对齐 version。
func TestEnforceCodexIdentityHeaders_EnforcementDisabledThirdPartyFallback(t *testing.T) {
	SetCodexIdentityEnforcementEnabled(false)
	t.Cleanup(func() { SetCodexIdentityEnforcementEnabled(true) })

	h := make(http.Header)
	h.Set("originator", "opencode")
	h.Set("user-agent", "luna/1.0.0")
	h.Set("version", "2.1.0")

	enforceCodexIdentityHeaders(h)

	require.Equal(t, openai.CodexDefaultOriginator, h.Get("originator"))
	require.Equal(t, codexCLIUserAgent, h.Get("user-agent"))
	require.Equal(t, codexCLIVersion, h.Get("version"))
}

// 收口必须幂等：透传等路径可能先后多次经过收口。
func TestEnforceCodexIdentityHeadersIsIdempotent(t *testing.T) {
	h := make(http.Header)
	h.Set("originator", "codex-tui")
	h.Set("user-agent", "codex-tui/0.140.2 (Mac OS X 14.0; arm64) iTerm (codex-tui; 0.140.2)")

	enforceCodexIdentityHeaders(h)
	firstUA, firstVersion := h.Get("user-agent"), h.Get("version")
	enforceCodexIdentityHeaders(h)

	require.Equal(t, firstUA, h.Get("user-agent"))
	require.Equal(t, firstVersion, h.Get("version"))
	require.Equal(t, openai.CodexDefaultOriginator, h.Get("originator"))
}

// 缺少 originator 时必须保持 no-op：compat 桥接等非 ChatGPT 内部接口路径会显式删除
// originator，不应被补回身份头。
func TestEnforceCodexIdentityHeaders_NoOriginatorIsNoop(t *testing.T) {
	h := make(http.Header)
	h.Set("user-agent", "third-party-client/1.0.0")

	enforceCodexIdentityHeaders(h)

	require.Empty(t, h.Get("originator"))
	require.Empty(t, h.Get("version"))
	require.Equal(t, "third-party-client/1.0.0", h.Get("user-agent"))
}

func TestNormalizeCodexClientVersion(t *testing.T) {
	require.Equal(t, "0.146.0", NormalizeCodexClientVersion(" 0.146.0 "))
	require.Equal(t, "0.147.0-alpha.4", NormalizeCodexClientVersion("0.147.0-alpha.4"))
	require.Equal(t, "1.2", NormalizeCodexClientVersion("1.2"))
	require.Empty(t, NormalizeCodexClientVersion(""))
	require.Empty(t, NormalizeCodexClientVersion("v0.146.0"))
	require.Empty(t, NormalizeCodexClientVersion("0.146.0 (Ubuntu)"))
	require.Empty(t, NormalizeCodexClientVersion("0.146.0\r\nX-Injected: 1"))
	require.Empty(t, NormalizeCodexClientVersion("latest"))
}

func TestBuildCodexCLIUserAgent(t *testing.T) {
	require.Equal(t, openai.CodexDefaultOriginator+"/0.200.1"+codexCLIUserAgentSuffix, buildCodexCLIUserAgent("0.200.1"))
	// 非法版本号必须回退到内置 UA，不能拼出畸形身份。
	require.Equal(t, codexCLIUserAgent, buildCodexCLIUserAgent("bogus version"))
	require.Equal(t, codexCLIUserAgent, buildCodexCLIUserAgent(""))
}
