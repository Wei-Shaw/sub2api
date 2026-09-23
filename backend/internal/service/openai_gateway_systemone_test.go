//go:build unit

package service

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func systemOneTestAccount(id int64) *Account {
	return &Account{
		ID:          id,
		Name:        "oc-systemone",
		Platform:    PlatformOpenCodeGo,
		Type:        AccountTypeAPIKey,
		Status:      StatusActive,
		Concurrency: 1,
		Credentials: map[string]any{
			"api_key":      "sk-systemone-test",
			"api_protocol": APIProtocolAdaptive,
			"account_mode": AccountModeZen,
			"base_url":     "https://opencode.ai/zen/v1",
		},
	}
}

func systemOneTestResponse() *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body: io.NopCloser(strings.NewReader(`{
			"model": "jev-1.13.0",
			"answers": {"is_urgent": {"type": "noul", "noul": 0.97}},
			"usage": {"input_tokens": 392, "output_tokens": 65}
		}`)),
	}
}

func systemOneTestBody() []byte {
	return []byte(`{
		"model": "jev-1.13",
		"state": "My payments have failed for three days and I am losing sales.",
		"questions": {"is_urgent": {"type": "noul", "instructions": "Does this request require urgent attention?"}}
	}`)
}

func TestBuildOpenAISystemOneURL(t *testing.T) {
	t.Parallel()
	require.Equal(t, "https://opencode.ai/zen/v1/systemone", buildOpenAISystemOneURL("https://opencode.ai/zen/v1"))
	require.Equal(t, "https://opencode.ai/zen/go/v1/systemone", buildOpenAISystemOneURL("https://opencode.ai/zen/go/v1"))
	require.Equal(t, "https://opencode.ai/zen/v1/systemone", buildOpenAISystemOneURL("https://opencode.ai/zen/v1/systemone"))
	require.Equal(t, "https://opencode.ai/zen/v1/systemone", buildOpenAISystemOneURL("https://opencode.ai/zen/v1/"))
}

func TestIsOpenCodeGoSystemOneModel(t *testing.T) {
	t.Parallel()
	require.True(t, isOpenCodeGoSystemOneModel("jev-1.13"))
	require.True(t, isOpenCodeGoSystemOneModel("jev-1.13-free"))
	require.True(t, isOpenCodeGoSystemOneModel("JEV-LATEST"))
	require.True(t, isOpenCodeGoSystemOneModel("opencode/jev-1.13"))
	require.False(t, isOpenCodeGoSystemOneModel("gpt-5.5"))
	require.False(t, isOpenCodeGoSystemOneModel("glm-5.3"))
	require.False(t, isOpenCodeGoSystemOneModel(""))
}

func TestDefaultOpenCodeGoModelIDsContainJev(t *testing.T) {
	t.Parallel()
	ids := DefaultOpenCodeGoModelIDs()
	require.Contains(t, ids, "jev-1.13")
	require.Contains(t, ids, "jev-1.13-free")
}

func TestForwardSystemOne_ForwardsToZenSystemOne(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	upstream := &httpUpstreamRecorder{resp: systemOneTestResponse()}
	svc := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig(), httpUpstream: upstream}
	account := systemOneTestAccount(501)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/systemone", nil)

	result, err := svc.ForwardSystemOne(context.Background(), c, account, systemOneTestBody(), "")
	require.NoError(t, err)
	require.NotNil(t, result)

	require.Len(t, upstream.requests, 1)
	req := upstream.requests[0]
	require.Equal(t, "https://opencode.ai/zen/v1/systemone", req.URL.String())
	require.Equal(t, "Bearer sk-systemone-test", req.Header.Get("Authorization"))
	require.Equal(t, openCodeUpstreamUserAgent, req.Header.Get("User-Agent"))
	require.Empty(t, req.Header.Get("X-OpenCode-Session"), "Zen 账号无调用方会话标识时不伪造会话头")
	require.Equal(t, "jev-1.13", gjson.GetBytes(upstream.lastBody, "model").String())

	// 上游解析后的版本原样透传，不回写请求模型。
	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t, "jev-1.13.0", gjson.GetBytes(recorder.Body.Bytes(), "model").String())
	require.Equal(t, 0.97, gjson.GetBytes(recorder.Body.Bytes(), "answers.is_urgent.noul").Float())

	require.Equal(t, "jev-1.13", result.Model)
	require.Equal(t, "jev-1.13", result.BillingModel)
	require.Equal(t, "jev-1.13", result.UpstreamModel)
	require.Equal(t, openAISystemOneUpstreamEndpoint, result.UpstreamEndpoint)
	require.False(t, result.Stream)
}

func TestForwardSystemOne_GoAccountGeneratesSessionHeader(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	upstream := &httpUpstreamRecorder{resp: systemOneTestResponse()}
	svc := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig(), httpUpstream: upstream}
	account := systemOneTestAccount(504)
	delete(account.Credentials, "account_mode")
	account.Credentials["base_url"] = "https://opencode.ai/zen/go/v1"

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/systemone", nil)

	result, err := svc.ForwardSystemOne(context.Background(), c, account, systemOneTestBody(), "")
	require.NoError(t, err)
	require.NotNil(t, result)

	require.Len(t, upstream.requests, 1)
	req := upstream.requests[0]
	require.Equal(t, "https://opencode.ai/zen/go/v1/systemone", req.URL.String())
	require.NotEmpty(t, req.Header.Get("X-OpenCode-Session"), "Go 账号自动生成会话头")
}

func TestAccountTestService_OpenCodeGoJevUsesSystemOne(t *testing.T) {
	account := openCodeGoTestAccount(501)
	svc, upstream := adaptiveCNAccountTestService(account, systemOneTestResponse())
	c, recorder := newTestContext()

	err := svc.TestAccountConnection(c, account.ID, "jev-1.13", "probe state", AccountTestModeDefault)
	require.NoError(t, err)
	require.Len(t, upstream.requests, 1)
	require.Equal(t, "https://opencode.ai/zen/go/v1/systemone", upstream.requests[0].URL.String())
	require.Equal(t, "Bearer sk-opencode-go-test", upstream.requests[0].Header.Get("Authorization"))
	require.NotEmpty(t, upstream.requests[0].Header.Get("X-OpenCode-Session"))
	require.Equal(t, "jev-1.13", gjson.GetBytes(upstream.lastBody, "model").String())
	require.Contains(t, recorder.Body.String(), `"type":"test_complete"`)
}

func TestForwardSystemOne_AppliesModelMapping(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	upstream := &httpUpstreamRecorder{resp: systemOneTestResponse()}
	svc := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig(), httpUpstream: upstream}
	account := systemOneTestAccount(502)
	account.Credentials["model_mapping"] = map[string]any{"jev-1.13": "jev-1.13-pinned"}

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/systemone", nil)

	result, err := svc.ForwardSystemOne(context.Background(), c, account, systemOneTestBody(), "")
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, "jev-1.13-pinned", gjson.GetBytes(upstream.lastBody, "model").String())
	require.Equal(t, "jev-1.13", result.Model)
}

func TestForwardSystemOne_MissingModel(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	upstream := &httpUpstreamRecorder{resp: systemOneTestResponse()}
	svc := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig(), httpUpstream: upstream}

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/systemone", nil)

	result, err := svc.ForwardSystemOne(context.Background(), c, systemOneTestAccount(503), []byte(`{"state":"x","questions":{"q":{"type":"noul","instructions":"x"}}}`), "")
	require.Error(t, err)
	require.Nil(t, result)
	require.Empty(t, upstream.requests)
	require.Equal(t, http.StatusBadRequest, recorder.Code)
}

func TestAccountTestService_OpenCodeGoJevUpstreamError(t *testing.T) {
	account := openCodeGoTestAccount(502)
	account.Credentials["account_mode"] = AccountModeZen
	account.Credentials["base_url"] = "https://opencode.ai/zen/v1"
	svc, _ := adaptiveCNAccountTestService(account, &http.Response{
		StatusCode: http.StatusUnauthorized,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(`{"error":{"message":"invalid key","type":"authentication_error"}}`)),
	})
	c, recorder := newTestContext()

	err := svc.TestAccountConnection(c, account.ID, "jev-1.13-free", "probe state", AccountTestModeDefault)
	require.Error(t, err)
	require.Contains(t, recorder.Body.String(), `"type":"error"`)
	require.NotContains(t, recorder.Body.String(), `"type":"test_complete"`)
}
