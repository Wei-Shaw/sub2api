package service

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type detachedIdempotencyRepo struct {
	*inMemoryIdempotencyRepo
	renewals       atomic.Int32
	savedWithBound bool
}

func (r *detachedIdempotencyRepo) MarkSucceeded(ctx context.Context, id int64, status int, body string, expiresAt time.Time) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	deadline, bounded := ctx.Deadline()
	r.savedWithBound = bounded && time.Until(deadline) <= 5*time.Second
	return r.inMemoryIdempotencyRepo.MarkSucceeded(ctx, id, status, body, expiresAt)
}

func (r *detachedIdempotencyRepo) ExtendProcessingLock(ctx context.Context, id int64, fingerprint string, lockedUntil, expiresAt time.Time) (bool, error) {
	ok, err := r.inMemoryIdempotencyRepo.ExtendProcessingLock(ctx, id, fingerprint, lockedUntil, expiresAt)
	if ok && err == nil {
		r.renewals.Add(1)
	}
	return ok, err
}

func TestIdempotencyCoordinator_DetachedExecutionPersistsAfterCancellation(t *testing.T) {
	for _, deadlineExpires := range []bool{false, true} {
		name := "client cancellation"
		if deadlineExpires {
			name = "execution deadline"
		}
		t.Run(name, func(t *testing.T) {
			repo := &detachedIdempotencyRepo{inMemoryIdempotencyRepo: newInMemoryIdempotencyRepo()}
			coordinator := NewIdempotencyCoordinator(repo, DefaultIdempotencyConfig())
			requestCtx, cancel := context.WithCancel(context.Background())
			defer cancel()
			opts := IdempotencyExecuteOptions{
				Scope: "admin.bulk", Method: "POST", Route: "/bulk", IdempotencyKey: "cancel-test", RequireKey: true,
				Payload: map[string]int{"days": 7}, ExecutionTimeout: time.Second,
			}
			if deadlineExpires {
				opts.ExecutionTimeout = 10 * time.Millisecond
			}
			executions := 0
			execute := func(ctx context.Context) (any, error) {
				executions++
				if deadlineExpires {
					<-ctx.Done()
				} else {
					cancel()
					require.NoError(t, ctx.Err(), "client cancellation must not interrupt the accepted mutation")
				}
				return map[string]int{"success_count": 1, "failed_count": 1}, nil
			}
			result, err := coordinator.Execute(requestCtx, opts, execute)
			require.NoError(t, err)
			require.NotNil(t, result.Data)
			require.True(t, repo.savedWithBound, "result persistence must use a live bounded context")
			replay, err := coordinator.Execute(context.Background(), opts, execute)
			require.NoError(t, err)
			require.True(t, replay.Replayed)
			require.Equal(t, 1, executions)
		})
	}
}

func TestIdempotencyCoordinator_DetachedExecutionRenewsLongBatchLease(t *testing.T) {
	repo := &detachedIdempotencyRepo{inMemoryIdempotencyRepo: newInMemoryIdempotencyRepo()}
	cfg := DefaultIdempotencyConfig()
	cfg.ProcessingTimeout = 30 * time.Millisecond
	cfg.DefaultTTL = 50 * time.Millisecond
	coordinator := NewIdempotencyCoordinator(repo, cfg)
	opts := IdempotencyExecuteOptions{
		Scope: "admin.bulk", Method: "POST", Route: "/bulk", IdempotencyKey: "long-batch", RequireKey: true,
		Payload: map[string]int{"days": 7}, ExecutionTimeout: 2 * time.Second,
	}
	var executions atomic.Int32
	release := make(chan struct{})
	t.Cleanup(func() {
		select {
		case <-release:
		default:
			close(release)
		}
	})
	execute := func(ctx context.Context) (any, error) {
		executions.Add(1)
		select {
		case <-release:
		case <-ctx.Done():
		}
		return map[string]int{"success_count": 1}, nil
	}
	completed := make(chan error, 1)
	go func() {
		_, err := coordinator.Execute(context.Background(), opts, execute)
		completed <- err
	}()
	// Ten renewals exceed both the original processing lease and record TTL.
	require.Eventually(t, func() bool { return repo.renewals.Load() >= 10 }, time.Second, 5*time.Millisecond)
	_, err := coordinator.Execute(context.Background(), opts, execute)
	require.ErrorIs(t, err, ErrIdempotencyInProgress)
	require.Equal(t, int32(1), executions.Load())
	close(release)
	require.NoError(t, <-completed)
	replay, err := coordinator.Execute(context.Background(), opts, execute)
	require.NoError(t, err)
	require.True(t, replay.Replayed)
	require.Equal(t, int32(1), executions.Load())
}
