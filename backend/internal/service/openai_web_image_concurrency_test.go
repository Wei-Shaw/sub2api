package service

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/chatgptwebimage"
	"github.com/stretchr/testify/require"
)

type webImageTestLimiter struct {
	semaphore chan struct{}
	acquires  atomic.Int32
	wait      bool
}

func (l *webImageTestLimiter) acquire(ctx context.Context) (func(), bool) {
	l.acquires.Add(1)
	if l.wait {
		select {
		case l.semaphore <- struct{}{}:
		case <-ctx.Done():
			return nil, false
		}
	} else {
		select {
		case l.semaphore <- struct{}{}:
		default:
			return nil, false
		}
	}
	var once sync.Once
	return func() { once.Do(func() { <-l.semaphore }) }, true
}

func TestWebImageJobSlotTransferSurvivesDisconnectAndReplayUsesNoCapacity(t *testing.T) {
	store := jobTestStore(t)
	limiter := &webImageTestLimiter{semaphore: make(chan struct{}, 1)}
	parent, cancel := context.WithCancel(context.Background())
	defer cancel()
	ctx, requestSlot := WithOpenAIImageGenerationSlot(parent, limiter.acquire)
	defer requestSlot.Release()
	// A caller that already reserved capacity must transfer, not reacquire it.
	require.NoError(t, requestSlot.Acquire(ctx))
	started, finish := make(chan struct{}), make(chan struct{})
	var finishOnce sync.Once
	t.Cleanup(func() { finishOnce.Do(func() { close(finish) }) })
	result := chatgptwebimage.Result{Images: []chatgptwebimage.Image{{Bytes: []byte("same image"), Format: "png"}}}
	done := make(chan error, 1)
	key := jobTestKey("inherited-slot")
	go func() {
		_, err := store.run(ctx, key, func(jobCtx context.Context) (chatgptwebimage.Result, error) {
			close(started)
			<-finish
			return result, jobCtx.Err()
		})
		done <- err
	}()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("existing image slot was not transferred (max=1 would deadlock)")
	}
	require.Equal(t, int32(1), limiter.acquires.Load())
	cancel()
	require.Error(t, <-done)
	requestSlot.Release()
	require.Len(t, limiter.semaphore, 1, "disconnect must not release the running job's slot")
	otherCtx, otherSlot := WithOpenAIImageGenerationSlot(context.Background(), limiter.acquire)
	defer otherSlot.Release()
	otherKey := jobTestKey("new-turn")
	_, err := store.run(otherCtx, otherKey, func(context.Context) (chatgptwebimage.Result, error) {
		t.Error("second job must not start while the disconnected job is running")
		return result, nil
	})
	require.Equal(t, "gateway_concurrency_limit", requireWebImageTestValue[*OpenAIImagesUpstreamError](t, err).Code)
	_, err = os.Stat(filepath.Join(store.dir, otherKey+".json"))
	require.True(t, os.IsNotExist(err), "capacity rejection must not persist an unstarted job")

	replayCtx, replaySlot := WithOpenAIImageGenerationSlot(context.Background(), func(context.Context) (func(), bool) {
		t.Error("replay must not acquire image capacity")
		return nil, false
	})
	defer replaySlot.Release()
	replayed := make(chan error, 1)
	go func() {
		_, err := store.run(replayCtx, key, func(context.Context) (chatgptwebimage.Result, error) {
			t.Error("active replay must not regenerate")
			return result, nil
		})
		replayed <- err
	}()
	finishOnce.Do(func() { close(finish) })
	require.NoError(t, <-replayed)
	require.Eventually(t, func() bool { return len(limiter.semaphore) == 0 }, time.Second, time.Millisecond)
	// Even with all generation capacity occupied, a persisted replay works.
	hold, acquired := limiter.acquire(context.Background())
	require.True(t, acquired)
	restarted := &webImageJobStore{dir: store.dir, active: make(map[string]*webImageRunningJob)}
	got, err := restarted.run(replayCtx, key, func(context.Context) (chatgptwebimage.Result, error) {
		t.Error("cached replay must not regenerate")
		return result, nil
	})
	require.NoError(t, err)
	require.Equal(t, result, got)
	hold()
	_, err = store.run(otherCtx, otherKey, func(context.Context) (chatgptwebimage.Result, error) { return result, nil })
	require.NoError(t, err, "previously rejected turn must remain retryable")
}

func TestWebImageJobWaitingDoesNotBlockCompletionAndCancelledQueueLeavesNoClaim(t *testing.T) {
	store := jobTestStore(t)
	limiter := &webImageTestLimiter{semaphore: make(chan struct{}, 1), wait: true}
	hold, acquired := limiter.acquire(context.Background())
	require.True(t, acquired)
	defer hold()
	parent, cancel := context.WithCancel(context.Background())
	defer cancel()
	ctx, slot := WithOpenAIImageGenerationSlot(parent, limiter.acquire)
	defer slot.Release()
	key := jobTestKey("cancelled-queue")
	done := make(chan error, 1)
	go func() {
		_, err := store.run(ctx, key, func(context.Context) (chatgptwebimage.Result, error) {
			t.Error("cancelled queue must not generate")
			return chatgptwebimage.Result{}, nil
		})
		done <- err
	}()
	require.Eventually(t, func() bool { return limiter.acquires.Load() == 2 }, time.Second, time.Millisecond)
	// A waiting request must not hold the store mutex needed by a completing
	// generation to persist its result and make capacity available.
	unlocked := make(chan bool, 1)
	go func() {
		store.mu.Lock()
		_, registered := store.active[key]
		store.mu.Unlock()
		unlocked <- registered
	}()
	select {
	case registered := <-unlocked:
		require.True(t, registered, "a queued job must remain available for duplicate callers")
	case <-time.After(time.Second):
		t.Fatal("waiting for image capacity holds the job store lock")
	}
	cancel()
	require.ErrorIs(t, <-done, context.Canceled)
	_, err := os.Stat(filepath.Join(store.dir, key+".json"))
	require.True(t, os.IsNotExist(err))
	require.Len(t, limiter.semaphore, 1)
}

func TestWebImageJobConcurrentNewTurnsRespectSharedLimit(t *testing.T) {
	store := jobTestStore(t)
	limiter := &webImageTestLimiter{semaphore: make(chan struct{}, 2)}
	finish := make(chan struct{})
	var once sync.Once
	t.Cleanup(func() { once.Do(func() { close(finish) }) })
	var generated atomic.Int32
	results := make(chan error, 8)
	for i := 0; i < 8; i++ {
		go func() {
			ctx, slot := WithOpenAIImageGenerationSlot(context.Background(), limiter.acquire)
			defer slot.Release()
			_, err := store.run(ctx, jobTestKey(string(rune('a'+i))), func(context.Context) (chatgptwebimage.Result, error) {
				generated.Add(1)
				<-finish
				return chatgptwebimage.Result{}, nil
			})
			results <- err
		}()
	}
	for i := 0; i < 6; i++ {
		select {
		case err := <-results:
			require.Equal(t, "gateway_concurrency_limit", requireWebImageTestValue[*OpenAIImagesUpstreamError](t, err).Code)
		case <-time.After(time.Second):
			t.Fatal("overflow should be rejected while the first two images run")
		}
	}
	require.Eventually(t, func() bool { return generated.Load() == 2 }, time.Second, time.Millisecond)
	once.Do(func() { close(finish) })
	require.NoError(t, <-results)
	require.NoError(t, <-results)
	require.Eventually(t, func() bool { return len(limiter.semaphore) == 0 }, time.Second, time.Millisecond)
}

func TestWebImageJobSlotReleasedOnStorageFailureAndProviderPanic(t *testing.T) {
	for _, scenario := range []string{"storage", "panic"} {
		t.Run(scenario, func(t *testing.T) {
			store := jobTestStore(t)
			limiter := &webImageTestLimiter{semaphore: make(chan struct{}, 1)}
			ctx, slot := WithOpenAIImageGenerationSlot(context.Background(), limiter.acquire)
			defer slot.Release()
			if scenario == "storage" {
				require.NoError(t, os.Remove(store.dir))
			}
			_, err := store.run(ctx, jobTestKey(scenario), func(context.Context) (chatgptwebimage.Result, error) { panic("provider panic") })
			require.Error(t, err)
			require.Eventually(t, func() bool { return len(limiter.semaphore) == 0 }, time.Second, time.Millisecond)
		})
	}
}
