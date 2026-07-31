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
	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

type anthropicThinkingCompatUpstream struct {
	responses []*http.Response
	bodies    [][]byte
}

func (u *anthropicThinkingCompatUpstream) Do(req *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
	if req != nil && req.Body != nil {
		body, err := io.ReadAll(req.Body)
		if err != nil {
			return nil, err
		}
		_ = req.Body.Close()
		u.bodies = append(u.bodies, append([]byte(nil), body...))
		req.Body = io.NopCloser(bytes.NewReader(body))
	}
	if len(u.responses) == 0 {
		return nil, io.ErrUnexpectedEOF
	}
	response := u.responses[0]
	u.responses = u.responses[1:]
	return response, nil
}

func (u *anthropicThinkingCompatUpstream) DoWithTLS(req *http.Request, proxyURL string, accountID int64, accountConcurrency int, profile *tlsfingerprint.Profile) (*http.Response, error) {
	return u.Do(req, proxyURL, accountID, accountConcurrency)
}
func TestGatewayService_AnthropicAPIKeyPassthrough_DoesNotCacheRejectedThinkingFallback(t *testing.T) {
	upstream := &anthropicThinkingCompatUpstream{
		responses: []*http.Response{
			{
				StatusCode: http.StatusBadRequest,
				Header:     http.Header{"Content-Type": []string{"application/json"}},
				Body:       newJSONReader(`{"error":{"type":"invalid_request_error","message":"\"thinking.type.enabled\" is not supported for this model. Use \"thinking.type.adaptive\"."}}`),
			},
			{
				StatusCode: http.StatusBadRequest,
				Header:     http.Header{"Content-Type": []string{"application/json"}},
				Body:       newJSONReader(`{"error":{"type":"invalid_request_error","message":"messages.0.content is invalid"}}`),
			},
		},
	}
	cfg := &config.Config{Gateway: config.GatewayConfig{MaxLineSize: defaultMaxLineSize}}
	svc := &GatewayService{
		cfg:                  cfg,
		responseHeaderFilter: compileResponseHeaderFilter(cfg),
		httpUpstream:         upstream,
		rateLimitService:     &RateLimitService{},
	}
	account := newAnthropicAPIKeyAccountForTest()
	body := []byte(`{"model":"claude-fable-5","max_tokens":16000,"thinking":{"type":"enabled","budget_tokens":10000},"messages":[{"role":"user","content":"hello"}]}`)

	first := forwardAnthropicThinkingCompatTestRequest(t, svc, account, body)
	require.Error(t, first.err)
	require.Len(t, upstream.bodies, 2)
	require.Equal(t, "adaptive", gjson.GetBytes(upstream.bodies[1], "thinking.type").String())

	upstream.responses = append(upstream.responses, &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       newJSONReader(`{"id":"msg_3","type":"message","role":"assistant","model":"claude-fable-5","content":[],"stop_reason":"end_turn","usage":{"input_tokens":1,"output_tokens":1}}`),
	})
	second := forwardAnthropicThinkingCompatTestRequest(t, svc, account, body)
	require.NoError(t, second.err)
	require.Len(t, upstream.bodies, 3)
	require.Equal(t, "enabled", gjson.GetBytes(upstream.bodies[2], "thinking.type").String())
}

func TestGatewayService_AnthropicAPIKeyPassthrough_LearnsThinkingCompatibility(t *testing.T) {
	upstream := &anthropicThinkingCompatUpstream{
		responses: []*http.Response{
			{
				StatusCode: http.StatusBadRequest,
				Header:     http.Header{"Content-Type": []string{"application/json"}},
				Body:       io.NopCloser(http.NoBody),
			},
			{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"application/json"}},
				Body:       io.NopCloser(newJSONReader(`{"id":"msg_1","type":"message","role":"assistant","model":"claude-fable-5","content":[],"stop_reason":"end_turn","usage":{"input_tokens":1,"output_tokens":1}}`)),
			},
		},
	}
	// Replace the first response body after construction so the error response is
	// explicit while keeping the response sequence easy to read above.
	upstream.responses[0].Body = newJSONReader(`{"error":{"type":"invalid_request_error","message":"\"thinking.type.enabled\" is not supported for this model. Use \"thinking.type.adaptive\" and \"output_config.effort\"."}}`)

	cfg := &config.Config{Gateway: config.GatewayConfig{MaxLineSize: defaultMaxLineSize}}
	svc := &GatewayService{
		cfg:                  cfg,
		responseHeaderFilter: compileResponseHeaderFilter(cfg),
		httpUpstream:         upstream,
		rateLimitService:     &RateLimitService{},
	}
	account := newAnthropicAPIKeyAccountForTest()
	body := []byte(`{"model":"claude-fable-5","max_tokens":16000,"thinking":{"type":"enabled","budget_tokens":10000},"messages":[{"role":"user","content":"hello"}]}`)

	result := forwardAnthropicThinkingCompatTestRequest(t, svc, account, body)
	require.NoError(t, result.err)
	require.NotNil(t, result.response)
	require.Len(t, upstream.bodies, 2)
	require.Equal(t, "enabled", gjson.GetBytes(upstream.bodies[0], "thinking.type").String())
	require.Equal(t, int64(10000), gjson.GetBytes(upstream.bodies[0], "thinking.budget_tokens").Int())
	require.Equal(t, "adaptive", gjson.GetBytes(upstream.bodies[1], "thinking.type").String())
	require.False(t, gjson.GetBytes(upstream.bodies[1], "thinking.budget_tokens").Exists())

	// The learned contract is keyed by account + upstream base URL + model. A
	// subsequent request is normalized before the first upstream call.
	upstream.responses = append(upstream.responses, &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       newJSONReader(`{"id":"msg_2","type":"message","role":"assistant","model":"claude-fable-5","content":[],"stop_reason":"end_turn","usage":{"input_tokens":1,"output_tokens":1}}`),
	})
	result = forwardAnthropicThinkingCompatTestRequest(t, svc, account, body)
	require.NoError(t, result.err)
	require.Len(t, upstream.bodies, 3)
	require.Equal(t, "adaptive", gjson.GetBytes(upstream.bodies[2], "thinking.type").String())
	require.False(t, gjson.GetBytes(upstream.bodies[2], "thinking.budget_tokens").Exists())
}

type anthropicThinkingCompatTestResult struct {
	response *ForwardResult
	err      error
}

func forwardAnthropicThinkingCompatTestRequest(t *testing.T, svc *GatewayService, account *Account, body []byte) anthropicThinkingCompatTestResult {
	t.Helper()
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	c.Request.Header.Set("User-Agent", "claude-cli/1.0.0")
	parsed := &ParsedRequest{Body: NewRequestBodyRef(body), Model: "claude-fable-5"}
	response, err := svc.Forward(context.Background(), c, account, parsed)
	return anthropicThinkingCompatTestResult{response: response, err: err}
}

func newJSONReader(payload string) io.ReadCloser {
	return io.NopCloser(strings.NewReader(payload))
}
