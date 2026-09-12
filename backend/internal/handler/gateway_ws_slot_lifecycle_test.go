package handler

import (
	"context"
	"testing"
	"testing/synctest"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestWSAPIKeySlotCancellationWaitsForExplicitTurnCleanup(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		cache := &helperConcurrencyCacheStub{userSeq: []bool{true}}
		helper := NewConcurrencyHelper(service.NewConcurrencyService(cache), SSEPingFormatNone, time.Second)
		ctx, cancel := context.WithCancel(context.Background())
		release, acquired, err := helper.TryAcquireWSUserSlotForAPIKey(ctx, 202, 3, 77, 0)
		require.NoError(t, err)
		require.True(t, acquired)
		defer release()

		closeStarted := make(chan struct{})
		finishClose := make(chan struct{})
		turnDone := make(chan struct{})
		go func() {
			<-ctx.Done()
			close(closeStarted)
			<-finishClose
			release() // AfterTurn, after upstream Close/join completes.
			close(turnDone)
		}()
		cancel()
		<-closeStarted
		synctest.Wait() // Let the slot worker observe all runnable cancellation work.
		require.Zero(t, cache.apiKeyReleaseCalls)
		require.Zero(t, cache.userReleaseCalls)
		close(finishClose)
		<-turnDone
		release() // Handler defer is also safe after AfterTurn.
		require.Equal(t, 1, cache.apiKeyReleaseCalls)
		require.Equal(t, 1, cache.userReleaseCalls)
	})
}

type canceledWSAcquisitionCache struct{ helperConcurrencyCacheStub }

func (c *canceledWSAcquisitionCache) AcquireUserSlot(ctx context.Context, _ int64, _ int, _ string) (bool, error) {
	<-ctx.Done()
	return false, ctx.Err()
}

func TestWSUserSlotAcquisitionRemainsCancellable(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		cache := &canceledWSAcquisitionCache{}
		helper := NewConcurrencyHelper(service.NewConcurrencyService(cache), SSEPingFormatNone, time.Second)
		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan struct{})
		go func() {
			defer close(done)
			release, acquired, err := helper.TryAcquireWSUserSlotForAPIKey(ctx, 202, 3, 77, 0)
			require.ErrorIs(t, err, context.Canceled)
			require.False(t, acquired)
			require.Nil(t, release)
		}()
		synctest.Wait()
		cancel()
		<-done
		// Key admission is attempted before the user slot (per the queue design),
		// and a cancelled user acquisition must not leak the key handle.
		require.Equal(t, 1, cache.apiKeyTrackCalls)
		require.Equal(t, 1, cache.apiKeyReleaseCalls)
	})
}

func TestHTTPAPIKeySlotStillReleasesOnCancellation(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		cache := &helperConcurrencyCacheStub{userSeq: []bool{true}}
		helper := NewConcurrencyHelper(service.NewConcurrencyService(cache), SSEPingFormatNone, time.Second)
		ctx, cancel := context.WithCancel(context.Background())
		release, acquired, err := helper.TryAcquireUserSlotForAPIKey(ctx, 202, 3, 77, 0)
		require.NoError(t, err)
		require.True(t, acquired)
		cancel()
		synctest.Wait()
		require.Equal(t, 1, cache.apiKeyReleaseCalls)
		release()
		require.Equal(t, 1, cache.apiKeyReleaseCalls)
	})
}
