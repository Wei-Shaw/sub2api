//go:build unit

package service

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// a204CapRepo simulates concurrent submits: CountUnfinishedJobs blocks until every submitter has
// counted, so a non-atomic count-then-insert always overshoots the cap. CreateJobWithinLimit counts
// and inserts under one lock, like the repository's advisory-locked transaction.
type a204CapRepo struct {
	ImageStudioRepository
	mu         sync.Mutex
	parties    int
	arrived    int
	allCounted chan struct{}
	created    int
}

func (r *a204CapRepo) FailStaleJobs(context.Context, int64, time.Time, string) error { return nil }

func (r *a204CapRepo) CountUnfinishedJobs(context.Context, int64) (int, error) {
	r.mu.Lock()
	count := r.created
	r.arrived++
	if r.arrived == r.parties {
		close(r.allCounted)
	}
	r.mu.Unlock()
	select {
	case <-r.allCounted:
	case <-time.After(2 * time.Second):
	}
	return count, nil
}

func (r *a204CapRepo) CreateJob(_ context.Context, job *ImageStudioJob) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.created++
	job.ID = int64(r.created)
	return nil
}

func (r *a204CapRepo) CreateJobWithinLimit(_ context.Context, job *ImageStudioJob, limit int) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.created >= limit {
		return ErrImageStudioBusy
	}
	r.created++
	job.ID = int64(r.created)
	return nil
}

func TestImageStudioSubmitConcurrentRespectsUnfinishedCapA204(t *testing.T) {
	const parties = 8
	repo := &a204CapRepo{parties: parties, allCounted: make(chan struct{})}
	studio := newTestImageStudio(&fakeImageStudioRepo{}, &fakeImageObjectStore{})
	studio.repo = repo

	var wg sync.WaitGroup
	errs := make(chan error, parties)
	for i := 0; i < parties; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := studio.Submit(context.Background(), 10, ImageStudioCreateInput{APIKeyID: 1, Model: "gpt-image-2", Prompt: "a pelican"})
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)

	accepted := 0
	for err := range errs {
		if err == nil {
			accepted++
			continue
		}
		require.ErrorIs(t, err, ErrImageStudioBusy)
	}
	require.Equal(t, imageStudioMaxUnfinishedPerUser, accepted, "concurrent submits must not bypass the per-user unfinished job cap")
	require.Equal(t, imageStudioMaxUnfinishedPerUser, repo.created)
}
