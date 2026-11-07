//go:build unit

package service

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestMCPToolNames_BridgeResponseRoundTrip(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, bridge := range []string{"chat", "responses"} {
		for _, stream := range []bool{false, true} {
			name := bridge + "/buffered"
			if stream {
				name = bridge + "/stream"
			}
			t.Run(name, func(t *testing.T) {
				svc := &GatewayService{cfg: &config.Config{Gateway: config.GatewayConfig{AnthropicMCPServerName: "helper"}}}
				rw := svc.buildToolNameRewriteFromBody([]byte(`{"tools":[{"name":"read_file"}]}`))
				alias := rw.Forward["read_file"]
				upstream := strings.NewReplacer("codex_app__read_thread", alias, `"123"`, `"`+alias+`"`).Replace(namespaceToolAnthropicStream())
				recorder := httptest.NewRecorder()
				c, _ := gin.CreateTestContext(recorder)
				c.Set(toolNameRewriteKey, rw)
				resp := &http.Response{Body: io.NopCloser(strings.NewReader(upstream))}
				var err error
				if bridge == "chat" {
					if stream {
						_, err = svc.handleCCStreamingFromAnthropic(resp, c, "model", "model", nil, time.Now())
					} else {
						_, err = svc.handleCCBufferedFromAnthropic(resp, c, "model", "model", nil, time.Now())
					}
				} else if stream {
					_, err = svc.handleResponsesStreamingResponse(resp, c, "model", "model", nil, time.Now(), apicompat.ResponsesClientToolMapping{})
				} else {
					_, err = svc.handleResponsesBufferedStreamingResponse(resp, c, "model", "model", nil, time.Now(), apicompat.ResponsesClientToolMapping{})
				}
				require.NoError(t, err)
				body := recorder.Body.String()
				require.Contains(t, body, `"name":"read_file"`)
				require.NotContains(t, body, `"name":"`+alias+`"`)
				require.Contains(t, body, `\"thread_id\":\"`+alias+`\"`, "arguments must remain opaque")
			})
		}
	}
}

func TestMCPToolNames_PersistentNamespaceOnAllRequestPaths(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, route := range []string{"messages", "count_tokens", "bridge_mimicry"} {
		t.Run(route, func(t *testing.T) {
			body := []byte(`{"model":"claude-sonnet-4-6","max_tokens":256,"tools":[{"name":"read_file","input_schema":{"type":"object"}}],"messages":[{"role":"user","content":"hello"}]}`)
			upstreamBody := `{"id":"msg_1","type":"message","role":"assistant","model":"claude-sonnet-4-6","content":[{"type":"text","text":"ok"}],"stop_reason":"end_turn","usage":{"input_tokens":1,"output_tokens":1}}`
			if route == "count_tokens" {
				upstreamBody = `{"input_tokens":1}`
			}
			upstream := &anthropicHTTPUpstreamRecorder{resp: &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"application/json"}},
				Body:       io.NopCloser(strings.NewReader(upstreamBody)),
			}}
			svc := newForwardPartialUsageServiceForTest(upstream)
			svc.cfg.Gateway.AnthropicMCPServerName = "project"
			account := newAnthropicOAuthAccountForPartialUsageTest()
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
			c.Request.Header.Set("User-Agent", "third-party-client/1.0")
			parsed, err := ParseGatewayRequest(NewRequestBodyRef(body), PlatformAnthropic)
			require.NoError(t, err)
			var out []byte
			switch route {
			case "messages":
				_, err = svc.Forward(context.Background(), c, account, parsed)
				out = upstream.lastBody
			case "count_tokens":
				err = svc.ForwardCountTokens(context.Background(), c, account, parsed)
				out = upstream.lastBody
			case "bridge_mimicry":
				out = svc.applyClaudeCodeOAuthMimicryToBody(context.Background(), c, account, body, nil, parsed.Model)
			}
			require.NoError(t, err)
			require.Equal(t, "mcp__project__read_file", gjson.GetBytes(out, "tools.0.name").String())
			if route != "count_tokens" {
				require.Equal(t, "read_file", toolNameRewriteFromContext(c).Reverse["mcp__project__read_file"])
			}
		})
	}
}

func TestMCPToolNames_RestoreBeforeResponsesNamespace(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := &GatewayService{cfg: &config.Config{Gateway: config.GatewayConfig{AnthropicMCPServerName: "helper"}}}
	rw := svc.buildToolNameRewriteFromBody([]byte(`{"tools":[{"name":"codex_app__read_thread"}]}`))
	alias := rw.Forward["codex_app__read_thread"]
	upstream := strings.ReplaceAll(namespaceToolAnthropicStream(), "codex_app__read_thread", alias)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Set(toolNameRewriteKey, rw)
	resp := &http.Response{Body: io.NopCloser(strings.NewReader(upstream))}
	_, err := svc.handleResponsesStreamingResponse(resp, c, "model", "model", nil, time.Now(), namespaceToolMapping())
	require.NoError(t, err)
	require.Contains(t, recorder.Body.String(), `"name":"read_thread"`)
	require.Contains(t, recorder.Body.String(), `"namespace":"codex_app"`)
	require.NotContains(t, recorder.Body.String(), alias)
}
