package service

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func forwardAPIKeyChatCompletionsForUpstreamBody(t *testing.T, body []byte) []byte {
	t.Helper()

	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusBadRequest,
		Header:     http.Header{"Content-Type": []string{"application/json"}, "x-request-id": []string{"rid_chat_apikey_system_promotion"}},
		Body:       io.NopCloser(strings.NewReader(`{"error":{"type":"invalid_request_error","message":"stop before response parsing"}}`)),
	}}
	svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream}
	account := &Account{
		ID:          5,
		Name:        "openai-apikey-relay",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Concurrency: 1,
		Credentials: map[string]any{"api_key": "sk-relay"},
		Extra:       map[string]any{"openai_responses_supported": true},
	}

	result, err := svc.ForwardAsChatCompletions(context.Background(), c, account, body, "", "gpt-5.4")
	require.Error(t, err)
	require.Nil(t, result)
	require.NotEmpty(t, upstream.lastBody)
	return upstream.lastBody
}

// Codex-compatible relays reject role:"system" in Responses input; API-key
// accounts skip the Codex transform, so the gateway must promote the prompt
// into instructions itself.
func TestForwardAsChatCompletions_APIKeyPromotesSystemMessageToInstructions(t *testing.T) {
	const systemPrompt = "Unique API-key system prefix for token accounting."
	body := []byte(`{"model":"gpt-5.4","messages":[{"role":"system","content":"` + systemPrompt + `"},{"role":"user","content":"hello"}],"stream":false}`)

	upstreamBody := forwardAPIKeyChatCompletionsForUpstreamBody(t, body)

	require.Equal(t, systemPrompt, gjson.GetBytes(upstreamBody, "instructions").String())
	require.Equal(t, int64(1), gjson.GetBytes(upstreamBody, "input.#").Int())
	require.Equal(t, "user", gjson.GetBytes(upstreamBody, "input.0.role").String())
	require.Equal(t, 1, strings.Count(string(upstreamBody), systemPrompt))
	require.NotContains(t, string(upstreamBody), `"role":"system"`)
}

func TestForwardAsChatCompletions_APIKeyKeepsMixedSystemContentInInput(t *testing.T) {
	const systemPrompt = "Inspect this reference image."
	body := []byte(`{"model":"gpt-5.4","messages":[{"role":"system","content":[{"type":"text","text":"` + systemPrompt + `"},{"type":"image_url","image_url":{"url":"https://example.com/reference.png"}}]},{"role":"user","content":"hello"}],"stream":false}`)

	upstreamBody := forwardAPIKeyChatCompletionsForUpstreamBody(t, body)

	require.Equal(t, systemPrompt, gjson.GetBytes(upstreamBody, "instructions").String())
	require.Equal(t, int64(2), gjson.GetBytes(upstreamBody, "input.#").Int())
	require.Equal(t, "developer", gjson.GetBytes(upstreamBody, "input.0.role").String())
	require.Equal(t, int64(2), gjson.GetBytes(upstreamBody, "input.0.content.#").Int())
	require.Equal(t, "input_image", gjson.GetBytes(upstreamBody, "input.0.content.1.type").String())
}
