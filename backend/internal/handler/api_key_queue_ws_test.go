package handler

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestWSKeyQueueWaitsThenTimesOutWithoutTicketLeak(t *testing.T) {
	helper, rawCache := newAPIKeyAdmissionHelper(t)
	helper.concurrencyService.SetAPIKeyQueuePolicy(service.APIKeyQueuePolicy{MaxWaiting: 1, Timeout: 200 * time.Millisecond})
	queueCache, ok := rawCache.(service.APIKeySlotQueueCache)
	require.True(t, ok)

	holderCtx, cancelHolder := service.WithAPIKeyAdmissionOwner(context.Background())
	defer cancelHolder()
	holder, err := helper.concurrencyService.ReserveAPIKeySlotWithWait(holderCtx, 55, 1)
	require.NoError(t, err)
	require.NotNil(t, holder)
	defer holder.Release()

	ctx, cancel := service.WithAPIKeyAdmissionOwner(context.Background())
	defer cancel()
	started := time.Now()
	release, acquired, err := helper.TryAcquireWSUserSlotForAPIKey(ctx, 202, 3, 55, 1)
	require.False(t, acquired)
	require.Nil(t, release)
	require.True(t, service.IsAPIKeyQueueErrorKind(err, service.APIKeyQueueErrorTimeout))
	require.GreaterOrEqual(t, time.Since(started), 200*time.Millisecond)
	require.Equal(t, coderwsStatusForQueueError(t, err), 1013)

	active, waiting, statsErr := queueCache.GetAPIKeyQueueStats(context.Background(), 55)
	require.NoError(t, statsErr)
	require.Equal(t, 1, active, "holder keeps the only execution slot")
	require.Zero(t, waiting, "timed out waiter must clean its ticket")
}

func TestWSKeyQueueFullRejectsFast(t *testing.T) {
	helper, rawCache := newAPIKeyAdmissionHelper(t)
	helper.concurrencyService.SetAPIKeyQueuePolicy(service.APIKeyQueuePolicy{MaxWaiting: 1, Timeout: 5 * time.Second})
	queueCache, ok := rawCache.(service.APIKeySlotQueueCache)
	require.True(t, ok)

	holderCtx, cancelHolder := service.WithAPIKeyAdmissionOwner(context.Background())
	defer cancelHolder()
	holder, err := helper.concurrencyService.ReserveAPIKeySlotWithWait(holderCtx, 66, 1)
	require.NoError(t, err)
	defer holder.Release()

	waiterCtx, cancelWaiter := service.WithAPIKeyAdmissionOwner(context.Background())
	defer cancelWaiter()
	type result struct {
		release  func()
		acquired bool
		err      error
	}
	done := make(chan result, 1)
	go func() {
		release, acquired, waitErr := helper.TryAcquireWSUserSlotForAPIKey(waiterCtx, 203, 3, 66, 1)
		done <- result{release: release, acquired: acquired, err: waitErr}
	}()
	require.Eventually(t, func() bool {
		_, waiting, statsErr := queueCache.GetAPIKeyQueueStats(context.Background(), 66)
		return statsErr == nil && waiting == 1
	}, 3*time.Second, 10*time.Millisecond)

	started := time.Now()
	release, acquired, err := helper.TryAcquireWSUserSlotForAPIKey(waiterCtx, 204, 3, 66, 1)
	require.False(t, acquired)
	require.Nil(t, release)
	require.True(t, service.IsAPIKeyQueueErrorKind(err, service.APIKeyQueueErrorFull))
	require.Less(t, time.Since(started), time.Second, "queue full must reject immediately")
	require.Equal(t, coderwsStatusForQueueError(t, err), 1013)

	cancelWaiter()
	select {
	case got := <-done:
		require.ErrorIs(t, got.err, context.Canceled)
	case <-time.After(3 * time.Second):
		t.Fatal("queued waiter did not stop after cancellation")
	}
}

func TestHTTPKeyQueueTimeoutKeepsResponseUnwritten(t *testing.T) {
	helper, _ := newAPIKeyAdmissionHelper(t)
	helper.concurrencyService.SetAPIKeyQueuePolicy(service.APIKeyQueuePolicy{MaxWaiting: 1, Timeout: 150 * time.Millisecond})

	holderCtx, cancelHolder := service.WithAPIKeyAdmissionOwner(context.Background())
	defer cancelHolder()
	holder, err := helper.concurrencyService.ReserveAPIKeySlotWithWait(holderCtx, 88, 1)
	require.NoError(t, err)
	defer holder.Release()

	c, recorder := newHelperTestContext(http.MethodPost, "/v1/messages")
	ownerCtx, cancelOwner := service.WithAPIKeyAdmissionOwner(c.Request.Context())
	defer cancelOwner()
	c.Request = c.Request.WithContext(ownerCtx)
	streamStarted := false
	release, err := helper.AcquireUserSlotWithWait(c, 202, 3, 88, 1, true, &streamStarted)
	require.Nil(t, release)
	require.True(t, service.IsAPIKeyQueueErrorKind(err, service.APIKeyQueueErrorTimeout))
	require.False(t, streamStarted, "the key wait must not start an SSE response")
	require.Empty(t, recorder.Body.String(), "the key wait must not write any response body")
}

func coderwsStatusForQueueError(t *testing.T, err error) int {
	t.Helper()
	closeErr := openAIWSUserSlotAcquireError(err)
	require.NotNil(t, closeErr)
	return int(closeErr.StatusCode())
}
