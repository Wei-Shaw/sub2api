package service

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestGatewayCompatPoolMode429AllowsSameAccountRetry(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name string
		path string
		body []byte
		call func(*GatewayService, context.Context, *gin.Context, *Account, []byte) (*ForwardResult, error)
REDACTED{
		{
			name: "chat completions",
			path: "/v1/chat/completions",
			body: []byte(`{"model":"claude-sonnet-4-5","messages":[{"role":"user","content":"hello"REDACTED]REDACTED`),
			call: func(svc *GatewayService, ctx context.Context, c *gin.Context, account *Account, body []byte) (*ForwardResult, error) {
				return svc.ForwardAsChatCompletions(ctx, c, account, body, nil)
		REDACTED,
	REDACTED,
		{
			name: "responses",
			path: "/v1/responses",
			body: []byte(`{"model":"claude-sonnet-4-5","input":"hello"REDACTED`),
			call: func(svc *GatewayService, ctx context.Context, c *gin.Context, account *Account, body []byte) (*ForwardResult, error) {
				return svc.ForwardAsResponses(ctx, c, account, body, nil)
		REDACTED,
	REDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			upstream := &queuedHTTPUpstreamStub{responses: []*http.Response{{
				StatusCode: http.StatusTooManyRequests,
				Header:     http.Header{"X-Request-Id": []string{"pool-429"REDACTEDREDACTED,
				Body:       io.NopCloser(http.NoBody),
		REDACTEDREDACTEDREDACTED
			svc := &GatewayService{
				cfg:                 &config.Config{REDACTED,
				httpUpstream:        upstream,
				tlsFPProfileService: &TLSFingerprintProfileService{REDACTED,
		REDACTED
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(http.MethodPost, tt.path, nil)
			account := &Account{
				ID:       1,
				Name:     "pool-account",
				Platform: PlatformAnthropic,
				Type:     AccountTypeAPIKey,
		REDACTED
					"api_key":   "test-key",
					"pool_mode": true,
			REDACTED,
		REDACTED

			result, err := tt.call(svc, context.Background(), c, account, tt.body)
			require.Nil(t, result)
			var failoverErr *UpstreamFailoverError
			require.ErrorAs(t, err, &failoverErr)
			require.Equal(t, http.StatusTooManyRequests, failoverErr.StatusCode)
			require.True(t, failoverErr.RetryableOnSameAccount)
			require.Equal(t, 1, upstream.callCount)
			require.Empty(t, recorder.Body.String())
	REDACTED)
REDACTED
REDACTED
