package securityaudit

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestEnqueuerNotifiesOnlyAfterPublish(t *testing.T) {
	cases := []struct {
		name       string
		publishErr error
		want       int
	}{
		{name: "published", want: 1},
		{name: "publish failed", publishErr: errors.New("db down"), want: 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := &fakeJobRepository{createJob: &Job{ID: 41}, publishErr: tc.publishErr}
			enqueuer := NewEnqueuer(&fakeConfigStore{cfg: asyncConfig(), active: true}, repo, &fakePayloadStore{values: map[int64]string{}})
			notified := 0
			enqueuer.notify = func() { notified++ }
			_ = enqueuer.Enqueue(context.Background(), asyncRequest())
			require.Equal(t, tc.want, notified)
		})
	}
}

// Both cases must finish before the first fallback tick (workerIdlePoll), so they
// exercise the start-up drain and the wake path rather than polling.
func TestWorkerClaimsWithoutWaitingForIdlePoll(t *testing.T) {
	newRunner := func(repo *fakeJobRepository) *Runner {
		payload := &fakePayloadStore{values: map[int64]string{51: "abc"}}
		return NewRunner(&fakeConfigStore{cfg: asyncConfig(), active: true}, repo, payload, PromptScannerFunc(func(context.Context, ActiveEndpoint, string, []string) (*NormalizedResult, error) {
			return integrationResult(EventPass), nil
		}), NewAtomicMetrics())
	}
	completed := func(repo *fakeJobRepository) func() bool {
		return func() bool { repo.mu.Lock(); defer repo.mu.Unlock(); return repo.completeCount == 1 }
	}
	stop := func(t *testing.T, runner *Runner) {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		require.NoError(t, runner.Shutdown(ctx))
	}

	t.Run("jobs left from a previous run are drained at start", func(t *testing.T) {
		repo := &fakeJobRepository{claimQueue: []*Job{workerJob(1, 3)}}
		runner := newRunner(repo)
		require.NoError(t, runner.Start(context.Background()))
		defer stop(t, runner)
		require.Eventually(t, completed(repo), workerIdlePoll/2, 5*time.Millisecond)
	})

	t.Run("notify wakes an idle worker", func(t *testing.T) {
		repo := &fakeJobRepository{}
		runner := newRunner(repo)
		require.NoError(t, runner.Start(context.Background()))
		defer stop(t, runner)
		// wait for the start-up drain to find an empty queue
		require.Eventually(t, func() bool { repo.mu.Lock(); defer repo.mu.Unlock(); return repo.claims > 0 }, workerIdlePoll/2, time.Millisecond)
		repo.mu.Lock()
		repo.claimQueue = append(repo.claimQueue, workerJob(1, 3))
		repo.mu.Unlock()
		runner.Notify()
		require.Eventually(t, completed(repo), workerIdlePoll/2, 5*time.Millisecond)
	})

	t.Run("notify never blocks", func(t *testing.T) {
		runner := newRunner(&fakeJobRepository{})
		for i := 0; i < 3; i++ {
			runner.Notify()
		}
		var nilRunner *Runner
		nilRunner.Notify()
	})
}
