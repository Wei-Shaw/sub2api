//go:build unit

package handler

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestOpenAIGatewayHandlerResponses_StreamQuotaErrorSwitchesAccount(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, stream := range []bool{false, true} {
		t.Run(fmt.Sprintf("stream=%t", stream), func(t *testing.T) {
			quotaEvent := `{"type":"error","error":{"code":"credit_balance_exhausted","type":"insufficient_quota","message":"You have no credits remaining."}}`
			successEvent := `{"type":"response.completed","response":{"id":"resp_healthy","status":"completed","model":"gpt-5.1","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"healthy account"}]}],"usage":{"input_tokens":1,"output_tokens":2}}}`
			sseResponse := func(event string) *http.Response {
				return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"text/event-stream"}}, Body: io.NopCloser(strings.NewReader("data: " + event + "\n\n"))}
			}
			upstream := newAstraProCapturedUpstream(
				&http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"text/event-stream"}}, Body: io.NopCloser(strings.NewReader("event: error\ndata: " + quotaEvent + "\n\n"))},
				sseResponse(successEvent),
				sseResponse(successEvent),
			)
			repo := &grokCredentialHandlerRepo{accounts: []service.Account{
				{ID: 1, Name: "exhausted", Platform: service.PlatformOpenAI, Type: service.AccountTypeAPIKey, Status: service.StatusActive, Schedulable: true, Priority: 0, Credentials: map[string]any{"api_key": "key-exhausted", "pool_mode": true}},
				{ID: 2, Name: "healthy", Platform: service.PlatformOpenAI, Type: service.AccountTypeAPIKey, Status: service.StatusActive, Schedulable: true, Priority: 1, Credentials: map[string]any{"api_key": "key-healthy"}},
			}}
			cfg := &config.Config{RunMode: config.RunModeSimple}
			rateLimits := service.NewRateLimitService(repo, nil, cfg, nil, nil)
			gateway := service.NewOpenAIGatewayService(repo, nil, nil, nil, nil, nil, nil, cfg, nil, nil, nil, rateLimits, nil, upstream, nil, nil, nil, nil, nil, nil, nil, nil)
			billing := service.NewBillingCacheService(nil, nil, nil, nil, nil, nil, cfg, nil)
			t.Cleanup(billing.Stop)
			handler := NewOpenAIGatewayHandler(gateway, service.NewConcurrencyService(nil), billing, service.NewAPIKeyService(nil, nil, nil, nil, nil, nil, cfg), nil, nil, nil, nil, cfg)
			handler.maxAccountSwitches = 3
			c, rec := newOpenAIResponsesFailoverTestContext(t, nil)
			body := fmt.Sprintf(`{"model":"gpt-5.1","stream":%t,"input":"hello"}`, stream)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(body))
			c.Request.Header.Set("Content-Type", "application/json")

			handler.Responses(c)

			_, accountIDs, _ := upstream.snapshot()
			require.Equal(t, []int64{1, 2}, accountIDs, "quota exhaustion must reach the next account in the same request")
			require.Equal(t, http.StatusOK, rec.Code)
			require.Contains(t, rec.Body.String(), "healthy account")
			require.NotContains(t, rec.Body.String(), "credit_balance_exhausted")
			rawEvents, ok := c.Get(service.OpsUpstreamErrorsKey)
			require.True(t, ok)
			events, ok := rawEvents.([]*service.OpsUpstreamErrorEvent)
			require.True(t, ok)
			require.Len(t, events, 1)
			require.Equal(t, "failover", events[0].Kind)
			require.Equal(t, http.StatusPaymentRequired, events[0].UpstreamStatusCode)

			exhausted, err := repo.GetByID(c.Request.Context(), 1)
			require.NoError(t, err)
			require.Equal(t, service.StatusError, exhausted.Status)
			require.False(t, exhausted.Schedulable)
			require.Contains(t, exhausted.ErrorMessage, "quota exhausted")
			// A new request must skip the exhausted account too, not merely exclude
			// it from the first request's failover set.
			next, nextRec := newOpenAIResponsesFailoverTestContext(t, nil)
			next.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(body))
			next.Request.Header.Set("Content-Type", "application/json")
			handler.Responses(next)
			_, accountIDs, _ = upstream.snapshot()
			require.Equal(t, []int64{1, 2, 2}, accountIDs)
			require.Equal(t, http.StatusOK, nextRec.Code)
			require.Contains(t, nextRec.Body.String(), "healthy account")
		})
	}
}
