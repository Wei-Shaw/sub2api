//go:build unit

package service

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func overloadedStreamSSE() string {
	return strings.Join([]string{
		`data: {"type":"response.created","response":{"id":"resp_sf","status":"in_progress"}}`,
		"",
		`data: {"type":"response.failed","response":{"id":"resp_sf","status":"failed","error":{"code":"server_is_overloaded","message":"Our servers are currently overloaded. Please try again later."}}}`,
		"",
	}, "\n")
}

// contentPolicyStreamSSE 命中规则关键词但 shouldFailover 为 false。
func contentPolicyStreamSSE() string {
	return strings.Join([]string{
		`data: {"type":"response.created","response":{"id":"resp_cp","status":"in_progress"}}`,
		"",
		`data: {"type":"response.failed","response":{"id":"resp_cp","status":"failed","error":{"type":"invalid_request_error","code":"content_policy","message":"Our servers are currently overloaded blocked by content_policy"}}}`,
		"",
	}, "\n")
}

func newSkipFailoverStreamService() *OpenAIGatewayService {
	return &OpenAIGatewayService{cfg: &config.Config{Gateway: config.GatewayConfig{
		MaxLineSize: defaultMaxLineSize,
	}}}
}

func newSkipFailoverStreamContext(t *testing.T, skipFailover bool) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	bindSkipFailoverRule(c, "Our servers are currently overloaded", 529, skipFailover)
	return c, rec
}

func runSkipFailoverStream(t *testing.T, c *gin.Context, body string) error {
	t.Helper()
	svc := newSkipFailoverStreamService()
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{},
		Body:       io.NopCloser(strings.NewReader(body)),
	}
	_, err := svc.handleStreamingResponse(
		c.Request.Context(), resp, c,
		&Account{ID: 1, Platform: PlatformOpenAI}, time.Now(), "model", "model",
	)
	return err
}

func TestResponsesStreamDefaultAllowsFailover(t *testing.T) {
	c, rec := newSkipFailoverStreamContext(t, false)

	err := runSkipFailoverStream(t, c, overloadedStreamSSE())

	require.Error(t, err)
	var failoverErr *UpstreamFailoverError
	require.ErrorAs(t, err, &failoverErr, "默认必须换号，不得被规则拦截")
	require.Empty(t, rec.Body.String())
}

func TestResponsesStreamSkipFailoverReturnsImmediately(t *testing.T) {
	c, rec := newSkipFailoverStreamContext(t, true)

	err := runSkipFailoverStream(t, c, overloadedStreamSSE())

	require.Error(t, err)
	var failoverErr *UpstreamFailoverError
	require.False(t, errors.As(err, &failoverErr), "skip_failover=true 不得换号")
	require.Equal(t, 529, rec.Code)
	require.Contains(t, rec.Body.String(), "overloaded")
}

func TestResponsesStreamNonRetryableStillAppliesRule(t *testing.T) {
	c, rec := newSkipFailoverStreamContext(t, false)

	err := runSkipFailoverStream(t, c, contentPolicyStreamSSE())

	require.Error(t, err)
	var failoverErr *UpstreamFailoverError
	require.False(t, errors.As(err, &failoverErr), "content_policy 不可重试")
	require.Equal(t, 529, rec.Code, "规则仍应改写状态码")
}

func TestResponsesStreamNoRuleStillFailsOver(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)

	err := runSkipFailoverStream(t, c, overloadedStreamSSE())

	require.Error(t, err)
	var failoverErr *UpstreamFailoverError
	require.ErrorAs(t, err, &failoverErr)
	require.Empty(t, rec.Body.String())
}
