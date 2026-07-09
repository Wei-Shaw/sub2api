package handler

import (
	"net/http"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

const testOpenAIShortRefusalCode = "openai_short_refusal"

func newShortRefusalFailoverErr() *service.UpstreamFailoverError {
	return &service.UpstreamFailoverError{
		StatusCode:   http.StatusBadGateway,
		ResponseBody: []byte(`{"error":{"type":"upstream_error","code":"` + testOpenAIShortRefusalCode + `","message":"OpenAI upstream returned a short refusal completion"}}`),
	}
}

func parseEventDataJSON(t *testing.T, body string) string {
	t.Helper()
	lines := strings.Split(strings.TrimSuffix(body, "\n\n"), "\n")
	require.GreaterOrEqual(t, len(lines), 2, "expected SSE event with data line, got %q", body)
	require.True(t, strings.HasPrefix(lines[1], "data: "), "expected data line, got %q", body)
	return strings.TrimPrefix(lines[1], "data: ")
}

func TestOpenAIHandleFailoverExhausted_ShortRefusalJSONIncludesCode(t *testing.T) {
	c, w := newGinContextForEndpoint(t, EndpointChatCompletions)
	h := &OpenAIGatewayHandler{}

	h.handleFailoverExhausted(c, newShortRefusalFailoverErr(), false)

	require.Equal(t, http.StatusBadGateway, w.Code)
	body := w.Body.String()
	assert.Equal(t, "upstream_error", gjson.Get(body, "error.type").String())
	assert.Equal(t, testOpenAIShortRefusalCode, gjson.Get(body, "error.code").String())
	assert.Contains(t, gjson.Get(body, "error.message").String(), "short refusal")
}

func TestOpenAIHandleFailoverExhausted_ShortRefusalChatStreamIncludesCode(t *testing.T) {
	c, w := newGinContextForEndpoint(t, EndpointChatCompletions)
	h := &OpenAIGatewayHandler{}

	h.handleFailoverExhausted(c, newShortRefusalFailoverErr(), true)

	body := w.Body.String()
	require.True(t, strings.HasPrefix(body, "event: error\n"), "got %q", body)
	payload := parseEventDataJSON(t, body)
	assert.Equal(t, "upstream_error", gjson.Get(payload, "error.type").String())
	assert.Equal(t, testOpenAIShortRefusalCode, gjson.Get(payload, "error.code").String())
}

func TestOpenAIHandleFailoverExhausted_ShortRefusalResponsesStreamIncludesCode(t *testing.T) {
	c, w := newGinContextForEndpoint(t, EndpointResponses)
	h := &OpenAIGatewayHandler{}

	h.handleFailoverExhausted(c, newShortRefusalFailoverErr(), true)

	_, errObj := parseResponsesFailedSSE(t, w.Body.String())
	assert.Equal(t, testOpenAIShortRefusalCode, errObj["code"])
}

func TestGatewayHandleFailoverExhausted_ShortRefusalJSONIncludesCode(t *testing.T) {
	c, w := newGinContextForEndpoint(t, EndpointMessages)
	h := &GatewayHandler{}

	h.handleFailoverExhausted(c, newShortRefusalFailoverErr(), service.PlatformOpenAI, false)

	require.Equal(t, http.StatusBadGateway, w.Code)
	body := w.Body.String()
	assert.Equal(t, "error", gjson.Get(body, "type").String())
	assert.Equal(t, "upstream_error", gjson.Get(body, "error.type").String())
	assert.Equal(t, testOpenAIShortRefusalCode, gjson.Get(body, "error.code").String())
}

func TestGatewayHandleFailoverExhausted_ShortRefusalResponsesStreamIncludesCode(t *testing.T) {
	c, w := newGinContextForEndpoint(t, EndpointResponses)
	h := &GatewayHandler{}

	h.handleFailoverExhausted(c, newShortRefusalFailoverErr(), service.PlatformOpenAI, true)

	_, errObj := parseResponsesFailedSSE(t, w.Body.String())
	assert.Equal(t, testOpenAIShortRefusalCode, errObj["code"])
}

func TestGatewayChatCompletionsFailoverExhausted_ShortRefusalJSONIncludesCode(t *testing.T) {
	c, w := newGinContextForEndpoint(t, EndpointChatCompletions)
	h := &GatewayHandler{}

	h.handleCCFailoverExhausted(c, newShortRefusalFailoverErr(), false)

	require.Equal(t, http.StatusBadGateway, w.Code)
	body := w.Body.String()
	assert.Equal(t, "upstream_error", gjson.Get(body, "error.type").String())
	assert.Equal(t, testOpenAIShortRefusalCode, gjson.Get(body, "error.code").String())
}

func TestGatewayResponsesFailoverExhausted_ShortRefusalJSONIncludesCode(t *testing.T) {
	c, w := newGinContextForEndpoint(t, EndpointResponses)
	h := &GatewayHandler{}

	h.handleResponsesFailoverExhausted(c, newShortRefusalFailoverErr(), false)

	require.Equal(t, http.StatusBadGateway, w.Code)
	body := w.Body.String()
	assert.Equal(t, testOpenAIShortRefusalCode, gjson.Get(body, "error.code").String())
	assert.Contains(t, gjson.Get(body, "error.message").String(), "short refusal")
}
