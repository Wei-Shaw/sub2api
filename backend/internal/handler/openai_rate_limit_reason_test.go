package handler

import (
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestOpenAIRateLimitReasonReachesHTTPAndSSETerminals(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, reason := range []string{"quota_exhausted", "concurrency_limited", "rate_limited", "unknown"} {
		for _, stream := range []bool{false, true} {
			for _, route := range []string{"/v1/responses", "/v1/chat/completions"} {
				t.Run(reason+route+map[bool]string{true: "SSE", false: "JSON"}[stream], func(t *testing.T) {
					recorder := httptest.NewRecorder()
					c, _ := gin.CreateTestContext(recorder)
					c.Request = httptest.NewRequest(http.MethodPost, route, nil)
					failure := &service.UpstreamFailoverError{StatusCode: 429, Reason: service.GatewayFailureReason("openai_429_" + reason), ResponseBody: []byte(`{"error":{"message":"do-not-expose-this-detail"}}`)}
					(&OpenAIGatewayHandler{}).handleFailoverExhausted(c, failure, stream)
					require.Equal(t, reason, recorder.Header().Get("X-Sub2API-Rate-Limit-Reason"))
					require.Contains(t, recorder.Body.String(), `"rate_limit_reason":"`+reason+`"`)
					require.NotContains(t, recorder.Body.String(), "do-not-expose-this-detail")
					if stream && strings.HasSuffix(route, "/responses") {
						require.Contains(t, recorder.Body.String(), "event: response.failed")
					}
				})
			}
		}
	}
}

func TestCodexLocalSlotBusyIsNotReportedAsQuota(t *testing.T) {
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	c.Set("openai_rate_limit_reason", "local_slot_busy")
	(&OpenAIGatewayHandler{}).handleStreamingAwareError(c, 429, "CODEX_DEVICE_SESSION_BUSY", "local capacity full", true)
	require.Contains(t, rec.Body.String(), `"rate_limit_reason":"local_slot_busy"`)
	require.NotContains(t, rec.Body.String(), "quota_exhausted")
}
