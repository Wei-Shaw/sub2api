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

func TestGatewayForward_NativeAnthropicDropsOpenAIThinkingWithoutMutatingReplay(t *testing.T) {
	gin.SetMode(gin.TestMode)
	body := []byte(`{"model":"claude-3-7-sonnet-20250219","stream":false,"messages":[{"role":"user","content":"start"},{"role":"assistant","content":[{"type":"thinking","thinking":"foreign","signature":"gAAAA-old-openai"},{"type":"thinking","thinking":"native","signature":"CAQS-native"},{"type":"text","text":"kept"},{"type":"tool_use","id":"toolu_1","name":"read","input":{"n":9007199254740993}}]},{"role":"user","content":[{"type":"tool_result","tool_use_id":"toolu_1","content":"ok"}]}]}`)
	parsed, err := ParseGatewayRequest(NewRequestBodyRef(body), PlatformAnthropic)
	require.NoError(t, err)
	accepted := 0
	parsed.OnUpstreamAccepted = func() { accepted++ }
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	upstream := &anthropicHTTPUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(`{"id":"msg_1","type":"message","role":"assistant","model":"claude-3-7-sonnet-20250219","content":[{"type":"text","text":"done"}],"stop_reason":"end_turn","usage":{"input_tokens":5,"output_tokens":1}}`)),
	}}
	svc := newForwardPartialUsageServiceForTest(upstream)
	account := &Account{
		ID: 101, Platform: PlatformAnthropic, Type: AccountTypeAPIKey, Concurrency: 1,
		Credentials: map[string]any{"api_key": "test-key", "base_url": "https://api.anthropic.com"},
		Extra:       map[string]any{"anthropic_passthrough": true}, Status: StatusActive, Schedulable: true,
	}
	result, err := svc.Forward(context.Background(), c, account, parsed)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.NotContains(t, string(upstream.lastBody), "gAAAA-old-openai")
	require.Equal(t, "CAQS-native", gjson.GetBytes(upstream.lastBody, "messages.1.content.0.signature").String())
	require.Equal(t, "kept", gjson.GetBytes(upstream.lastBody, "messages.1.content.1.text").String())
	require.Equal(t, "toolu_1", gjson.GetBytes(upstream.lastBody, "messages.1.content.2.id").String())
	require.Equal(t, "9007199254740993", gjson.GetBytes(upstream.lastBody, "messages.1.content.2.input.n").Raw)
	require.Equal(t, "toolu_1", gjson.GetBytes(upstream.lastBody, "messages.2.content.0.tool_use_id").String())
	require.Equal(t, body, parsed.Body.Bytes(), "original replay history must survive another route/account attempt")
	require.NotNil(t, parsed.OnUpstreamAccepted, "original replay callback must remain attached")
	parsed.OnUpstreamAccepted()
	require.Equal(t, 1, accepted)
}
