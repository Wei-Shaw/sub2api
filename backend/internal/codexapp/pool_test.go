//go:build unit

package codexapp

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type recordingWorker struct {
	mu       sync.Mutex
	requests []TurnRequest
	closed   bool
}

func (w *recordingWorker) RunTurn(_ context.Context, request TurnRequest, sink EventSink) (TurnResult, error) {
	w.mu.Lock()
	w.requests = append(w.requests, request)
	turn := len(w.requests)
	w.mu.Unlock()

	threadID := request.ThreadID
	if threadID == "" {
		threadID = fmt.Sprintf("thread-%d", turn)
	}
	if sink != nil {
		if err := sink(Event{Type: EventAgentMessageDelta, ThreadID: threadID, TurnID: fmt.Sprintf("turn-%d", turn), Delta: "ok"}); err != nil {
			return TurnResult{}, err
		}
	}
	return TurnResult{
		ThreadID: threadID,
		TurnID:   fmt.Sprintf("turn-%d", turn),
		Status:   "completed",
		Usage: TokenUsage{
			InputTokens:  2,
			OutputTokens: 1,
			TotalTokens:  3,
		},
	}, nil
}

func (w *recordingWorker) Close(context.Context) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.closed = true
	return nil
}

func (w *recordingWorker) Healthy() bool { return true }

func (w *recordingWorker) snapshot() ([]TurnRequest, bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return append([]TurnRequest(nil), w.requests...), w.closed
}

func TestPoolReusesWorkerAndThreadWithinAccountSession(t *testing.T) {
	var mu sync.Mutex
	created := make(map[string][]*recordingWorker)
	pool := NewPool(PoolConfig{
		MaxWorkers: 4,
		IdleTTL:    time.Hour,
		Factory: func(_ context.Context, workerKey string) (Worker, error) {
			worker := &recordingWorker{}
			mu.Lock()
			created[workerKey] = append(created[workerKey], worker)
			mu.Unlock()
			return worker, nil
		},
	})
	t.Cleanup(func() { require.NoError(t, pool.Close(context.Background())) })

	first, err := pool.RunTurn(context.Background(), "account:41", "api:7:session:alpha", TurnRequest{
		Model: "gpt-test", Input: []InputItem{{Type: "text", Text: "one"}},
	}, nil)
	require.NoError(t, err)
	second, err := pool.RunTurn(context.Background(), "account:41", "api:7:session:alpha", TurnRequest{
		Model: "gpt-test", Input: []InputItem{{Type: "text", Text: "two"}},
	}, nil)
	require.NoError(t, err)

	require.Equal(t, first.ThreadID, second.ThreadID)
	mu.Lock()
	require.Len(t, created["account:41"], 1, "one account should keep one warm app-server worker")
	worker := created["account:41"][0]
	mu.Unlock()
	requests, _ := worker.snapshot()
	require.Len(t, requests, 2)
	require.Empty(t, requests[0].ThreadID)
	require.False(t, requests[0].Ephemeral)
	require.Equal(t, first.ThreadID, requests[1].ThreadID)
}

func TestPoolIsolatesWorkersByAccountAndMakesStatelessTurnsEphemeral(t *testing.T) {
	var mu sync.Mutex
	created := make(map[string]int)
	pool := NewPool(PoolConfig{
		MaxWorkers: 2,
		IdleTTL:    time.Hour,
		Factory: func(_ context.Context, workerKey string) (Worker, error) {
			mu.Lock()
			created[workerKey]++
			mu.Unlock()
			return &recordingWorker{}, nil
		},
	})
	t.Cleanup(func() { require.NoError(t, pool.Close(context.Background())) })

	for _, workerKey := range []string{"account:1", "account:2"} {
		_, err := pool.RunTurn(context.Background(), workerKey, "", TurnRequest{
			Model: "gpt-test", Input: []InputItem{{Type: "text", Text: "hello"}},
		}, nil)
		require.NoError(t, err)
	}

	mu.Lock()
	defer mu.Unlock()
	require.Equal(t, map[string]int{"account:1": 1, "account:2": 1}, created)
}

func TestPoolRejectsNewAccountWhenAllWorkersAreBusy(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	pool := NewPool(PoolConfig{
		MaxWorkers: 1,
		IdleTTL:    time.Hour,
		Factory: func(_ context.Context, _ string) (Worker, error) {
			return blockingWorker{started: started, release: release}, nil
		},
	})
	t.Cleanup(func() { require.NoError(t, pool.Close(context.Background())) })

	done := make(chan error, 1)
	go func() {
		_, err := pool.RunTurn(context.Background(), "account:1", "", TurnRequest{
			Model: "gpt-test", Input: []InputItem{{Type: "text", Text: "hello"}},
		}, nil)
		done <- err
	}()
	<-started

	_, err := pool.RunTurn(context.Background(), "account:2", "", TurnRequest{
		Model: "gpt-test", Input: []InputItem{{Type: "text", Text: "hello"}},
	}, nil)
	require.ErrorIs(t, err, ErrPoolExhausted)
	close(release)
	require.NoError(t, <-done)
}

func TestPoolDiscardsUnhealthyWorkerAndRecreatesIt(t *testing.T) {
	var mu sync.Mutex
	created := 0
	pool := NewPool(PoolConfig{
		MaxWorkers: 1,
		Factory: func(_ context.Context, _ string) (Worker, error) {
			mu.Lock()
			created++
			mu.Unlock()
			return &unhealthyAfterTurnWorker{}, nil
		},
	})
	t.Cleanup(func() { require.NoError(t, pool.Close(context.Background())) })

	for range 2 {
		_, err := pool.RunTurn(context.Background(), "account:1", "session", TurnRequest{
			Model: "gpt-test", Input: []InputItem{{Type: "text", Text: "hello"}},
		}, nil)
		require.NoError(t, err)
	}

	mu.Lock()
	defer mu.Unlock()
	require.Equal(t, 2, created, "an unhealthy process must not be reused")
}

func TestPoolEvictsIdleWorkerAndRejectsUseAfterClose(t *testing.T) {
	first := &recordingWorker{}
	second := &recordingWorker{}
	created := 0
	pool := NewPool(PoolConfig{
		MaxWorkers: 1,
		IdleTTL:    time.Nanosecond,
		Factory: func(_ context.Context, _ string) (Worker, error) {
			created++
			if created == 1 {
				return first, nil
			}
			return second, nil
		},
	})

	_, err := pool.RunTurn(context.Background(), "account:1", "", TurnRequest{
		Model: "gpt-test", Input: []InputItem{{Type: "text", Text: "one"}},
	}, nil)
	require.NoError(t, err)
	time.Sleep(time.Millisecond)
	_, err = pool.RunTurn(context.Background(), "account:2", "", TurnRequest{
		Model: "gpt-test", Input: []InputItem{{Type: "text", Text: "two"}},
	}, nil)
	require.NoError(t, err)
	require.Eventually(t, func() bool {
		_, closed := first.snapshot()
		return closed
	}, time.Second, time.Millisecond)

	require.NoError(t, pool.Close(context.Background()))
	_, err = pool.RunTurn(context.Background(), "account:3", "", TurnRequest{
		Model: "gpt-test", Input: []InputItem{{Type: "text", Text: "three"}},
	}, nil)
	require.EqualError(t, err, "codex app-server worker pool is closed")
}

func TestPoolReportsFactoryFailureAndMissingConfiguration(t *testing.T) {
	wantErr := errors.New("start failed")
	pool := NewPool(PoolConfig{Factory: func(context.Context, string) (Worker, error) {
		return nil, wantErr
	}})
	_, err := pool.RunTurn(context.Background(), "account:1", "", TurnRequest{
		Model: "gpt-test", Input: []InputItem{{Type: "text", Text: "hello"}},
	}, nil)
	require.ErrorIs(t, err, wantErr)

	missingFactory := NewPool(PoolConfig{})
	_, err = missingFactory.RunTurn(context.Background(), "account:1", "", TurnRequest{
		Model: "gpt-test", Input: []InputItem{{Type: "text", Text: "hello"}},
	}, nil)
	require.EqualError(t, err, "codex app-server worker factory is required")

	var nilPool *Pool
	_, err = nilPool.RunTurn(context.Background(), "account:1", "", TurnRequest{}, nil)
	require.EqualError(t, err, "codex app-server worker pool is nil")
	require.NoError(t, nilPool.Close(context.Background()))
}

type blockingWorker struct {
	started chan<- struct{}
	release <-chan struct{}
}

func (w blockingWorker) RunTurn(context.Context, TurnRequest, EventSink) (TurnResult, error) {
	close(w.started)
	<-w.release
	return TurnResult{ThreadID: "thread", TurnID: "turn", Status: "completed"}, nil
}

func (blockingWorker) Close(context.Context) error { return nil }
func (blockingWorker) Healthy() bool               { return true }

type unhealthyAfterTurnWorker struct{}

func (*unhealthyAfterTurnWorker) RunTurn(context.Context, TurnRequest, EventSink) (TurnResult, error) {
	return TurnResult{ThreadID: "thread", TurnID: "turn", Status: "completed"}, nil
}

func (*unhealthyAfterTurnWorker) Close(context.Context) error { return nil }
func (*unhealthyAfterTurnWorker) Healthy() bool               { return false }
