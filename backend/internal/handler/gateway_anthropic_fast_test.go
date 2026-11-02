package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestAnthropicFastCreditsStopsAccountSwitchAndSurfacesReason(t *testing.T) {
	const message = "Usage credits are required for fast mode."
	failure := &service.UpstreamFailoverError{
		StatusCode:   http.StatusTooManyRequests,
		ResponseBody: []byte(`{"type":"error","error":{"type":"rate_limit_error","message":"Usage credits are required for fast mode."}}`),
		Stage:        service.GatewayFailureStageInference, Scope: service.GatewayFailureScopeRequest,
		Reason:            service.GatewayFailureReason("anthropic_fast_credits_required"),
		NextAccountAction: service.NextAccountStop,
		ClientStatusCode:  http.StatusTooManyRequests, ClientMessage: message,
	}
	fs := NewFailoverState(4, true)
	unscheduler := &mockTempUnscheduler{}
	action := fs.HandleFailoverError(context.Background(), unscheduler, 1, service.PlatformAnthropic, 3, failure)
	require.Equal(t, FailoverExhausted, action)
	require.Zero(t, fs.SwitchCount)
	require.Empty(t, fs.FailedAccountIDs)
	require.Empty(t, fs.SameAccountRetryCount)
	require.Empty(t, unscheduler.calls)
	require.False(t, fs.ForceCacheBilling)

	for _, route := range []string{"messages", "chat", "responses"} {
		for _, streaming := range []bool{false, true} {
			t.Run(route+map[bool]string{false: "/json", true: "/stream"}[streaming], func(t *testing.T) {
				gin.SetMode(gin.TestMode)
				recorder := httptest.NewRecorder()
				c, _ := gin.CreateTestContext(recorder)
				c.Request = httptest.NewRequest(http.MethodPost, "/v1/"+route, nil)
				h := &GatewayHandler{}
				switch route {
				case "messages":
					h.handleFailoverExhausted(c, failure, service.PlatformAnthropic, streaming)
				case "chat":
					h.handleCCFailoverExhausted(c, failure, streaming)
				case "responses":
					h.handleResponsesFailoverExhausted(c, failure, streaming)
				}
				require.Contains(t, recorder.Body.String(), message)
				require.NotContains(t, recorder.Body.String(), "accounts exhausted")
				if !streaming {
					require.Equal(t, http.StatusTooManyRequests, recorder.Code)
				}
			})
		}
	}
}
