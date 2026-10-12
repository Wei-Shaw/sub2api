//go:build integration

package repository

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestVideoSubmissionConcurrentClaimAndOwnerScope(t *testing.T) {
	r := &usageBillingRepository{db: integrationDB}
	ctx := context.Background()
	key := uuid.NewString()
	var claims atomic.Int64
	var wg sync.WaitGroup
	errs := make(chan error, 12)
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			claimed, _, err := r.ClaimVideoSubmission(ctx, 90001, key, "request-a")
			if claimed {
				claims.Add(1)
			}
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}
	require.EqualValues(t, 1, claims.Load(), "concurrent creates may submit upstream only once")
	claimed, id, err := r.ClaimVideoSubmission(ctx, 90001, key, "request-a")
	require.NoError(t, err)
	require.False(t, claimed)
	require.Empty(t, id, "unknown outcome stays blocked")
	require.NoError(t, r.CompleteVideoSubmission(ctx, 90001, key, "task-1"))
	claimed, id, err = r.ClaimVideoSubmission(ctx, 90001, key, "request-a")
	require.NoError(t, err)
	require.False(t, claimed)
	require.Equal(t, "task-1", id)
	_, _, err = r.ClaimVideoSubmission(ctx, 90001, key, "request-b")
	require.ErrorIs(t, err, service.ErrVideoSubmissionConflict)
	claimed, id, err = r.ClaimVideoSubmission(ctx, 90002, key, "request-a")
	require.NoError(t, err)
	require.True(t, claimed)
	require.Empty(t, id)
	require.Error(t, r.CompleteVideoSubmission(ctx, 90001, key, "task-other"))
}
