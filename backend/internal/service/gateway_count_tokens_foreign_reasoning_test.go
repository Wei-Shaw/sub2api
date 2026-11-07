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

func TestForwardCountTokens_NativeAnthropicDropsOpenAIThinkingWithoutMutatingReplay(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, passthrough := range []bool{false, true} {
		name := "normal"
		if passthrough {
			name = "passthrough"
		}
		t.Run(name, func(t *testing.T) {
			body := []byte(`{"model":"claude-3-7-sonnet-20250219","messages":[{"role":"user","content":"start"},{"role":"assistant","content":[{"type":"thinking","thinking":"foreign","signature":"gAAAA-old-openai"},{"type":"thinking","thinking":"native","signature":"CAQS-native"},{"type":"text","text":"kept"},{"type":"tool_use","id":"toolu_1","name":"read","input":{"n":9007199254740993}}]},{"role":"user","content":[{"type":"tool_result","tool_use_id":"toolu_1","content":"ok"}]}],"tools":[{"name":"read","input_schema":{"type":"object","properties":{"n":{"type":"integer"}}}}]}`)
			parsed, err := ParseGatewayRequest(NewRequestBodyRef(body), PlatformAnthropic)
			require.NoError(t, err)
			originalMessages := string(parsed.MessagesRaw())
			accepted := 0
			parsed.OnUpstreamAccepted = func() { accepted++ }
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages/count_tokens", nil)
			upstream := &anthropicHTTPUpstreamRecorder{resp: &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"application/json"}},
				Body:       io.NopCloser(strings.NewReader(`{"input_tokens":17}`)),
			}}
			svc := newForwardPartialUsageServiceForTest(upstream)
			account := newAnthropicAPIKeyAccountForTest()
			account.Extra["anthropic_passthrough"] = passthrough

			require.NoError(t, svc.ForwardCountTokens(context.Background(), c, account, parsed))
			require.Equal(t, http.StatusOK, rec.Code)
			require.JSONEq(t, `{"input_tokens":17}`, rec.Body.String())
			require.NotNil(t, upstream.lastReq, "request must reach only the mocked upstream")
			require.Equal(t, "/v1/messages/count_tokens", upstream.lastReq.URL.Path)
			require.NotContains(t, string(upstream.lastBody), "gAAAA-old-openai")
			require.Len(t, gjson.GetBytes(upstream.lastBody, "messages.1.content").Array(), 3)
			require.Equal(t, "CAQS-native", gjson.GetBytes(upstream.lastBody, "messages.1.content.0.signature").String())
			require.Equal(t, "native", gjson.GetBytes(upstream.lastBody, "messages.1.content.0.thinking").String())
			require.Equal(t, "kept", gjson.GetBytes(upstream.lastBody, "messages.1.content.1.text").String())
			require.Equal(t, "toolu_1", gjson.GetBytes(upstream.lastBody, "messages.1.content.2.id").String())
			require.Equal(t, "read", gjson.GetBytes(upstream.lastBody, "messages.1.content.2.name").String())
			require.Equal(t, "9007199254740993", gjson.GetBytes(upstream.lastBody, "messages.1.content.2.input.n").Raw)
			require.Equal(t, "toolu_1", gjson.GetBytes(upstream.lastBody, "messages.2.content.0.tool_use_id").String())
			require.Equal(t, "ok", gjson.GetBytes(upstream.lastBody, "messages.2.content.0.content").String())
			require.JSONEq(t, gjson.GetBytes(body, "tools").Raw, gjson.GetBytes(upstream.lastBody, "tools").Raw)
			require.Equal(t, body, parsed.Body.Bytes(), "original replay history must survive another route/account attempt")
			require.Equal(t, originalMessages, string(parsed.MessagesRaw()))
			require.Equal(t, "claude-3-7-sonnet-20250219", parsed.Model)
			require.NotNil(t, parsed.OnUpstreamAccepted)
			require.Zero(t, accepted, "count_tokens must not accept a messages turn")
			parsed.OnUpstreamAccepted()
			require.Equal(t, 1, accepted)
		})
	}
}

func TestForwardCountTokens_NativeAnthropicBedrockRemainsUnsupported(t *testing.T) {
	gin.SetMode(gin.TestMode)
	body := []byte(`{"model":"claude-3-7-sonnet-20250219","messages":[{"role":"assistant","content":[{"type":"thinking","thinking":"foreign","signature":"gAAAA-old-openai"}]}]}`)
	parsed, err := ParseGatewayRequest(NewRequestBodyRef(body), PlatformAnthropic)
	require.NoError(t, err)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages/count_tokens", nil)
	account := &Account{Platform: PlatformAnthropic, Type: AccountTypeBedrock}
	// No upstream or credential services: unsupported requests must stop before either.
	require.NoError(t, (&GatewayService{}).ForwardCountTokens(context.Background(), c, account, parsed))
	require.Equal(t, http.StatusNotFound, rec.Code)
	require.Contains(t, rec.Body.String(), "count_tokens endpoint is not supported for Bedrock")
	require.Equal(t, body, parsed.Body.Bytes())
}
