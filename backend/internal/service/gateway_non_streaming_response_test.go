package service

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type nonJSONTempUnschedAccountRepo struct {
	AccountRepository
	tempUnschedCalls int
	tempReason       string
}

func (r *nonJSONTempUnschedAccountRepo) SetTempUnschedulable(_ context.Context, _ int64, _ time.Time, reason string) error {
	r.tempUnschedCalls++
	r.tempReason = reason
	return nil
}

func TestHandleNonStreamingResponse_NonJSON2xxTriggersFailover(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)

	body := []byte("(upstream request failed)")
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header: http.Header{
			"Content-Type": []string{"text/plain"},
			"X-Request-Id": []string{"rid-invalid-json"},
		},
		Body: io.NopCloser(bytes.NewReader(body)),
	}
	svc := &GatewayService{
		cfg:              &config.Config{},
		rateLimitService: &RateLimitService{},
	}

	usage, err := svc.handleNonStreamingResponse(context.Background(), resp, c, &Account{ID: 1}, "claude-sonnet-4-6", "claude-sonnet-4-6")

	require.Nil(t, usage)
	var failoverErr *UpstreamFailoverError
	require.True(t, errors.As(err, &failoverErr))
	require.Equal(t, http.StatusBadGateway, failoverErr.StatusCode)
	require.Equal(t, body, failoverErr.ResponseBody)
	require.Equal(t, "rid-invalid-json", failoverErr.ResponseHeaders.Get("x-request-id"))
	require.False(t, c.Writer.Written(), "invalid upstream response must not be committed before failover")
}

func TestHandleNonStreamingResponse_ValidJSONUnchanged(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)

	body := []byte(`{"id":"msg_1","type":"message","usage":{"input_tokens":12,"output_tokens":7}}`)
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(bytes.NewReader(body)),
	}
	svc := &GatewayService{
		cfg:              &config.Config{},
		rateLimitService: &RateLimitService{},
	}

	usage, err := svc.handleNonStreamingResponse(context.Background(), resp, c, &Account{ID: 1}, "claude-sonnet-4-6", "claude-sonnet-4-6")

	require.NoError(t, err)
	require.NotNil(t, usage)
	require.Equal(t, 12, usage.InputTokens)
	require.Equal(t, 7, usage.OutputTokens)
	require.JSONEq(t, string(body), rec.Body.String())
}

func TestHandleNonStreamingResponseAnthropicAPIKeyPassthrough_NonJSON2xxTriggersFailover(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)

	body := []byte("(upstream request failed)")
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/plain"}},
		Body:       io.NopCloser(bytes.NewReader(body)),
	}
	svc := &GatewayService{cfg: &config.Config{}}

	usage, err := svc.handleNonStreamingResponseAnthropicAPIKeyPassthrough(context.Background(), resp, c, &Account{ID: 2})

	require.Nil(t, usage)
	var failoverErr *UpstreamFailoverError
	require.True(t, errors.As(err, &failoverErr))
	require.Equal(t, http.StatusBadGateway, failoverErr.StatusCode)
	require.Equal(t, body, failoverErr.ResponseBody)
	require.False(t, c.Writer.Written(), "invalid passthrough response must not be committed before failover")
}

func TestHandleNonStreamingResponseAnthropicAPIKeyPassthrough_ValidJSONUnchanged(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)

	body := []byte(`{"id":"msg_1","type":"message","usage":{"input_tokens":5,"output_tokens":3}}`)
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(bytes.NewReader(body)),
	}
	svc := &GatewayService{cfg: &config.Config{}}

	usage, err := svc.handleNonStreamingResponseAnthropicAPIKeyPassthrough(context.Background(), resp, c, &Account{ID: 2})

	require.NoError(t, err)
	require.NotNil(t, usage)
	require.Equal(t, 5, usage.InputTokens)
	require.Equal(t, 3, usage.OutputTokens)
	require.JSONEq(t, string(body), rec.Body.String())
}

func TestHandleNonStreamingResponse_NonJSON2xxMatchesTempUnschedulableRule(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)

	repo := &nonJSONTempUnschedAccountRepo{}
	rateLimitService := NewRateLimitService(repo, nil, &config.Config{}, nil, nil)
	svc := &GatewayService{
		cfg:              &config.Config{},
		rateLimitService: rateLimitService,
	}
	account := &Account{
		ID:       3,
		Platform: PlatformAnthropic,
		Type:     AccountTypeAPIKey,
		Credentials: map[string]any{
			"temp_unschedulable_enabled": true,
			"temp_unschedulable_rules": []any{
				map[string]any{
					"error_code":       float64(http.StatusBadGateway),
					"keywords":         []any{"upstream request failed"},
					"duration_minutes": float64(10),
				},
			},
		},
	}
	body := []byte("(upstream request failed)")
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{},
		Body:       io.NopCloser(bytes.NewReader(body)),
	}

	_, err := svc.handleNonStreamingResponse(context.Background(), resp, c, account, "claude-sonnet-4-6", "claude-sonnet-4-6")

	var failoverErr *UpstreamFailoverError
	require.True(t, errors.As(err, &failoverErr))
	require.Equal(t, http.StatusBadGateway, failoverErr.StatusCode)
	require.Equal(t, body, failoverErr.ResponseBody)
	require.Equal(t, 1, repo.tempUnschedCalls)
	require.Contains(t, repo.tempReason, `"status_code":502`)
	require.Contains(t, repo.tempReason, `"matched_keyword":"upstream request failed"`)
}
