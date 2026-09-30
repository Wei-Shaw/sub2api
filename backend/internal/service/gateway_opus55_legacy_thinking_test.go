package service

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

func TestOpus55LegacyThinkingPreservesRequest(t *testing.T) {
	body := []byte(`{"model":"public-opus","max_tokens":1152,"stream":true,"thinking":{"type":"enabled","budget_tokens":1024,"display":"omitted"},"output_config":{"effort":"xhigh","format":{"type":"json_schema","schema":{"type":"object"}}},"messages":[{"role":"assistant","content":[{"type":"thinking","thinking":"","signature":"signed-history"},{"type":"redacted_thinking","data":"encrypted"},{"type":"tool_use","id":"toolu_1","name":"lookup","input":{}}]},{"role":"user","content":[{"type":"tool_result","tool_use_id":"toolu_1","content":"ok"}]}],"system":"keep","metadata":{"user_id":"original"},"tools":[{"name":"lookup","input_schema":{"type":"object"}}]}`)
	before := append([]byte(nil), body...)
	out, applied := NormalizeClaudeOpus55Thinking(body, "claude-opus-5-5")
	require.True(t, applied)
	require.Equal(t, before, body, "do not mutate shared input bytes")
	for _, field := range []string{"model", "max_tokens", "stream", "messages", "system", "metadata", "tools", "thinking.display", "output_config"} {
		require.Equal(t, gjson.GetBytes(before, field).Raw, gjson.GetBytes(out, field).Raw, field)
	}
	require.NoError(t, validateClaude55Request(out, "claude-opus-5-5"))
	again, applied := NormalizeClaudeOpus55Thinking(out, "claude-opus-5-5")
	require.False(t, applied)
	require.Equal(t, out, again)
}

func TestOpus55LegacyThinkingNoOp(t *testing.T) {
	for _, body := range []string{
		`{invalid`,
		`{"thinking":{"type":"enabled"}`, // incomplete JSON
		`{"thinking":{"type":"adaptive","budget_tokens":1024}}`,
		`{"thinking":{"type":"disabled"}}`,
		`{"thinking":{"type":"between_tools"}}`,
		`{"thinking":null}`, `{}`, `[]`,
		`{"thinking":{"type":"enabled"},"output_config":null}`,
		`{"thinking":{"type":"enabled"},"output_config":"invalid"}`,
		`{"thinking":{"type":"enabled"},"output_config":[]}`,
	} {
		out, applied := NormalizeClaudeOpus55Thinking([]byte(body), "claude-opus-5-5")
		require.False(t, applied, body)
		require.Equal(t, body, string(out))
	}
	for _, model := range []string{"claude-opus-5", "claude-opus-5-5-preview", "claude-sonnet-5-5", "claude-opus-4-6", "unknown"} {
		body := []byte(`{"thinking":{"type":"enabled","budget_tokens":1024}}`)
		out, applied := NormalizeClaudeOpus55Thinking(body, model)
		require.False(t, applied, model)
		require.Equal(t, body, out)
	}
}

func TestOpus55LegacyThinkingExplicitEffort(t *testing.T) {
	// Even invalid explicit values must not silently become medium.
	for _, effort := range []string{`"low"`, `"medium"`, `"high"`, `"xhigh"`, `"max"`, `"invalid"`, `""`, `null`, `12`} {
		body := []byte(`{"thinking":{"type":"enabled"},"output_config":{"effort":` + effort + `}}`)
		out, applied := NormalizeClaudeOpus55Thinking(body, "claude-opus-5-5")
		require.True(t, applied)
		require.Equal(t, effort, gjson.GetBytes(out, "output_config.effort").Raw)
	}
}

func TestOpus55LegacyThinkingGateway(t *testing.T) {
	// Use the real forwarders and an in-memory transport. No credentials or
	// external inference are needed to inspect the actual outgoing JSON.
	for _, route := range []string{"apikey", "passthrough", "oauth"} {
		for _, mode := range []string{"sync", "stream", "count_tokens"} {
			for _, enabled := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/%s/enabled=%t", route, mode, enabled), func(t *testing.T) {
					body := []byte(`{"model":"public-opus","max_tokens":1152,"thinking":{"type":"enabled","budget_tokens":1024},"messages":[{"role":"user","content":"hello"}]}`)
					if route == "oauth" {
						body = []byte(strings.ReplaceAll(string(body), "public-opus", "claude-opus-5-5"))
					}
					body, err := sjson.SetBytes(body, "stream", mode == "stream")
					require.NoError(t, err)
					original, err := ParseGatewayRequest(NewRequestBodyRef(body), PlatformAnthropic)
					require.NoError(t, err)
					parsed, err := original.CloneForBody(body)
					require.NoError(t, err)
					response := `{"id":"msg_test","type":"message","role":"assistant","model":"claude-opus-5-5","content":[{"type":"text","text":"OK"}],"stop_reason":"end_turn","usage":{"input_tokens":4,"output_tokens":1}}`
					contentType := "application/json"
					path := "/v1/messages"
					if mode == "count_tokens" {
						path += "/count_tokens"
						response = `{"input_tokens":42}`
					}
					if mode == "stream" {
						contentType = "text/event-stream"
						response = "event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"id\":\"msg_test\",\"model\":\"claude-opus-5-5\",\"usage\":{\"input_tokens\":4,\"output_tokens\":0}}}\n\nevent: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\"OK\"}}\n\nevent: message_delta\ndata: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"end_turn\"},\"usage\":{\"output_tokens\":1}}\n\nevent: message_stop\ndata: {\"type\":\"message_stop\"}\n\n"
					}
					upstream := &anthropicHTTPUpstreamRecorder{resp: &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{contentType}}, Body: io.NopCloser(strings.NewReader(response))}}
					cfg := &config.Config{Gateway: config.GatewayConfig{ClaudeOpus55LegacyThinkingEnabled: enabled, MaxLineSize: defaultMaxLineSize}}
					svc := &GatewayService{cfg: cfg, httpUpstream: upstream, responseHeaderFilter: compileResponseHeaderFilter(cfg), rateLimitService: &RateLimitService{}}
					account := newAnthropicAPIKeyAccountForTest()
					account.Extra["anthropic_passthrough"] = route == "passthrough"
					account.Credentials["model_mapping"] = map[string]any{"public-opus": "claude-opus-5-5"}
					if route == "oauth" {
						account.Type = AccountTypeOAuth
						account.Credentials = map[string]any{"access_token": "test-oauth-token"}
					}
					rec := httptest.NewRecorder()
					c, _ := gin.CreateTestContext(rec)
					c.Request = httptest.NewRequest(http.MethodPost, path, nil)
					if mode == "count_tokens" {
						err = svc.ForwardCountTokens(context.Background(), c, account, parsed)
					} else {
						_, err = svc.Forward(context.Background(), c, account, parsed)
					}
					require.Equal(t, body, original.Body.Bytes(), "another account attempt must still see the original request")
					if !enabled {
						require.ErrorContains(t, err, "requires adaptive thinking")
						require.Equal(t, 400, rec.Code)
						require.Nil(t, upstream.lastReq)
						return
					}
					require.NoError(t, err)
					require.Equal(t, 200, rec.Code)
					require.NotNil(t, upstream.lastReq)
					require.Equal(t, path, upstream.lastReq.URL.Path)
					require.Equal(t, "claude-opus-5-5", gjson.GetBytes(upstream.lastBody, "model").String())
					require.Equal(t, "adaptive", gjson.GetBytes(upstream.lastBody, "thinking.type").String())
					require.False(t, gjson.GetBytes(upstream.lastBody, "thinking.budget_tokens").Exists())
					require.Equal(t, "medium", gjson.GetBytes(upstream.lastBody, "output_config.effort").String())
					if mode != "count_tokens" {
						require.Equal(t, int64(1152), gjson.GetBytes(upstream.lastBody, "max_tokens").Int())
					}
					if mode == "count_tokens" {
						require.JSONEq(t, `{"input_tokens":42}`, rec.Body.String())
					} else {
						require.Contains(t, rec.Body.String(), "OK")
					}
				})
			}
		}
	}
}

func TestOpus55LegacyThinkingRetainsValidation(t *testing.T) {
	for _, count := range []bool{false, true} {
		for _, field := range []string{`"thinking":{"type":"disabled"}`, `"thinking":{"type":"enabled","budget_tokens":1024},"tool_choice":{"type":"any"}`} {
			body := []byte(`{"model":"claude-opus-5-5","messages":[],` + field + `}`)
			parsed, err := ParseGatewayRequest(NewRequestBodyRef(body), PlatformAnthropic)
			require.NoError(t, err)
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
			svc := &GatewayService{cfg: &config.Config{Gateway: config.GatewayConfig{ClaudeOpus55LegacyThinkingEnabled: true}}}
			account := newAnthropicAPIKeyAccountForTest()
			if count {
				err = svc.ForwardCountTokens(context.Background(), c, account, parsed)
			} else {
				_, err = svc.Forward(context.Background(), c, account, parsed)
			}
			require.Error(t, err)
			require.Equal(t, 400, rec.Code)
		}
	}
}
