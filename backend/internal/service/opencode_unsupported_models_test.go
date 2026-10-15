package service

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestForwardOpenCodeUnsupportedModels_ChatCompletionsReturns400(t *testing.T) {
	gin.SetMode(gin.TestMode)

	account := &Account{
		ID:       101,
		Platform: PlatformOpenCodeGo,
		Type:     AccountTypeAPIKey,
		Credentials: map[string]any{
			"api_key":      "test-key",
			"account_mode": AccountModeZen,
		},
	}

	svc := &OpenAIGatewayService{
		cfg: &config.Config{},
	}

	for _, model := range []string{"gemini-3.8-flash", "jev-1.13", "opencode/gemini-3.7-flash", "opencode-go/jev-1.13-free"} {
		rec := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rec)
		body := []byte(`{"model":"` + model + `","messages":[{"role":"user","content":"hello"}]}`)
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))

		result, err := svc.ForwardAsChatCompletions(context.Background(), c, account, body, "", "")
		require.Nil(t, result)
		require.Error(t, err)
		require.Contains(t, err.Error(), "opencode unsupported model")

		require.Equal(t, http.StatusBadRequest, rec.Code)
		respBody := rec.Body.String()
		require.Equal(t, "invalid_request_error", gjson.Get(respBody, "error.type").String())
		require.Equal(t, "model_not_supported", gjson.Get(respBody, "error.code").String())
		require.Contains(t, gjson.Get(respBody, "error.message").String(), "not supported on OpenCode standard gateway")
	}
}

func TestForwardOpenCodeUnsupportedModels_ResponsesReturns400(t *testing.T) {
	gin.SetMode(gin.TestMode)

	account := &Account{
		ID:       102,
		Platform: PlatformOpenCodeGo,
		Type:     AccountTypeAPIKey,
		Credentials: map[string]any{
			"api_key":      "test-key",
			"account_mode": AccountModeZen,
		},
	}

	svc := &OpenAIGatewayService{
		cfg: &config.Config{},
	}

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	body := []byte(`{"model":"gemini-3.8-flash","input":[{"role":"user","content":"hello"}]}`)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))

	result, err := svc.Forward(context.Background(), c, account, body)
	require.Nil(t, result)
	require.Error(t, err)
	require.Contains(t, err.Error(), "opencode unsupported model")

	require.Equal(t, http.StatusBadRequest, rec.Code)
	respBody := rec.Body.String()
	require.Equal(t, "invalid_request_error", gjson.Get(respBody, "error.type").String())
	require.Equal(t, "model_not_supported", gjson.Get(respBody, "error.code").String())
}

func TestForwardOpenCodeUnsupportedModels_MessagesReturns400(t *testing.T) {
	gin.SetMode(gin.TestMode)

	account := &Account{
		ID:       103,
		Platform: PlatformOpenCodeGo,
		Type:     AccountTypeAPIKey,
		Credentials: map[string]any{
			"api_key":      "test-key",
			"account_mode": AccountModeZen,
		},
	}

	svc := &OpenAIGatewayService{
		cfg: &config.Config{},
	}

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	body := []byte(`{"model":"gemini-3.8-flash","messages":[{"role":"user","content":"hello"}]}`)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewReader(body))

	result, err := svc.ForwardAsAnthropic(context.Background(), c, account, body, "", "")
	require.Nil(t, result)
	require.Error(t, err)
	require.Contains(t, err.Error(), "opencode unsupported model")

	require.Equal(t, http.StatusBadRequest, rec.Code)
	respBody := rec.Body.String()
	require.Equal(t, "invalid_request_error", gjson.Get(respBody, "error.type").String())
	require.Contains(t, gjson.Get(respBody, "error.message").String(), "not supported on OpenCode standard gateway")
}

func TestAccountTestService_OpenCodeUnsupportedModelReturnsError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/test", nil)

	account := &Account{
		ID:       104,
		Platform: PlatformOpenCodeGo,
		Type:     AccountTypeAPIKey,
	}

	svc := &AccountTestService{}
	err := svc.testModelRoutedAccountConnection(c, account, "gemini-3.8-flash", "hi")
	require.Error(t, err)
	require.Contains(t, err.Error(), "not supported on OpenCode standard gateway")

	err = svc.testModelRoutedAccountConnection(c, account, "jev-1.13", "hi")
	require.Error(t, err)
	require.Contains(t, err.Error(), "not supported on OpenCode standard gateway")
}
