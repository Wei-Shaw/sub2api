package service

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/chatgptwebimage"
	"github.com/stretchr/testify/require"
)

func TestWebImageJobPreGenerationFailureAllowsSameRequestRetry(t *testing.T) {
	for _, failure := range []*chatgptwebimage.Error{
		{Stage: "bootstrap", Kind: "network"},
		{Stage: "requirements", Kind: "http", StatusCode: http.StatusForbidden},
		{Stage: "prepare", Kind: "http", StatusCode: http.StatusServiceUnavailable},
		{Stage: "generate", Kind: "http", StatusCode: http.StatusTooManyRequests},
	} {
		t.Run(failure.Stage, func(t *testing.T) {
			store := jobTestStore(t)
			key := jobTestKey("retry same body")
			_, err := store.run(context.Background(), key, func(context.Context) (chatgptwebimage.Result, error) {
				return chatgptwebimage.Result{}, fmt.Errorf("provider: %w", failure)
			})
			require.Error(t, err)
			_, statErr := os.Stat(filepath.Join(store.dir, key+".json"))
			require.True(t, errors.Is(statErr, os.ErrNotExist), "a request that never started must not retain a blocking claim")
			// A restarted gateway must also retry the same request, then retain
			// the successful image and normal billing identity for replay.
			restarted := &webImageJobStore{dir: store.dir, active: make(map[string]*webImageRunningJob)}
			calls := 0
			generate := func(context.Context) (chatgptwebimage.Result, error) {
				calls++
				return chatgptwebimage.Result{Images: []chatgptwebimage.Image{{Bytes: []byte("recovered image")}}}, nil
			}
			for i := 0; i < 2; i++ {
				result, retryErr := restarted.run(context.Background(), key, generate)
				require.NoError(t, retryErr)
				require.Equal(t, []byte("recovered image"), result.Images[0].Bytes)
			}
			require.Equal(t, 1, calls)
			ref, refErr := restarted.billingRef(key)
			require.NoError(t, refErr)
			require.NotEmpty(t, ref.billingID)
		})
	}
}

func TestWebImageJobUncertainFailureNeverReleasesClaim(t *testing.T) {
	for name, failure := range map[string]error{
		"accepted network failure": &chatgptwebimage.Error{Stage: "generate", Kind: "network", Accepted: true},
		"accepted rate limit":      &chatgptwebimage.Error{Stage: "poll", Kind: "http", StatusCode: http.StatusTooManyRequests, Accepted: true},
		"untyped timeout":          context.DeadlineExceeded,
		"legacy generic 502":       &OpenAIImagesUpstreamError{StatusCode: http.StatusBadGateway, Code: "chatgpt_web_image_failed"},
	} {
		t.Run(name, func(t *testing.T) {
			store := jobTestStore(t)
			key := jobTestKey("uncertain generation")
			calls := 0
			generate := func(context.Context) (chatgptwebimage.Result, error) {
				calls++
				return chatgptwebimage.Result{}, failure
			}
			_, err := store.run(context.Background(), key, generate)
			require.Error(t, err)
			restarted := &webImageJobStore{dir: store.dir, active: make(map[string]*webImageRunningJob)}
			_, err = restarted.run(context.Background(), key, generate)
			require.Error(t, err)
			require.Equal(t, 1, calls, "uncertain image requests must not be resubmitted")
			record, readErr := readWebImageJob(filepath.Join(store.dir, key+".json"))
			require.NoError(t, readErr)
			require.Equal(t, "complete", record.State)
			require.NotNil(t, record.Error)
		})
	}
}

func TestWebImageJobConcurrentWaitersSharePreGenerationFailure(t *testing.T) {
	store := jobTestStore(t)
	key := jobTestKey("concurrent failed bootstrap")
	started, finish, joined := make(chan struct{}), make(chan struct{}), make(chan struct{}, 4)
	var once sync.Once
	t.Cleanup(func() { once.Do(func() { close(finish) }) })
	var calls atomic.Int32
	generate := func(context.Context) (chatgptwebimage.Result, error) {
		if calls.Add(1) == 1 {
			close(started)
			<-finish
			return chatgptwebimage.Result{}, &chatgptwebimage.Error{Stage: "bootstrap", Kind: "transport"}
		}
		return chatgptwebimage.Result{}, nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	errorsOut := make(chan error, 5)
	go func() { _, err := store.run(ctx, key, generate); errorsOut <- err }()
	select {
	case <-started:
	case <-ctx.Done():
		t.Fatal("first image job did not start")
	}
	for i := 0; i < 4; i++ {
		// Joining a running job releases the duplicate request reservation.
		// Use that observable release as a barrier, without timing sleeps.
		waiterCtx, slot := WithOpenAIImageGenerationSlot(ctx, func(context.Context) (func(), bool) {
			return func() { joined <- struct{}{} }, true
		})
		require.NoError(t, slot.Acquire(waiterCtx))
		go func() { _, err := store.run(waiterCtx, key, generate); errorsOut <- err }()
	}
	for i := 0; i < 4; i++ {
		select {
		case <-joined:
		case <-ctx.Done():
			t.Fatal("retry did not join the active job")
		}
	}
	once.Do(func() { close(finish) })
	for i := 0; i < 5; i++ {
		err := <-errorsOut
		require.Equal(t, "chatgpt_web_image_failed", requireWebImageTestValue[*OpenAIImagesUpstreamError](t, err).Code)
	}
	require.Equal(t, int32(1), calls.Load())
	_, err := store.run(ctx, key, generate)
	require.NoError(t, err)
	require.Equal(t, int32(2), calls.Load(), "a later request may retry after the shared pre-generation failure")
}
