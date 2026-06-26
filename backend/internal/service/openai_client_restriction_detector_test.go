package service

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/openai"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func newCodexDetectorTestContext(ua string, originator string) *gin.Context {
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	if ua != "" {
		c.Request.Header.Set("User-Agent", ua)
	}
	if originator != "" {
		c.Request.Header.Set("originator", originator)
	}
	return c
}

func TestOpenAICodexClientRestrictionDetector_Detect(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("未开启开关时绕过", func(t *testing.T) {
		detector := NewOpenAICodexClientRestrictionDetector(nil)
		account := &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth, Extra: map[string]any{}}

		result := detector.Detect(newCodexDetectorTestContext("curl/8.0", ""), account, CodexRestrictionPolicy{}, nil)
		require.False(t, result.Enabled)
		require.False(t, result.Matched)
		require.Equal(t, CodexClientRestrictionReasonDisabled, result.Reason)
	})

	t.Run("开启后 codex_cli_rs 命中", func(t *testing.T) {
		detector := NewOpenAICodexClientRestrictionDetector(nil)
		account := &Account{
			Platform: PlatformOpenAI,
			Type:     AccountTypeOAuth,
			Extra:    map[string]any{"codex_cli_only": true},
		}

		result := detector.Detect(newCodexDetectorTestContext("codex_cli_rs/0.99.0", ""), account, CodexRestrictionPolicy{}, nil)
		require.True(t, result.Enabled)
		require.True(t, result.Matched)
		require.Equal(t, CodexClientRestrictionReasonMatchedUA, result.Reason)
	})

	t.Run("开启后 codex_vscode 命中", func(t *testing.T) {
		detector := NewOpenAICodexClientRestrictionDetector(nil)
		account := &Account{
			Platform: PlatformOpenAI,
			Type:     AccountTypeOAuth,
			Extra:    map[string]any{"codex_cli_only": true},
		}

		result := detector.Detect(newCodexDetectorTestContext("codex_vscode/1.0.0", ""), account, CodexRestrictionPolicy{}, nil)
		require.True(t, result.Enabled)
		require.True(t, result.Matched)
		require.Equal(t, CodexClientRestrictionReasonMatchedUA, result.Reason)
	})

	t.Run("开启后 codex_app 命中", func(t *testing.T) {
		detector := NewOpenAICodexClientRestrictionDetector(nil)
		account := &Account{
			Platform: PlatformOpenAI,
			Type:     AccountTypeOAuth,
			Extra:    map[string]any{"codex_cli_only": true},
		}

		result := detector.Detect(newCodexDetectorTestContext("codex_app/2.1.0", ""), account, CodexRestrictionPolicy{}, nil)
		require.True(t, result.Enabled)
		require.True(t, result.Matched)
		require.Equal(t, CodexClientRestrictionReasonMatchedUA, result.Reason)
	})

	t.Run("开启后 originator 命中", func(t *testing.T) {
		detector := NewOpenAICodexClientRestrictionDetector(nil)
		account := &Account{
			Platform: PlatformOpenAI,
			Type:     AccountTypeOAuth,
			Extra:    map[string]any{"codex_cli_only": true},
		}

		result := detector.Detect(newCodexDetectorTestContext("myterm/0.141.0", "codex_chatgpt_desktop"), account, CodexRestrictionPolicy{}, nil)
		require.True(t, result.Enabled)
		require.True(t, result.Matched)
		require.Equal(t, CodexClientRestrictionReasonMatchedOriginator, result.Reason)
	})

	t.Run("开启后非官方客户端拒绝", func(t *testing.T) {
		detector := NewOpenAICodexClientRestrictionDetector(nil)
		account := &Account{
			Platform: PlatformOpenAI,
			Type:     AccountTypeOAuth,
			Extra:    map[string]any{"codex_cli_only": true},
		}

		result := detector.Detect(newCodexDetectorTestContext("curl/8.0", "my_client"), account, CodexRestrictionPolicy{}, nil)
		require.True(t, result.Enabled)
		require.False(t, result.Matched)
		require.Equal(t, CodexClientRestrictionReasonNotMatchedUA, result.Reason)
	})

	t.Run("开启 ForceCodexCLI 时允许通过", func(t *testing.T) {
		detector := NewOpenAICodexClientRestrictionDetector(&config.Config{
			Gateway: config.GatewayConfig{ForceCodexCLI: true},
		})
		account := &Account{
			Platform: PlatformOpenAI,
			Type:     AccountTypeOAuth,
			Extra:    map[string]any{"codex_cli_only": true},
		}

		result := detector.Detect(newCodexDetectorTestContext("curl/8.0", "my_client"), account, CodexRestrictionPolicy{}, nil)
		require.True(t, result.Enabled)
		require.True(t, result.Matched)
		require.Equal(t, CodexClientRestrictionReasonForceCodexCLI, result.Reason)
	})
}

func TestOpenAICodexClientRestrictionDetector_Detect_AllowedClients(t *testing.T) {
	gin.SetMode(gin.TestMode)

	const (
		claudeCodeUA         = "Claude Code/0.5.0 (Macos 15.5; arm64) iTerm2.app (Claude Code; 1.0.4)"
		claudeCodeOriginator = "Claude Code"
	)

	t.Run("未配置白名单时 Claude Code 签名仍拒绝", func(t *testing.T) {
		detector := NewOpenAICodexClientRestrictionDetector(nil)
		account := &Account{
			Platform: PlatformOpenAI,
			Type:     AccountTypeOAuth,
			Extra:    map[string]any{"codex_cli_only": true},
		}

		result := detector.Detect(newCodexDetectorTestContext(claudeCodeUA, claudeCodeOriginator), account, CodexRestrictionPolicy{}, nil)
		require.True(t, result.Enabled)
		require.False(t, result.Matched)
		require.Equal(t, CodexClientRestrictionReasonNotMatchedUA, result.Reason)
	})

	t.Run("未开启 codex_cli_only 时直接绕过", func(t *testing.T) {
		detector := NewOpenAICodexClientRestrictionDetector(nil)
		account := &Account{
			Platform: PlatformOpenAI,
			Type:     AccountTypeOAuth,
			Extra:    map[string]any{},
		}

		result := detector.Detect(newCodexDetectorTestContext(claudeCodeUA, claudeCodeOriginator), account, CodexRestrictionPolicy{}, nil)
		require.False(t, result.Enabled)
		require.False(t, result.Matched)
		require.Equal(t, CodexClientRestrictionReasonDisabled, result.Reason)
	})

	t.Run("全局白名单含 Claude Code 签名 → 放行(whitelist)", func(t *testing.T) {
		detector := NewOpenAICodexClientRestrictionDetector(nil)
		account := &Account{
			Platform: PlatformOpenAI,
			Type:     AccountTypeOAuth,
			Extra:    map[string]any{"codex_cli_only": true},
		}
		result := detector.Detect(
			newCodexDetectorTestContext("Claude Code/0.5.0 (Macos 15.5; arm64) iTerm2.app (Claude Code; 1.0.4)", "Claude Code"),
			account,
			CodexRestrictionPolicy{Whitelist: []openai.AllowedClientEntry{{Originator: "Claude Code", UAContains: []string{"Claude Code/"}}}},
			nil,
		)
		require.True(t, result.Enabled)
		require.True(t, result.Matched)
		require.Equal(t, CodexClientRestrictionReasonMatchedWhitelistClient, result.Reason)
	})

	t.Run("全局白名单含 Claude Code + 非签名 → 403", func(t *testing.T) {
		detector := NewOpenAICodexClientRestrictionDetector(nil)
		account := &Account{
			Platform: PlatformOpenAI,
			Type:     AccountTypeOAuth,
			Extra:    map[string]any{"codex_cli_only": true},
		}
		result := detector.Detect(
			newCodexDetectorTestContext("curl/8.0", "my_client"),
			account,
			CodexRestrictionPolicy{Whitelist: []openai.AllowedClientEntry{{Originator: "Claude Code", UAContains: []string{"Claude Code/"}}}},
			nil,
		)
		require.True(t, result.Enabled)
		require.False(t, result.Matched)
		require.Equal(t, CodexClientRestrictionReasonNotMatchedUA, result.Reason)
	})

	t.Run("全局列表为空 + 账号未配 → 403", func(t *testing.T) {
		detector := NewOpenAICodexClientRestrictionDetector(nil)
		account := &Account{
			Platform: PlatformOpenAI,
			Type:     AccountTypeOAuth,
			Extra:    map[string]any{"codex_cli_only": true},
		}
		result := detector.Detect(
			newCodexDetectorTestContext("Claude Code/0.5.0 (Macos) (Claude Code; 1.0.4)", "Claude Code"),
			account,
			CodexRestrictionPolicy{},
			nil,
		)
		require.True(t, result.Enabled)
		require.False(t, result.Matched)
		require.Equal(t, CodexClientRestrictionReasonNotMatchedUA, result.Reason)
	})

}

func TestDetect_V3_AppServerAndSkipAndVersionScope(t *testing.T) {
	gin.SetMode(gin.TestMode)
	acc := func() *Account {
		return &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth, Extra: map[string]any{"codex_cli_only": true}}
	}

	t.Run("AppServer OFF：未列名客户端拒", func(t *testing.T) {
		d := NewOpenAICodexClientRestrictionDetector(nil)
		r := d.Detect(newCodexDetectorTestContext("opencode/1.0", "opencode"), acc(), CodexRestrictionPolicy{}, nil)
		require.False(t, r.Matched)
		require.Equal(t, CodexClientRestrictionReasonNotMatchedUA, r.Reason)
	})

	t.Run("AppServer ON + 引擎头 → 放行(app_server)", func(t *testing.T) {
		d := NewOpenAICodexClientRestrictionDetector(nil)
		c := newCodexDetectorTestContext("opencode/1.0", "opencode")
		c.Request.Header.Set("x-codex-window-id", "1")
		r := d.Detect(c, acc(), CodexRestrictionPolicy{AllowAppServerClients: true, EngineFingerprintSignals: openai.DefaultEngineFingerprintSignals}, nil)
		require.True(t, r.Matched)
		require.Equal(t, CodexClientRestrictionReasonMatchedAppServerClient, r.Reason)
	})

	t.Run("AppServer ON + 无引擎头 + strict → 拒", func(t *testing.T) {
		d := NewOpenAICodexClientRestrictionDetector(nil)
		r := d.Detect(newCodexDetectorTestContext("opencode/1.0", "opencode"), acc(), CodexRestrictionPolicy{AllowAppServerClients: true, EngineFingerprintSignals: openai.DefaultEngineFingerprintSignals}, nil)
		require.False(t, r.Matched)
		require.Equal(t, CodexClientRestrictionReasonMissingEngineFingerprint, r.Reason)
	})

	t.Run("白名单 skip=true + 无引擎头 + strict → 放行", func(t *testing.T) {
		d := NewOpenAICodexClientRestrictionDetector(nil)
		pol := CodexRestrictionPolicy{
			Whitelist: []openai.AllowedClientEntry{{Originator: "opencode", UAContains: []string{"opencode/"}, SkipEngineFingerprint: true}},
		}
		r := d.Detect(newCodexDetectorTestContext("opencode/1.0", "opencode"), acc(), pol, nil)
		require.True(t, r.Matched)
		require.Equal(t, CodexClientRestrictionReasonMatchedWhitelistClient, r.Reason)
	})

	t.Run("白名单 skip=false + 无引擎头 + strict → 拒", func(t *testing.T) {
		d := NewOpenAICodexClientRestrictionDetector(nil)
		pol := CodexRestrictionPolicy{
			EngineFingerprintSignals: openai.DefaultEngineFingerprintSignals,
			Whitelist:                []openai.AllowedClientEntry{{Originator: "opencode", UAContains: []string{"opencode/"}}},
		}
		r := d.Detect(newCodexDetectorTestContext("opencode/1.0", "opencode"), acc(), pol, nil)
		require.False(t, r.Matched)
		require.Equal(t, CodexClientRestrictionReasonMissingEngineFingerprint, r.Reason)
	})

	t.Run("版本门仅官方：白名单无版本不拒", func(t *testing.T) {
		d := NewOpenAICodexClientRestrictionDetector(nil)
		pol := CodexRestrictionPolicy{
			Whitelist: []openai.AllowedClientEntry{{Originator: "opencode", UAContains: []string{"opencode"}}},
		}
		r := d.Detect(newCodexDetectorTestContext("opencode", "opencode"), acc(), pol, nil)
		require.True(t, r.Matched)
		require.Equal(t, CodexClientRestrictionReasonMatchedWhitelistClient, r.Reason)
	})

	t.Run("版本门仍卡官方：官方 originator 无版本 → VersionUndetectable", func(t *testing.T) {
		d := NewOpenAICodexClientRestrictionDetector(nil)
		r := d.Detect(newCodexDetectorTestContext("noversion", "codex_cli_rs"), acc(), CodexRestrictionPolicy{}, nil)
		require.False(t, r.Matched)
		require.Equal(t, CodexClientRestrictionReasonVersionUndetectable, r.Reason)
	})
}

func TestDetect_EngineFingerprintSignals(t *testing.T) {
	gin.SetMode(gin.TestMode)
	det := NewOpenAICodexClientRestrictionDetector(&config.Config{})
	acct := &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth, Extra: map[string]any{"codex_cli_only": true}}

	officialUA := "codex_cli_rs/0.141.0 (x) (codex_cli_rs; 0.141.0)"
	policy := CodexRestrictionPolicy{
		EngineFingerprintSignals: openai.DefaultEngineFingerprintSignals, // 只勾 x-codex-
	}

	t.Run("官方UA+带x-codex-头 → 放行", func(t *testing.T) {
		c := newCodexDetectorTestContext(officialUA, "")
		c.Request.Header.Set("x-codex-window-id", "a1")
		got := det.Detect(c, acct, policy, nil)
		require.True(t, got.Matched)
	})
	t.Run("官方UA+无x-codex-头 → 拒(缺指纹)", func(t *testing.T) {
		c := newCodexDetectorTestContext(officialUA, "")
		c.Request.Header.Set("session-id", "u1") // 默认 session 未勾,不满足必须项
		got := det.Detect(c, acct, policy, nil)
		require.False(t, got.Matched)
		require.Equal(t, CodexClientRestrictionReasonMissingEngineFingerprint, got.Reason)
	})
	t.Run("body通道: 勾body_path后 仅body命中 → 放行", func(t *testing.T) {
		bodyPolicy := CodexRestrictionPolicy{
			EngineFingerprintSignals: []openai.EngineFingerprintSignal{
				{Type: openai.FingerprintSignalBodyPath, Match: []string{"client_metadata.x-codex-window-id"}, Required: true},
			},
		}
		c := newCodexDetectorTestContext(officialUA, "")
		got := det.Detect(c, acct, bodyPolicy, []byte(`{"client_metadata":{"x-codex-window-id":"c3"}}`))
		require.True(t, got.Matched)
	})
}

func TestDetect_AccountAppServerToggle(t *testing.T) {
	gin.SetMode(gin.TestMode)
	d := NewOpenAICodexClientRestrictionDetector(nil)
	acctOn := func() *Account {
		return &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth, Extra: map[string]any{"codex_cli_only": true, "codex_cli_only_allow_app_server": true}}
	}
	acctOff := func() *Account {
		return &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth, Extra: map[string]any{"codex_cli_only": true}}
	}
	withFP := func(ua, originator string) *gin.Context {
		c := newCodexDetectorTestContext(ua, originator)
		c.Request.Header.Set("x-codex-window-id", "1")
		return c
	}
	defaultSignals := CodexRestrictionPolicy{EngineFingerprintSignals: openai.DefaultEngineFingerprintSignals}

	t.Run("账号 app-server ON + 引擎头 → 放行(全局 OFF 也放行)", func(t *testing.T) {
		r := d.Detect(withFP("opencode/1.0", "opencode"), acctOn(), defaultSignals, nil)
		require.True(t, r.Matched)
		require.Equal(t, CodexClientRestrictionReasonMatchedAppServerClient, r.Reason)
	})

	t.Run("账号 app-server ON + 无引擎头 → 拒(缺指纹)", func(t *testing.T) {
		r := d.Detect(newCodexDetectorTestContext("opencode/1.0", "opencode"), acctOn(), defaultSignals, nil)
		require.False(t, r.Matched)
		require.Equal(t, CodexClientRestrictionReasonMissingEngineFingerprint, r.Reason)
	})

	t.Run("账号 app-server OFF + 全局 OFF → 拒(未命中)", func(t *testing.T) {
		r := d.Detect(withFP("opencode/1.0", "opencode"), acctOff(), defaultSignals, nil)
		require.False(t, r.Matched)
		require.Equal(t, CodexClientRestrictionReasonNotMatchedUA, r.Reason)
	})

	t.Run("账号 app-server OFF + 全局 ON → 放行(OR)", func(t *testing.T) {
		pol := CodexRestrictionPolicy{AllowAppServerClients: true, EngineFingerprintSignals: openai.DefaultEngineFingerprintSignals}
		r := d.Detect(withFP("opencode/1.0", "opencode"), acctOff(), pol, nil)
		require.True(t, r.Matched)
		require.Equal(t, CodexClientRestrictionReasonMatchedAppServerClient, r.Reason)
	})
}
