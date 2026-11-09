package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func newPoolRecoveryTestContext(t *testing.T) (*gin.Context, context.CancelFunc) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx, cancel := context.WithCancel(context.Background())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil).WithContext(ctx)
	return c, cancel
}

func TestOpenAISelectionRecovery_FirstFailureRetriesOnceWithoutExclusions(t *testing.T) {
	c, cancel := newPoolRecoveryTestContext(t)
	defer cancel()
	var r openAISelectionRecovery

	// 首次选号失败（未换过号）：只做一次忽略模型冷却的重选，随后按原逻辑报错。
	require.True(t, r.next(c, map[int64]struct{}{}, nil, nil))
	require.True(t, r.modelCooldownBypassed)
	require.False(t, r.next(c, map[int64]struct{}{}, nil, nil))
	require.Zero(t, r.poolRounds)
}

func TestOpenAISelectionRecovery_TransientFailoverRetriesFailedAccounts(t *testing.T) {
	c, cancel := newPoolRecoveryTestContext(t)
	defer cancel()
	r := openAISelectionRecovery{modelCooldownBypassed: true}
	failed := map[int64]struct{}{10: {}}
	lastErr := &service.UpstreamFailoverError{StatusCode: http.StatusBadGateway, SameAccountRetryDelay: time.Millisecond}

	for round := 1; round <= maxOpenAIPoolRecoveryRounds; round++ {
		failed[10] = struct{}{}
		require.True(t, r.next(c, failed, lastErr, nil), "round %d", round)
		require.Empty(t, failed, "排除集合应在恢复轮清空，让已失败账号重新参与选号")
	}
	failed[10] = struct{}{}
	require.False(t, r.next(c, failed, lastErr, nil), "恢复轮次必须有界")
}

func TestOpenAISelectionRecovery_NonTransientErrorsAreNotRetried(t *testing.T) {
	c, cancel := newPoolRecoveryTestContext(t)
	defer cancel()

	for _, lastErr := range []*service.UpstreamFailoverError{
		nil,
		{StatusCode: http.StatusTooManyRequests},
		{StatusCode: http.StatusBadRequest},
		{StatusCode: http.StatusUnauthorized},
		{StatusCode: http.StatusBadGateway, Stage: service.GatewayFailureStageAccountAuth},
	} {
		r := openAISelectionRecovery{modelCooldownBypassed: true}
		failed := map[int64]struct{}{10: {}}
		require.False(t, r.next(c, failed, lastErr, nil))
		require.Len(t, failed, 1)
	}
}

func TestOpenAISelectionRecovery_RequestScopedCapacityShedIsRetried(t *testing.T) {
	c, cancel := newPoolRecoveryTestContext(t)
	defer cancel()
	r := openAISelectionRecovery{modelCooldownBypassed: true}
	failed := map[int64]struct{}{3: {}, 7: {}}
	// 流内 server_is_overloaded 以 HTTP 200 承载，靠 RequestScopedTransient 识别。
	lastErr := &service.UpstreamFailoverError{StatusCode: http.StatusOK, RequestScopedTransient: true, SameAccountRetryDelay: time.Millisecond}

	require.True(t, r.next(c, failed, lastErr, nil))
	require.Empty(t, failed)
}

func TestOpenAISelectionRecovery_StopsWhenClientCanceled(t *testing.T) {
	c, cancel := newPoolRecoveryTestContext(t)
	r := openAISelectionRecovery{modelCooldownBypassed: true}
	failed := map[int64]struct{}{10: {}}
	cancel()

	require.False(t, r.next(c, failed, &service.UpstreamFailoverError{StatusCode: http.StatusServiceUnavailable}, nil))
	require.Len(t, failed, 1)
}

func TestOpenAIPoolRecoveryDelay(t *testing.T) {
	require.Equal(t, 6*time.Second, openAIPoolRecoveryDelay(&service.UpstreamFailoverError{SameAccountRetryDelay: 6 * time.Second}, 1))
	require.Equal(t, maxRequestScopedRetryDelay, openAIPoolRecoveryDelay(&service.UpstreamFailoverError{SameAccountRetryDelay: time.Minute}, 1))
	require.Equal(t, openAIPoolRecoveryBaseDelay, openAIPoolRecoveryDelay(&service.UpstreamFailoverError{}, 1))
	require.Equal(t, 2*openAIPoolRecoveryBaseDelay, openAIPoolRecoveryDelay(nil, 2))
	require.Equal(t, maxRequestScopedRetryDelay, openAIPoolRecoveryDelay(nil, 10))
}
