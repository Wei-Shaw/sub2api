package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// generic 网关（composite 跨族账号池经 Anthropic 协议链服务 ollama_cloud）同样在上游
// I/O 之前拒绝无价出站模型，不得先服务再按 0 元计费。
func TestGenericGatewayOllamaCloudPricingPreflight(t *testing.T) {
	gin.SetMode(gin.TestMode)
	billing := NewBillingService(&config.Config{}, nil)
	svc := &GatewayService{resolver: NewModelPricingResolver(nil, billing)}
	ollama := &Account{ID: 9, Platform: PlatformOllamaCloud, Type: AccountTypeAPIKey,
		Credentials: map[string]any{"api_key": "sk-ollama"}}

	newCtx := func(path string) (*gin.Context, *httptest.ResponseRecorder) {
		rec := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rec)
		c.Request = httptest.NewRequest(http.MethodPost, path, nil)
		return c, rec
	}

	c, rec := newCtx("/v1/messages")
	_, _, err := svc.buildUpstreamRequest(context.Background(), c, ollama,
		[]byte(`{"model":"unpriced-ollama-model","max_tokens":8,"messages":[]}`), "sk-ollama", "apikey", "unpriced-ollama-model", false, false)
	require.Error(t, err)
	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Equal(t, "error", gjson.GetBytes(rec.Body.Bytes(), "type").String(), "Anthropic 入站使用 Anthropic 错误信封")
	require.Equal(t, "invalid_request_error", gjson.GetBytes(rec.Body.Bytes(), "error.type").String())

	c, rec = newCtx("/v1/responses")
	_, _, err = svc.buildUpstreamRequest(context.Background(), c, ollama,
		[]byte(`{"model":"unpriced-ollama-model","max_tokens":8,"messages":[]}`), "sk-ollama", "apikey", "unpriced-ollama-model", false, false)
	require.Error(t, err)
	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.False(t, gjson.GetBytes(rec.Body.Bytes(), "type").Exists(), "OpenAI 入站使用 OpenAI 错误信封")

	// 有价模型、非 ollama 账号不受预检影响。
	require.NoError(t, svc.enforceOllamaCloudRequestPricingPreflight(context.Background(), nil, ollama, []byte(`{"model":"gpt-oss:120b"}`), ""))
	require.NoError(t, svc.enforceOllamaCloudRequestPricingPreflight(context.Background(), nil,
		&Account{ID: 10, Platform: PlatformKimi, Type: AccountTypeAPIKey}, []byte(`{"model":"unpriced-ollama-model"}`), ""))
}
