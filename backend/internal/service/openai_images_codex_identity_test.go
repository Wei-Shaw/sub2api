package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestOAuthImageCodexIdentityPreservesNonIdentityHeaders(t *testing.T) {
	h := http.Header{"Authorization": {"Bearer fixture"}, "Session_id": {"isolated-session"}, "Chatgpt-Account-Id": {"fixture-account"}, "Openai-Beta": {"responses=experimental"}, "Accept": {"text/event-stream"}}
	before := h.Clone()
	applyOAuthImageCodexIdentity(h, &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth})
	require.Equal(t, "codex_exec/0.166.0 (Mac OS 15.6.0; arm64) unknown (codex_exec; 0.166.0)", h.Get("User-Agent"))
	require.Equal(t, "codex_exec", h.Get("Originator"))
	require.Equal(t, "0.166.0", h.Get("Version"))
	for _, key := range []string{"User-Agent", "Originator", "Version"} {
		h.Del(key)
	}
	require.Equal(t, before, h)
}

func TestOAuthImageCodexIdentityOtherChannelsUnchanged(t *testing.T) {
	for _, account := range []*Account{nil, {Platform: PlatformOpenAI, Type: AccountTypeAPIKey}, {Platform: PlatformOpenAI, Type: AccountTypeSetupToken}, {Platform: PlatformOpenAI, Type: AccountTypeWebImage}, nativeImageTestAccount(), {Platform: PlatformOpenAI, Type: AccountTypeOAuth, Extra: map[string]any{featureKeyChatGPTWebImageGeneration: true}}} {
		h := http.Header{"User-Agent": {"existing-client"}, "Originator": {"existing-origin"}, "Version": {"existing-version"}}
		before := h.Clone()
		applyOAuthImageCodexIdentity(h, account)
		require.Equal(t, before, h)
	}
}

func TestOAuthImageCodexIdentityTextRequestUnchanged(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/v1/responses", nil)
	account := &Account{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Credentials: map[string]any{"chatgpt_account_id": "fixture-account"}}
	svc := &OpenAIGatewayService{}
	req, err := svc.buildUpstreamRequest(context.Background(), c, account, []byte(`{"model":"gpt-6.1-sol","input":"hello"}`), "fixture", true, "", false)
	require.NoError(t, err)
	require.Equal(t, CodexCanonicalUserAgent(), req.Header.Get("User-Agent"))
	require.Equal(t, CodexCanonicalClientVersion(), req.Header.Get("Version"))
	require.Equal(t, chatgptCodexURL, req.URL.String())
}

// Prevent a future version bump from leaving image-only paths on an old identity.
func TestImageCodexIdentitySharesCanonicalVersion(t *testing.T) {
	require.Equal(t, codexCLIVersion, oauthImageCodexCLIVersion)
	require.Equal(t, codexCLIUserAgent, oauthImageCodexCLIUserAgent)
	require.Equal(t, codexCLIUserAgent, nativeImageCLIUserAgent)
}
