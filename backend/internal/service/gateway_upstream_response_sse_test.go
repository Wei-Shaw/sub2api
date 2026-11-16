package service

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestGatewayUpstreamErrorAfterKeepalive(t *testing.T) {
	gin.SetMode(gin.TestMode)

	const rejected = `{"type":"error","error":{"type":"invalid_request_error","message":"Invalid schema for field messages"}}`
	tests := []struct {
		name       string
		status     int
		body       string
		exhausted  bool
		rule       bool
		wantStatus int
		wantType   string
		wantMsg    string
	}{
		{"bad request", 400, rejected, false, false, 400, "invalid_request_error", "Invalid schema for field messages"},
		{"upstream error type", 400, `{"error":{"type":"request_too_large","message":"Request is too large"}}`, false, false, 400, "request_too_large", "Request is too large"},
		{"detail bad request", 400, `{"detail":"Request rejected"}`, false, false, 400, "invalid_request_error", "Request rejected"},
		{"empty bad request", 400, "", false, false, 400, "invalid_request_error", "Upstream rejected the request"},
		{"HTML bad request", 400, "<html>upstream error</html>", false, false, 400, "invalid_request_error", "Upstream rejected the request"},
		{"sanitized bad request", 400, `{"error":{"message":"invalid endpoint https://example.test/?key=synthetic&access_token=synthetic"}}`, false, false, 400, "invalid_request_error", "invalid endpoint https://example.test/?key=***&access_token=***"},
		{"authentication", 401, rejected, false, false, 502, "upstream_error", "Upstream authentication failed, please contact administrator"},
		{"forbidden", 403, rejected, false, false, 502, "upstream_error", "Upstream access forbidden, please contact administrator"},
		{"rate limit", 429, rejected, false, false, 429, "rate_limit_error", "Upstream rate limit exceeded, please retry later"},
		{"overloaded", 529, rejected, false, false, 503, "overloaded_error", "Upstream service overloaded, please retry later"},
		{"server error", 500, rejected, false, false, 502, "upstream_error", "Upstream service temporarily unavailable"},
		{"default mapping", 422, rejected, false, false, 502, "upstream_error", "Upstream request failed"},
		{"passthrough rule", 422, rejected, false, true, 418, "upstream_error", "Mapped error\nwith a quoted \"message\""},
		{"bad request rule", 400, rejected, false, true, 418, "upstream_error", "Mapped error\nwith a quoted \"message\""},
		{"retries exhausted", 422, rejected, true, false, 502, "upstream_error", "Upstream request failed after retries"},
		{"retries exhausted rule", 422, rejected, true, true, 418, "upstream_error", "Mapped error\nwith a quoted \"message\""},
	}
	states := []struct {
		name        string
		contentType string
		prefix      string
		wantSSE     bool
	}{
		{name: "before headers"},
		{name: "unflushed SSE headers", contentType: "text/event-stream"},
		{name: "comment ping", contentType: "text/event-stream; charset=utf-8", prefix: ": ping\n\n", wantSSE: true},
		{name: "Anthropic queue ping", contentType: "text/event-stream", prefix: "data: {\"type\": \"ping\"}\n\n", wantSSE: true},
		{name: "JSON whitespace", contentType: "application/json", prefix: "\n"},
	}
	for _, tt := range tests {
		for _, state := range states {
			t.Run(tt.name+"/"+state.name, func(t *testing.T) {
				rec := httptest.NewRecorder()
				c, _ := gin.CreateTestContext(rec)
				c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
				if state.contentType != "" {
					c.Header("Content-Type", state.contentType)
				}
				if state.prefix != "" {
					_, err := c.Writer.WriteString(state.prefix)
					require.NoError(t, err)
					c.Writer.Flush()
				}
				if tt.rule {
					rule := newNonFailoverPassthroughRule(tt.status, "Invalid schema", tt.wantStatus, tt.wantMsg)
					rule.SkipMonitoring = true
					rules := &ErrorPassthroughService{}
					rules.setLocalCache([]*model.ErrorPassthroughRule{rule})
					BindErrorPassthroughService(c, rules)
				}
				resp := &http.Response{StatusCode: tt.status, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(tt.body))}
				account := &Account{ID: 1, Platform: PlatformAnthropic, Type: AccountTypeAPIKey}
				svc := &GatewayService{}
				var result *ForwardResult
				var err error
				if tt.exhausted {
					result, err = svc.handleRetryExhaustedError(context.Background(), resp, c, account)
				} else {
					result, err = svc.handleErrorResponse(context.Background(), resp, c, account)
				}
				require.Error(t, err)
				require.Nil(t, result)
				var failoverErr *UpstreamFailoverError
				require.NotErrorAs(t, err, &failoverErr)
				require.True(t, IsResponseCommitted(c), "the outer handler must not append a second error")
				wantHTTPStatus := tt.wantStatus
				if state.prefix != "" {
					wantHTTPStatus = http.StatusOK
				}
				require.Equal(t, wantHTTPStatus, rec.Code)
				if tt.rule {
					require.True(t, c.GetBool(OpsSkipPassthroughKey))
				}
				if state.wantSSE {
					require.Equal(t, state.contentType, rec.Result().Header.Get("Content-Type"))
					require.True(t, rec.Flushed)
					payload := requireGatewayKeepaliveErrorEvent(t, rec.Body.String(), state.prefix)
					require.Equal(t, tt.wantType, payload.Error.Type)
					require.Equal(t, tt.wantMsg, payload.Error.Message)
					streamErr, ok := GetOpsStreamError(c)
					require.True(t, ok, "HTTP 200 stream failures must remain visible to Ops")
					require.Equal(t, tt.wantStatus, streamErr.IntendedStatus)
					require.Equal(t, tt.wantType, streamErr.ErrType)
					require.Equal(t, tt.wantMsg, streamErr.Message)
				} else {
					require.NotContains(t, rec.Body.String(), "event: error")
					require.Empty(t, GetOpsStreamErrors(c))
					if tt.status == 400 && !tt.rule && !tt.exhausted {
						require.Equal(t, state.prefix+tt.body, rec.Body.String(), "unstarted responses retain raw HTTP 400 passthrough")
					} else {
						var payload gatewayKeepaliveErrorPayload
						require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &payload))
						require.Equal(t, "error", payload.Type)
						require.Equal(t, tt.wantType, payload.Error.Type)
						require.Equal(t, tt.wantMsg, payload.Error.Message)
					}
				}
			})
		}
	}
}

type gatewayKeepaliveErrorPayload struct {
	Type  string `json:"type"`
	Error struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"error"`
}

func requireGatewayKeepaliveErrorEvent(t *testing.T, body, prefix string) gatewayKeepaliveErrorPayload {
	t.Helper()
	require.True(t, strings.HasPrefix(body, prefix))
	tail := strings.TrimPrefix(body, prefix)
	scanner := bufio.NewScanner(strings.NewReader(tail))
	var event string
	var data []string
	var events int
	var payload gatewayKeepaliveErrorPayload
	for scanner.Scan() {
		line := scanner.Text()
		switch {
		case strings.HasPrefix(line, "event:"):
			event = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
		case strings.HasPrefix(line, "data:"):
			data = append(data, strings.TrimPrefix(strings.TrimPrefix(line, "data:"), " "))
		case line == "":
			require.Equal(t, "error", event)
			require.NoError(t, json.Unmarshal([]byte(strings.Join(data, "\n")), &payload))
			require.Equal(t, "error", payload.Type)
			events++
			event, data = "", nil
		default:
			t.Fatalf("unexpected non-SSE line after keepalive: %q", line)
		}
	}
	require.NoError(t, scanner.Err())
	require.Equal(t, 1, events, "the stream must end with exactly one complete error event")
	require.Empty(t, event)
	require.Empty(t, data)
	return payload
}

func TestGatewayForwardErrorAfterKeepalive(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, passthrough := range []bool{false, true} {
		for _, started := range []bool{false, true} {
			t.Run(fmt.Sprintf("passthrough=%v/started=%v", passthrough, started), func(t *testing.T) {
				rec := httptest.NewRecorder()
				c, _ := gin.CreateTestContext(rec)
				c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
				const ping = "data: {\"type\": \"ping\"}\n\n"
				if started {
					c.Header("Content-Type", "text/event-stream")
					_, err := c.Writer.WriteString(ping)
					require.NoError(t, err)
					c.Writer.Flush()
				}
				const upstreamBody = `{"type":"error","error":{"type":"invalid_request_error","message":"Invalid schema for field messages"}}`
				upstream := &anthropicHTTPUpstreamRecorder{resp: &http.Response{
					StatusCode: http.StatusBadRequest,
					Header:     http.Header{"Content-Type": []string{"application/json"}},
					Body:       io.NopCloser(strings.NewReader(upstreamBody)),
				}}
				svc := newForwardPartialUsageServiceForTest(upstream)
				account := newAnthropicOAuthAccountForPartialUsageTest()
				if passthrough {
					account = newAnthropicAPIKeyAccountForTest()
				}
				parsed, err := ParseGatewayRequest(NewRequestBodyRef([]byte(`{"model":"claude-3-5-sonnet-latest","stream":true,"messages":[{"role":"user","content":"hello"}]}`)), PlatformAnthropic)
				require.NoError(t, err)
				result, err := svc.Forward(context.Background(), c, account, parsed)
				require.Error(t, err)
				require.Nil(t, result)
				require.NotNil(t, upstream.lastReq, "exercise the real Forward entry point")
				require.True(t, IsResponseCommitted(c))
				if started {
					require.Equal(t, http.StatusOK, rec.Code)
					payload := requireGatewayKeepaliveErrorEvent(t, rec.Body.String(), ping)
					require.Equal(t, "invalid_request_error", payload.Error.Type)
					require.Equal(t, "Invalid schema for field messages", payload.Error.Message)
				} else {
					require.Equal(t, http.StatusBadRequest, rec.Code)
					require.Equal(t, upstreamBody, rec.Body.String())
				}
			})
		}
	}
}
