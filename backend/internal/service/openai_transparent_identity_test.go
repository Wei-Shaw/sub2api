package service

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/openai"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestResolveCodexTransparentIdentityPolicy(t *testing.T) {
	fallback := codexOutboundIdentity{
		userAgent:  "codex-tui/0.146.0 (Linux)",
		originator: "codex-tui",
		version:    "0.146.0",
	}
	cases := []struct {
		name        string
		ua          string
		wantVersion string
	}{
		{"minimum", "codex-tui/0.144.0 (Mac OS X; arm64)", "0.144.0"},
		{"two_parts", "codex-tui/0.144 (Linux)", "0.144"},
		{"windows", "codex_cli_rs/0.145.2 (Windows NT; x86_64)", "0.145.2"},
		{"desktop", "Codex Desktop/0.145.2 (Mac OS X; arm64)", "0.145.2"},
		{"later_prerelease", "codex-tui/0.144.1-alpha.1 (Linux)", "0.144.1-alpha.1"},
		{"minimum_prerelease", "codex-tui/0.144.0-alpha.1 (Linux)", ""},
		{"old", "codex-tui/0.143.9 (Mac OS X; arm64)", ""},
		{"missing", "", ""},
		{"invalid_version", "codex-tui/not-a-version (Linux)", ""},
		{"missing_version", "codex-tui/ (Linux)", ""},
		{"four_parts", "codex-tui/0.144.0.1 (Linux)", ""},
		{"leading_zero", "codex-tui/00.144.0 (Linux)", ""},
		{"third_party", "other-client/1.0.0", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			identity := resolveCodexTransparentIdentity(tc.ua, fallback)
			if tc.wantVersion == "" {
				require.Equal(t, fallback, identity)
			} else {
				require.Equal(t, tc.ua, identity.userAgent)
				require.Equal(t, tc.wantVersion, identity.version)
			}
			require.Equal(t, identity.version, openai.CodexUserAgentVersion(identity.userAgent))
			originator, _, ok := openai.PairCodexClientIdentity(identity.userAgent)
			require.True(t, ok)
			require.Equal(t, originator, identity.originator)
			require.Equal(t, identity, resolveCodexTransparentIdentity(identity.userAgent, fallback))
		})
	}
}

func TestTransparentCodexIdentityHTTPAndWebSocketAgree(t *testing.T) {
	wasEnabled := codexIdentityEnforcement.Load()
	SetCodexIdentityEnforcementEnabled(false)
	t.Cleanup(func() { SetCodexIdentityEnforcementEnabled(wasEnabled) })

	for _, tc := range []struct {
		ua, wantUA, version string
	}{
		{"codex-tui/0.145.2 (Mac OS X 14.0; arm64) iTerm", "codex-tui/0.145.2 (Mac OS X 14.0; arm64) iTerm", "0.145.2"},
		{"codex_cli_rs/0.144.0 (Windows NT; x86_64)", "codex_cli_rs/0.144.0 (Windows NT; x86_64)", "0.144.0"},
		{"codex-tui/0.143.9 (Mac OS X; arm64)", codexCLIUserAgent, codexCLIVersion},
		{"codex-tui/0.144.0-alpha.1 (Linux)", codexCLIUserAgent, codexCLIVersion},
		{"codex-tui/invalid (Linux)", codexCLIUserAgent, codexCLIVersion},
	} {
		t.Run(tc.ua, func(t *testing.T) {
			body := []byte(`{"model":"audit-model","stream":true}`)
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))
			c.Request.Header.Set("User-Agent", tc.ua)
			account := &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth,
				Credentials: map[string]any{"chatgpt_account_id": "audit-account"}}
			svc := &OpenAIGatewayService{}
			req, err := svc.buildUpstreamRequest(context.Background(), c, account, body, "dummy-token", true, "", true)
			require.NoError(t, err)
			headers, _, err := svc.buildOpenAIWSHeaders(context.Background(), c, account, "dummy-token",
				OpenAIWSProtocolDecision{Transport: OpenAIUpstreamTransportResponsesWebsocketV2}, true, "", "", "", "", "")
			require.NoError(t, err)
			originator, _, ok := openai.PairCodexClientIdentity(tc.wantUA)
			require.True(t, ok)
			for _, h := range []http.Header{req.Header, headers} {
				require.Equal(t, tc.wantUA, h.Get("User-Agent"))
				require.Equal(t, tc.version, h.Get("version"))
				require.Equal(t, originator, h.Get("originator"))
			}
		})
	}
}
