//go:build unit

package service

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// Cline API 的非流式 chat/completions 响应会被包在 {"success":true,"data":{...}} 中，
// 而流式分片是标准格式。
const clineEnvelopedChatCompletion = `{"success":true,"data":{"id":"gen_01","object":"chat.completion","created":1791539666,"model":"deepseek/deepseek-v4.1-flash",` +
	`"choices":[{"index":0,"finish_reason":"stop","message":{"role":"assistant","content":"<severity>0</severity>","reasoning":"harmless"}}],` +
	`"usage":{"prompt_tokens":73,"completion_tokens":41,"total_tokens":114}}}`

const plainChatCompletion = `{"id":"chatcmpl-1","object":"chat.completion","created":1791539666,"model":"gpt-x",` +
	`"choices":[{"index":0,"finish_reason":"stop","message":{"role":"assistant","content":"hello"}}],` +
	`"usage":{"prompt_tokens":5,"completion_tokens":1,"total_tokens":6}}`

func newCCEnvelopeTestContext() (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	return c, rec
}

func newCCEnvelopeUpstreamResponse(body string) *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

func TestReadCCUpstreamJSONResponse_UnwrapsDataEnvelope(t *testing.T) {
	c, _ := newCCEnvelopeTestContext()
	s := &OpenAIGatewayService{}

	ccResp, usage, err := s.readCCUpstreamJSONResponse(c, newCCEnvelopeUpstreamResponse(clineEnvelopedChatCompletion), writeOpenAIResponsesFallbackError)
	require.NoError(t, err)
	require.Len(t, ccResp.Choices, 1)
	require.Equal(t, "gen_01", ccResp.ID)
	require.Equal(t, 73, usage.InputTokens)
	require.Equal(t, 41, usage.OutputTokens)
	require.NotNil(t, ccResp.Usage)
	require.Equal(t, 73, ccResp.Usage.PromptTokens)
}

func TestReadCCUpstreamJSONResponse_PlainResponseUnchanged(t *testing.T) {
	c, _ := newCCEnvelopeTestContext()
	s := &OpenAIGatewayService{}

	ccResp, usage, err := s.readCCUpstreamJSONResponse(c, newCCEnvelopeUpstreamResponse(plainChatCompletion), writeOpenAIResponsesFallbackError)
	require.NoError(t, err)
	require.Len(t, ccResp.Choices, 1)
	require.Equal(t, "chatcmpl-1", ccResp.ID)
	require.Equal(t, 5, usage.InputTokens)
}

func TestBufferChatCompletionsAsAnthropic_ClineDataEnvelope(t *testing.T) {
	c, rec := newCCEnvelopeTestContext()
	s := &OpenAIGatewayService{}

	_, err := s.bufferChatCompletionsAsAnthropic(c, newCCEnvelopeUpstreamResponse(clineEnvelopedChatCompletion),
		"cline-pass/deepseek-v4.1-flash", "cline-pass/deepseek-v4.1-flash", "cline-pass/deepseek-v4.1-flash", nil, nil, time.Now())
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, rec.Code)

	var got apicompat.AnthropicResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	var text strings.Builder
	for _, block := range got.Content {
		if block.Type == "text" {
			text.WriteString(block.Text)
		}
	}
	require.Equal(t, "<severity>0</severity>", text.String())
	require.Equal(t, 73, got.Usage.InputTokens)
	require.Equal(t, 41, got.Usage.OutputTokens)
	require.NotNil(t, got.StopReason)
	require.Equal(t, "end_turn", *got.StopReason)
}
