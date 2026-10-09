package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// Gemini 账号绝不能进入 Anthropic 协议转发链：该链按 Anthropic 上游拼地址并按
// Anthropic 方式携带凭据（无 base_url 的 Gemini API Key 账号会回落 api.anthropic.com）。
// 服务未装配 httpUpstream，若守卫失效会在发请求时 panic。
func TestAnthropicChainRejectsGeminiAccounts(t *testing.T) {
	gin.SetMode(gin.TestMode)
	account := &Account{ID: 7, Platform: PlatformGemini, Type: AccountTypeAPIKey, Credentials: map[string]any{"api_key": "google-secret-key"}}
	svc := &GatewayService{}
	newCtx := func() *gin.Context {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
		return c
	}

	body := []byte(`{"model":"my-model","max_tokens":16,"messages":[{"role":"user","content":"hi"}]}`)
	parsed, err := ParseGatewayRequest(NewRequestBodyRef(body), PlatformAnthropic)
	require.NoError(t, err)
	_, err = svc.Forward(context.Background(), newCtx(), account, parsed)
	require.ErrorContains(t, err, "gemini account")

	_, err = svc.ForwardAsResponses(context.Background(), newCtx(), account, []byte(`{"model":"my-model","input":"hi"}`), parsed)
	require.ErrorContains(t, err, "gemini account")

	_, err = svc.ForwardAsChatCompletions(context.Background(), newCtx(), account, []byte(`{"model":"my-model","messages":[{"role":"user","content":"hi"}]}`), parsed)
	require.ErrorContains(t, err, "gemini account")

	require.NoError(t, rejectGeminiAccountOnAnthropicChain(&Account{Platform: PlatformAnthropic}))
	require.NoError(t, rejectGeminiAccountOnAnthropicChain(&Account{Platform: PlatformAntigravity}))
	require.NoError(t, rejectGeminiAccountOnAnthropicChain(nil))
}

// count_tokens：Gemini 账号与 Antigravity 一样返回 404 让客户端回落本地估算，不发上游。
func TestForwardCountTokensGeminiAccountReturnsNotFoundWithoutUpstream(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages/count_tokens", nil)
	account := &Account{ID: 8, Platform: PlatformGemini, Type: AccountTypeAPIKey, Credentials: map[string]any{"api_key": "google-secret-key"}}
	parsed, err := ParseGatewayRequest(NewRequestBodyRef([]byte(`{"model":"my-model","messages":[{"role":"user","content":"hi"}]}`)), PlatformAnthropic)
	require.NoError(t, err)

	require.NoError(t, (&GatewayService{}).ForwardCountTokens(context.Background(), c, account, parsed))
	require.Equal(t, http.StatusNotFound, rec.Code)
}
