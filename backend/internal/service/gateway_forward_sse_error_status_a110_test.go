//go:build unit

package service

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// A1-10: 流内 `event: error` 的语义状态码应按上游 error.type 推导，
// 而不是一律 403（会被映射成 "Upstream access forbidden, please contact administrator"）。
// 账号副作用仍仅限"未输出前的 overloaded_error"。
func TestA110GatewayForward_SSEErrorEventSemanticStatus(t *testing.T) {
	gin.SetMode(gin.TestMode)

	const messageStart = "event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"usage\":{\"input_tokens\":1}}}\n\n"

	cases := []struct {
		name          string
		errType       string
		afterOutput   bool
		wantStatus    int
		wantOverloads int
	}{
		{name: "post_output_overloaded", errType: "overloaded_error", afterOutput: true, wantStatus: 529},
		{name: "post_output_rate_limit", errType: "rate_limit_error", afterOutput: true, wantStatus: http.StatusTooManyRequests},
		{name: "post_output_api_error", errType: "api_error", afterOutput: true, wantStatus: http.StatusInternalServerError},
		{name: "pre_output_api_error", errType: "api_error", wantStatus: http.StatusInternalServerError},
		{name: "pre_output_rate_limit", errType: "rate_limit_error", wantStatus: http.StatusTooManyRequests},
		{name: "pre_output_invalid_request", errType: "invalid_request_error", wantStatus: http.StatusBadRequest},
		{name: "pre_output_authentication", errType: "authentication_error", wantStatus: http.StatusUnauthorized},
		{name: "pre_output_permission", errType: "permission_error", wantStatus: http.StatusForbidden},
		{name: "pre_output_overloaded", errType: "overloaded_error", wantStatus: 529, wantOverloads: 1},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)

			body := []byte(`{"model":"claude-3-5-sonnet-latest","stream":true,"messages":[{"role":"user","content":"hello"}]}`)
			parsed, err := ParseGatewayRequest(NewRequestBodyRef(body), PlatformAnthropic)
			require.NoError(t, err)

			errorJSON := `{"type":"error","error":{"type":"` + tc.errType + `","message":"boom"}}`
			fixture := "event: error\ndata: " + errorJSON + "\n\n"
			if tc.afterOutput {
				fixture = messageStart + fixture
			}
			repo := &gatewayForwardErrorPolicyRepoStub{}
			cfg := &config.Config{Gateway: config.GatewayConfig{MaxLineSize: defaultMaxLineSize}}
			svc := &GatewayService{
				cfg:                  cfg,
				responseHeaderFilter: compileResponseHeaderFilter(cfg),
				httpUpstream: &anthropicHTTPUpstreamRecorder{resp: &http.Response{
					StatusCode: http.StatusOK,
					Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
					Body:       io.NopCloser(strings.NewReader(fixture)),
				}},
				rateLimitService: NewRateLimitService(repo, nil, cfg, nil, nil),
				deferredService:  &DeferredService{},
			}

			result, err := svc.Forward(context.Background(), c, newAnthropicOAuthAccountForPartialUsageTest(), parsed)
			require.Error(t, err)
			require.Nil(t, result)

			var failoverErr *UpstreamFailoverError
			require.ErrorAs(t, err, &failoverErr)
			require.Equal(t, tc.wantStatus, failoverErr.StatusCode)
			require.JSONEq(t, errorJSON, string(failoverErr.ResponseBody))
			require.Equal(t, tc.wantOverloads, repo.overloadCalls)
			require.Zero(t, repo.tempCalls)
			require.Empty(t, repo.modelRateLimitCalls)
			if tc.afterOutput {
				require.Contains(t, rec.Body.String(), "message_start")
			} else {
				require.Empty(t, rec.Body.String())
			}
		})
	}
}
