package service

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/openai"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// Exercise the builders, not just the final pairing function: filtering ingress
// identity before pairing used to make disabling enforcement ineffective.
// These tests only construct requests; they never send traffic upstream.
func TestCodexIdentityEnforcementRequestBuilders(t *testing.T) {
	previousGinMode := gin.Mode()
	gin.SetMode(gin.TestMode)
	t.Cleanup(func() { gin.SetMode(previousGinMode) })
	previous := codexIdentityEnforcement.Load()
	t.Cleanup(func() { SetCodexIdentityEnforcementEnabled(previous) })
	const tuiUA = "codex-tui/0.145.2 (Mac OS 15.6.0; arm64) iTerm.app/3.6.10 (codex-tui; 0.145.2)"
	const remoteTUIUA = "codex-tui/0.150.0 (Mac OS 15.6.0; arm64) unknown (codex-tui; 0.149.0)"
	const customUA = "codex_exec/0.150.0 (Mac OS 15.6.0; arm64) unknown (codex_exec; 0.150.0)"

	for _, enabled := range []bool{true, false} {
		for _, accountType := range []string{AccountTypeOAuth, AccountTypeSetupToken, AccountTypeAPIKey} {
			for _, transport := range []string{"http", "passthrough", "http-compact", "passthrough-compact", "ws"} {
				for _, tc := range []struct {
					name, inboundUA, customUA string
					forceCodexCLI             bool
				}{
					{name: "official", inboundUA: tuiUA},
					{name: "remote-tui-ingress", inboundUA: remoteTUIUA},
					{name: "third-party", inboundUA: "luna/1.0.0"},
					{name: "account-override", inboundUA: tuiUA, customUA: customUA},
					{name: "account-remote-tui", inboundUA: tuiUA, customUA: remoteTUIUA},
					{name: "force-overrides-account", inboundUA: tuiUA, customUA: customUA, forceCodexCLI: true},
				} {
					t.Run(fmt.Sprintf("enforced=%t/%s/%s/%s", enabled, accountType, transport, tc.name), func(t *testing.T) {
						SetCodexIdentityEnforcementEnabled(enabled)
						cfg := &config.Config{}
						cfg.Gateway.ForceCodexCLI = tc.forceCodexCLI
						svc := &OpenAIGatewayService{cfg: cfg}
						account := &Account{
							ID: 999, Platform: PlatformOpenAI, Type: accountType,
							Credentials: map[string]any{
								"access_token": "test-token", "chatgpt_account_id": "test-account",
								"user_agent": tc.customUA,
							},
						}
						path := "/v1/responses"
						if transport == "http-compact" || transport == "passthrough-compact" {
							path += "/compact"
						}
						c, _ := gin.CreateTestContext(httptest.NewRecorder())
						c.Request = httptest.NewRequest(http.MethodPost, path, nil)
						c.Request.Header.Set("User-Agent", tc.inboundUA)
						c.Request.Header.Set("originator", "deliberately-mismatched")
						c.Request.Header.Set("version", "9.9.9")

						var h http.Header
						switch transport {
						case "http", "http-compact":
							req, err := svc.buildUpstreamRequest(context.Background(), c, account, []byte(`{}`), "test-token", true, "", true)
							require.NoError(t, err)
							h = req.Header
						case "passthrough", "passthrough-compact":
							req, err := svc.buildUpstreamRequestOpenAIPassthrough(context.Background(), c, account, []byte(`{}`), "test-token")
							require.NoError(t, err)
							h = req.Header
						case "ws":
							var err error
							h, _, err = svc.buildOpenAIWSHeaders(context.Background(), c, account, "test-token",
								OpenAIWSProtocolDecision{Transport: OpenAIUpstreamTransportResponsesWebsocketV2}, true, "", "", "", "", "")
							require.NoError(t, err)
						}

						wantUA := tc.inboundUA
						if tc.customUA != "" {
							wantUA = tc.customUA
						}
						if tc.forceCodexCLI {
							wantUA = CodexCanonicalUserAgent()
						}
						if account.UsesOpenAICodexProtocol() {
							if enabled {
								wantUA = resolveCodexOutboundIdentity(svc.codexIdentityOverrideUA(account)).userAgent
							} else if tc.name == "third-party" {
								wantUA = CodexCanonicalUserAgent()
							}
							wantOriginator, _, ok := openai.PairCodexClientIdentity(wantUA)
							require.True(t, ok)
							require.Equal(t, wantOriginator, h.Get("originator"))
							if enabled {
								require.Equal(t, openai.CodexUserAgentVersion(wantUA), h.Get("version"))
							}
						}
						require.Equal(t, wantUA, h.Get("User-Agent"))
						// Disabling enforcement does not expand the existing header whitelist.
						require.NotEqual(t, "9.9.9", h.Get("version"))
					})
				}
			}
		}
	}
}

// These are synthetic protocol fixtures, not attestations of released artifacts.
// A client name does not prove an embedded/same-build connection: remote TUI
// sends its own build version while the app-server supplies the Core version.
func TestCodexIdentityPreservesExplicitDualVersionProfiles(t *testing.T) {
	for _, tc := range []struct{ originator, frontend string }{
		{"codex-tui", "codex-tui"},
		{"codex_exec", "codex_exec"},
		{"codex_cli_rs", "codex_cli_rs"},
		{"Codex Desktop", "codex-tui"},
		{"codex-tui", "CODEX-TUI"},
	} {
		t.Run(tc.originator+"/"+tc.frontend, func(t *testing.T) {
			ua := fmt.Sprintf("%s/0.150.0 (Mac OS 15.6.0; arm64) unknown (%s; 0.149.0)", tc.originator, tc.frontend)
			profile, ok := openai.ParseCodexWireProfile(ua)
			require.True(t, ok)
			require.True(t, validCodexDualVersionProfile(profile))
			identity, ok := codexOutboundIdentityFromUA(ua, "0.200.1")
			require.True(t, ok, "different frontend/Core versions alone do not make a profile invalid")
			require.Equal(t, profile.UserAgent(), identity.userAgent)
			require.Equal(t, profile.Originator, identity.originator)
			require.Equal(t, "0.150.0", identity.version)
			require.Equal(t, identity, resolveCodexOutboundIdentity(ua))

			settings := NewSettingService(&codexVersionSettingRepoStub{values: map[string]string{
				SettingKeyOpenAICodexUserAgent: ua, SettingKeyOpenAICodexClientVersionSynced: "0.200.1",
			}}, nil)
			require.Equal(t, profile.UserAgent(), settings.GetOpenAICodexCanonicalUserAgent(context.Background()))
		})
	}
}

func TestCodexIdentityRejectsInvalidDualVersionProfiles(t *testing.T) {
	for _, tc := range []struct{ core, frontend string }{
		{"0.143.0", "0.149.0"},
		{"invalid", "0.149.0"},
		{"0.150.0", "invalid"},
	} {
		t.Run(tc.core+"/"+tc.frontend, func(t *testing.T) {
			ua := fmt.Sprintf("codex-tui/%s (Mac OS 15.6.0; arm64) unknown (codex-tui; %s)", tc.core, tc.frontend)
			_, ok := codexOutboundIdentityFromUA(ua, "0.200.1")
			require.False(t, ok)
			require.Equal(t, resolveCodexOutboundIdentity(""), resolveCodexOutboundIdentity(ua))
			settings := NewSettingService(&codexVersionSettingRepoStub{values: map[string]string{
				SettingKeyOpenAICodexUserAgent: ua, SettingKeyOpenAICodexClientVersionSynced: "0.200.1",
			}}, nil)
			require.Equal(t, codexCLIUserAgent, settings.GetOpenAICodexCanonicalUserAgent(context.Background()))
		})
	}
}

func TestCodexIdentityPreservesIndependentFrontendTuples(t *testing.T) {
	for _, ua := range []string{
		codexCLIUserAgent,
		"codex_vscode/0.153.4 (Mac OS 15.6.0; arm64) unknown (codex_vscode; 9.8.7)",
		"Codex Desktop/0.153.4 (Mac OS 15.6.0; arm64) unknown (codex_vscode; 9.8.7)",
		"codex-tui/0.153.4 (Mac OS 15.6.0; arm64) unknown (codex_vscode; 9.8.7)",
	} {
		t.Run(ua, func(t *testing.T) {
			identity, ok := codexOutboundIdentityFromUA(ua, "0.200.1")
			require.True(t, ok)
			require.Equal(t, ua, identity.userAgent)
			require.Equal(t, "0.153.4", identity.version)
			settings := NewSettingService(&codexVersionSettingRepoStub{values: map[string]string{
				SettingKeyOpenAICodexUserAgent: ua, SettingKeyOpenAICodexClientVersionSynced: "0.200.1",
			}}, nil)
			require.Equal(t, ua, settings.GetOpenAICodexCanonicalUserAgent(context.Background()))
		})
	}
}
