package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/Wei-Shaw/sub2api/internal/testutil"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func newYieldTestGinContext(t *testing.T) *gin.Context {
	t.Helper()
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	return c
}

func TestAcquireAccountSlotWithWaitTimeoutForClass_NewSessionYieldsWhileContinuationWaits(t *testing.T) {
	cache := testutil.NewTestConcurrencyCache(t)
	helper := NewConcurrencyHelper(service.NewConcurrencyService(cache), "", 0)
	ctx := context.Background()
	ok, err := cache.AcquireAccountSlot(ctx, 901, 1, "other")
	require.NoError(t, err)
	require.True(t, ok)
	ok, err = cache.IncrementAccountContinuationWaitCount(ctx, 901, 3)
	require.NoError(t, err)
	require.True(t, ok)
	go func() {
		time.Sleep(150 * time.Millisecond)
		_ = cache.ReleaseAccountSlot(ctx, 901, "other")
	}()

	streamStarted := false
	release, err := helper.AcquireAccountSlotWithWaitTimeoutForClass(newYieldTestGinContext(t), 901, 1, 700*time.Millisecond, false, &streamStarted, service.AccountWaitClassNewSession, 0)
	require.Nil(t, release)
	var concurrencyErr *ConcurrencyError
	require.ErrorAs(t, err, &concurrencyErr)
	require.True(t, concurrencyErr.IsTimeout, "有续聊等待者时新会话等到超时也不拿槽")
	n, err := cache.GetAccountConcurrency(ctx, 901)
	require.NoError(t, err)
	require.Equal(t, 0, n)

	require.NoError(t, cache.DecrementAccountContinuationWaitCount(ctx, 901))
	release, err = helper.AcquireAccountSlotWithWaitTimeoutForClass(newYieldTestGinContext(t), 901, 1, 700*time.Millisecond, false, &streamStarted, service.AccountWaitClassNewSession, 0)
	require.NoError(t, err)
	require.NotNil(t, release)
	release()
}

func TestAcquireAccountSlotWithWaitTimeout_LegacyCallersDoNotYield(t *testing.T) {
	cache := testutil.NewTestConcurrencyCache(t)
	helper := NewConcurrencyHelper(service.NewConcurrencyService(cache), "", 0)
	ctx := context.Background()
	ok, err := cache.IncrementAccountContinuationWaitCount(ctx, 902, 3)
	require.NoError(t, err)
	require.True(t, ok)
	streamStarted := false
	release, err := helper.AcquireAccountSlotWithWaitTimeout(newYieldTestGinContext(t), 902, 1, 500*time.Millisecond, false, &streamStarted)
	require.NoError(t, err, "旧签名调用点（Anthropic、Gemini 等）行为不变")
	require.NotNil(t, release)
	release()
}

func TestTryAcquireAccountSlotForPlan_NewSessionYieldsWhenSlotFree(t *testing.T) {
	cache := testutil.NewTestConcurrencyCache(t)
	helper := NewConcurrencyHelper(service.NewConcurrencyService(cache), "", 0)
	ctx := context.Background()
	ok, err := cache.IncrementAccountContinuationWaitCount(ctx, 904, 3)
	require.NoError(t, err)
	require.True(t, ok)

	result, err := helper.TryAcquireAccountSlotForPlan(ctx, 904, 1, service.AccountWaitClassNewSession, 0)
	require.NoError(t, err)
	require.False(t, result.Acquired, "账号有空槽但有续聊等待者，新会话入口不快抢")
	n, err := cache.GetAccountConcurrency(ctx, 904)
	require.NoError(t, err)
	require.Equal(t, 0, n)

	for _, class := range []service.AccountWaitClass{service.AccountWaitClassContinuation, service.AccountWaitClassLegacy} {
		result, err = helper.TryAcquireAccountSlotForPlan(ctx, 904, 1, class, 0)
		require.NoError(t, err)
		require.True(t, result.Acquired, class.String())
		result.ReleaseFunc()
	}

	require.NoError(t, cache.DecrementAccountContinuationWaitCount(ctx, 904))
	result, err = helper.TryAcquireAccountSlotForPlan(ctx, 904, 1, service.AccountWaitClassNewSession, 0)
	require.NoError(t, err)
	require.True(t, result.Acquired, "续聊清空后新会话恢复快抢")
	result.ReleaseFunc()
}

func TestEnterAccountWaitQueue_ChoosesCounterByClass(t *testing.T) {
	cache := testutil.NewTestConcurrencyCache(t)
	h := &OpenAIGatewayHandler{concurrencyHelper: NewConcurrencyHelper(service.NewConcurrencyService(cache), "", 0)}
	ctx := context.Background()

	canWait, leave, err := h.enterAccountWaitQueue(ctx, 903, &service.AccountWaitPlan{MaxWaiting: 3, Class: service.AccountWaitClassContinuation})
	require.NoError(t, err)
	require.True(t, canWait)
	cont, _ := cache.GetAccountContinuationWaitingCount(ctx, 903)
	legacy, _ := cache.GetAccountWaitingCount(ctx, 903)
	require.Equal(t, 1, cont)
	require.Equal(t, 0, legacy)
	leave()

	for _, class := range []service.AccountWaitClass{service.AccountWaitClassLegacy, service.AccountWaitClassNewSession} {
		canWait, leave, err = h.enterAccountWaitQueue(ctx, 903, &service.AccountWaitPlan{MaxWaiting: 100, Class: class})
		require.NoError(t, err)
		require.True(t, canWait)
		cont, _ = cache.GetAccountContinuationWaitingCount(ctx, 903)
		legacy, _ = cache.GetAccountWaitingCount(ctx, 903)
		require.Equal(t, 0, cont)
		require.Equal(t, 1, legacy, class.String())
		leave()
	}
}
