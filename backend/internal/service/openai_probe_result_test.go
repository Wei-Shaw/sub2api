package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type openAIProbeContextKey string

type openAIProbePersistenceContextRecord struct {
	err           error
	hasDeadline   bool
	remaining     time.Duration
	retainedValue any
	accountID     int64
	updates       map[string]any
}

type openAIProbePersistenceContextRepo struct {
	record openAIProbePersistenceContextRecord
	err    error
}

func (r *openAIProbePersistenceContextRepo) UpdateExtra(ctx context.Context, accountID int64, updates map[string]any) error {
	r.record.err = ctx.Err()
	deadline, hasDeadline := ctx.Deadline()
	r.record.hasDeadline = hasDeadline
	if hasDeadline {
		r.record.remaining = time.Until(deadline)
	}
	r.record.retainedValue = ctx.Value(openAIProbeContextKey("trace"))
	r.record.accountID = accountID
	r.record.updates = updates
	return r.err
}

func TestPersistOpenAIProbeExtra_DetachesCallerCancellationAndBoundsWrite(t *testing.T) {
	callerCtx := context.WithValue(context.Background(), openAIProbeContextKey("trace"), "trace-123")
	callerCtx, cancel := context.WithCancel(callerCtx)
	cancel()

	repo := &openAIProbePersistenceContextRepo{}
	updates := map[string]any{"openai_compact_supported": nil}
	require.NoError(t, persistOpenAIProbeExtra(callerCtx, repo, 42, updates))
	require.NoError(t, repo.record.err)
	require.True(t, repo.record.hasDeadline)
	require.Positive(t, repo.record.remaining)
	require.LessOrEqual(t, repo.record.remaining, openAIProbePersistenceTimeout)
	require.Equal(t, "trace-123", repo.record.retainedValue)
	require.Equal(t, int64(42), repo.record.accountID)
	require.Equal(t, updates, repo.record.updates)
}

func TestPersistOpenAIProbeExtra_HandlesNilRepoAndEmptyUpdates(t *testing.T) {
	require.NoError(t, persistOpenAIProbeExtra(context.TODO(), nil, 1, map[string]any{"key": "value"}))
	require.NoError(t, persistOpenAIProbeExtra(context.TODO(), &openAIProbePersistenceContextRepo{}, 1, nil))
}

func TestPersistOpenAIProbeExtra_PropagatesRepositoryError(t *testing.T) {
	wantErr := errors.New("database unavailable")
	repo := &openAIProbePersistenceContextRepo{err: wantErr}
	err := persistOpenAIProbeExtra(context.Background(), repo, 7, map[string]any{"key": "value"})
	require.ErrorIs(t, err, wantErr)
	require.Equal(t, int64(7), repo.record.accountID)
}
