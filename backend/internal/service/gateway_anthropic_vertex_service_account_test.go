package service

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestGatewayService_BuildAnthropicVertexServiceAccountRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	c.Request.Header.Set("Authorization", "Bearer inbound-token")
	c.Request.Header.Set("X-Api-Key", "inbound-api-key")
	c.Request.Header.Set("Anthropic-Version", "2023-06-01")
	c.Request.Header.Set("Anthropic-Beta", "interleaved-thinking-2025-05-14")

	account := &Account{
		ID:       301,
		Platform: PlatformAnthropic,
		Type:     AccountTypeServiceAccount,
		Credentials: map[string]any{
			"project_id": "vertex-proj",
			"location":   "us-east5",
		},
	}
	body := []byte(`{"model":"claude-sonnet-4-5","stream":false,"max_tokens":32,"messages":[{"role":"user","content":"hello"}]}`)

	svc := &GatewayService{}
	req, err := svc.buildUpstreamRequest(
		context.Background(),
		c,
		account,
		body,
		"vertex-token",
		"service_account",
		"claude-sonnet-4-5@20250929",
		false,
		false,
	)
	require.NoError(t, err)
	require.Equal(t, "https://us-east5-aiplatform.googleapis.com/v1/projects/vertex-proj/locations/us-east5/publishers/anthropic/models/claude-sonnet-4-5@20250929:rawPredict", req.URL.String())
	require.Equal(t, "Bearer vertex-token", getHeaderRaw(req.Header, "authorization"))
	require.Empty(t, getHeaderRaw(req.Header, "x-api-key"))
	require.Empty(t, getHeaderRaw(req.Header, "anthropic-version"))
	require.Equal(t, "interleaved-thinking-2025-05-14", getHeaderRaw(req.Header, "anthropic-beta"))

	got := readRequestBodyForTest(t, req)
	require.Equal(t, "", gjson.GetBytes(got, "model").String())
	require.Equal(t, vertexAnthropicVersion, gjson.GetBytes(got, "anthropic_version").String())
	require.Equal(t, "hello", gjson.GetBytes(got, "messages.0.content").String())
}

func readRequestBodyForTest(t *testing.T, req *http.Request) []byte {
	t.Helper()
	require.NotNil(t, req.Body)
	body, err := io.ReadAll(req.Body)
	require.NoError(t, err)
	return body
}

// Vertex 路径回归保护：同样需要
// body↔beta header 能力维度对称。客户端 header 不带 context-management beta
// 但 body 带 context_management 字段 → Vertex builder 必须 strip 字段，与 Anthropic
// 直连 / Bedrock 路径保持一致。
func TestGatewayService_BuildAnthropicVertexServiceAccount_StripsContextManagementWhenBetaMissing(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	// 客户端 header 只带 interleaved-thinking，不带 context-management-2025-06-27
	c.Request.Header.Set("Anthropic-Beta", "interleaved-thinking-2025-05-14")

	account := &Account{
		ID: 302, Platform: PlatformAnthropic, Type: AccountTypeServiceAccount,
		Credentials: map[string]any{"project_id": "vertex-proj", "location": "us-east5"},
	}
	// body 带了 context_management 字段（客户端透传 / normalize 补齐 / mimicry 注入等场景都可能导致）
	body := []byte(`{"model":"claude-haiku-4-5","context_management":{"edits":[{"type":"clear_thinking_20251015","keep":"all"}]},"messages":[{"role":"user","content":"hi"}]}`)

	svc := &GatewayService{}
	req, err := svc.buildUpstreamRequest(
		context.Background(), c, account, body,
		"vertex-token", "service_account", "claude-haiku-4-5@20251001", false, false,
	)
	require.NoError(t, err)

	got := readRequestBodyForTest(t, req)
	require.False(t, gjson.GetBytes(got, "context_management").Exists(),
		"Vertex 路径下客户端 header 缺 context-management beta 时，必须 strip body 同名字段")
	// header 对称断言：覆盖未来某人在 Vertex builder 里加入与 sanitize 不一致的 header 处理。
	outBeta := getHeaderRaw(req.Header, "anthropic-beta")
	require.False(t, anthropicBetaTokensContains(outBeta, "context-management-2025-06-27"),
		"与 body 对称：outgoing anthropic-beta header 也不含 context-management beta")
}

// Vertex 路径反面：客户端 header 含 context-management beta 时保留字段。
func TestGatewayService_BuildAnthropicVertexServiceAccount_PreservesContextManagementWhenBetaPresent(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	c.Request.Header.Set("Anthropic-Beta", "interleaved-thinking-2025-05-14,context-management-2025-06-27")

	account := &Account{
		ID: 303, Platform: PlatformAnthropic, Type: AccountTypeServiceAccount,
		Credentials: map[string]any{"project_id": "vertex-proj", "location": "us-east5"},
	}
	body := []byte(`{"model":"claude-sonnet-4-6","context_management":{"edits":[{"type":"clear_thinking_20251015"}]},"messages":[]}`)

	svc := &GatewayService{}
	req, err := svc.buildUpstreamRequest(
		context.Background(), c, account, body,
		"vertex-token", "service_account", "claude-sonnet-4-6@20260218", false, false,
	)
	require.NoError(t, err)

	got := readRequestBodyForTest(t, req)
	require.True(t, gjson.GetBytes(got, "context_management").Exists(),
		"Vertex + 客户端 header 包含 context-management beta 时字段必须保留")
	outBeta := getHeaderRaw(req.Header, "anthropic-beta")
	require.True(t, anthropicBetaTokensContains(outBeta, "context-management-2025-06-27"),
		"与 body 对称：outgoing anthropic-beta header 同步含 context-management beta")
}
