package handler

import (
	"context"
	"fmt"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestOpenAIForwardMayFailoverOnlyAfterNonSemanticWrite(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	before := service.OpenAICompactKeepaliveAdjustedWrittenSize(c)

	_, err := fmt.Fprint(c.Writer, ":\n\n")
	require.NoError(t, err)
	c.Writer.Flush()

	require.True(t, openAIForwardMayFailover(c, before, &service.UpstreamFailoverError{
		SafeToFailoverAfterWrite: true,
	}))
	require.False(t, openAIForwardMayFailover(c, before, &service.UpstreamFailoverError{}))
}

func TestOpenAIFirstOutputFailoverStopsAfterOneAccountSwitch(t *testing.T) {
	failoverErr := &service.UpstreamFailoverError{SafeToFailoverAfterWrite: true}
	count := 0

	require.False(t, openAIFirstOutputFailoverExhausted(failoverErr, &count))
	require.Equal(t, 1, count)
	require.True(t, openAIFirstOutputFailoverExhausted(failoverErr, &count))
	require.Equal(t, 1, count)
}

func TestOpenAIRequestAllowsFailoverReplayStopsCanceledClient(t *testing.T) {
	require.False(t, openAIRequestAllowsFailoverReplay(nil))

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	requestCtx, cancel := context.WithCancel(context.Background())
	c.Request = httptest.NewRequest("POST", "/v1/responses", nil).WithContext(requestCtx)

	require.True(t, openAIRequestAllowsFailoverReplay(c))
	cancel()
	require.False(t, openAIRequestAllowsFailoverReplay(c))
}

func TestOpenAICapacityFailoverAttemptCountsOnlyAfterRetries(t *testing.T) {
	account := &service.Account{Type: service.AccountTypeAPIKey, Credentials: map[string]any{
		"pool_mode": true, "pool_mode_retry_count": float64(3),
	}}
	err := &service.UpstreamFailoverError{StatusCode: 503, RetryableOnSameAccount: true, RequestScopedTransient: true}
	for attempt := 0; attempt < 3; attempt++ {
		require.False(t, shouldReportOpenAIFailoverAttempt(account, err, attempt))
	}
	require.True(t, shouldReportOpenAIFailoverAttempt(account, err, 3))
	err.RetryableOnSameAccount = false
	err.AccountHealthHandled = true
	require.True(t, shouldReportOpenAIFailoverAttempt(account, err, 0), "explicit rules skip the remaining retries")
}
