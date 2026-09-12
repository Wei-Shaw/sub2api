//go:build unit

package service

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func responsesCancelledSSE(streamStarted bool) string {
	frames := []string{}
	if streamStarted {
		frames = append(frames,
			`data: {"type":"response.created","response":{"id":"resp_cancelled","object":"response","model":"gpt-5.6-sol","status":"in_progress","output":[]}}`,
			"",
			`data: {"type":"response.output_text.delta","output_index":0,"content_index":0,"delta":"partial"}`,
			"",
		)
	}
	frames = append(frames,
		`data: {"type":"response.cancelled","response":{"id":"resp_cancelled","object":"response","model":"gpt-5.6-sol","status":"cancelled","output":[],"usage":{"input_tokens":8,"output_tokens":1,"total_tokens":9}}}`,
		"",
		`data: [DONE]`,
		"",
	)
	return strings.Join(frames, "\n")
}

func runCancelledMessagesFixture(t *testing.T, stream bool) (*httptest.ResponseRecorder, error) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	body := []byte(`{"model":"gpt-5.6-sol","max_tokens":32,"messages":[{"role":"user","content":"hello"}],"stream":` + map[bool]string{true: "true", false: "false"}[stream] + `}`)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body:       io.NopCloser(strings.NewReader(responsesCancelledSSE(stream))),
	}}
	svc := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig(), httpUpstream: upstream}
	account := rawChatCompletionsTestAccount()
	_, err := svc.ForwardAsAnthropic(context.Background(), c, account, body, "", "")
	return rec, err
}

func TestForwardAsAnthropic_BufferedCancelledIsVisibleError(t *testing.T) {
	rec, err := runCancelledMessagesFixture(t, false)
	require.Error(t, err)
	require.Contains(t, err.Error(), "cancelled")
	require.Equal(t, http.StatusBadGateway, rec.Code)
	require.Contains(t, rec.Body.String(), `"type":"error"`)
	require.NotContains(t, rec.Body.String(), `"type":"message"`)
}

func TestForwardAsAnthropic_StreamingCancelledAfterOutputIsVisibleError(t *testing.T) {
	rec, err := runCancelledMessagesFixture(t, true)
	require.Error(t, err)
	require.Contains(t, err.Error(), "cancelled")
	require.Contains(t, rec.Body.String(), `"text":"partial"`)
	require.Contains(t, rec.Body.String(), "event: error")
	require.NotContains(t, rec.Body.String(), "event: message_stop")
}

func TestOpenAICompatResponsesTerminalEventIncludesCancellation(t *testing.T) {
	require.True(t, isOpenAICompatResponsesTerminalEvent("response.cancelled"))
	require.True(t, isOpenAICompatResponsesTerminalEvent("response.canceled"))
}
