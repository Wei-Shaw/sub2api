//go:build integration

package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

var backgroundTaskIntegrationSequence atomic.Int64

func backgroundTaskIntegrationInput(t *testing.T, suffix string) *service.CreateBackgroundTaskInput {
	t.Helper()
	sequence := backgroundTaskIntegrationSequence.Add(1)
	key := fmt.Sprintf("integration:%s:%d", suffix, sequence)
	t.Cleanup(func() {
		_, _ = integrationDB.ExecContext(context.Background(), `DELETE FROM background_task_runs WHERE dedupe_key = $1`, key)
	})
	return &service.CreateBackgroundTaskInput{
		TaskType: "integration_task", ResourceType: "test_resource", ResourceID: fmt.Sprint(sequence),
		Payload: json.RawMessage(`{"private":"value"}`), Display: json.RawMessage(`{"display":"value"}`),
		RunAt: time.Now().Add(-time.Minute), DedupeKey: key,
		CreationRequestKey:     "creation:" + key,
		GenerateIdempotencyKey: true, MaxAttempts: 5, CreatedBy: 1,
	}
}

func TestBackgroundTaskRepositoryConcurrentSameCreationKeyReturnsOneTaskAcrossDedupeKeys(t *testing.T) {
	repo := NewBackgroundTaskRepository(integrationDB)
	first := backgroundTaskIntegrationInput(t, "creation-key-first")
	second := backgroundTaskIntegrationInput(t, "creation-key-second")
	second.CreationRequestKey = first.CreationRequestKey

	start := make(chan struct{})
	results := make(chan *service.BackgroundTaskRun, 2)
	errorsFound := make(chan error, 2)
	var wg sync.WaitGroup
	for _, source := range []*service.CreateBackgroundTaskInput{first, second} {
		wg.Add(1)
		go func(source *service.CreateBackgroundTaskInput) {
			defer wg.Done()
			<-start
			input := *source
			task, _, err := repo.Create(context.Background(), &input)
			if err != nil {
				errorsFound <- err
				return
			}
			results <- task
		}(source)
	}
	close(start)
	wg.Wait()
	close(results)
	close(errorsFound)

	for err := range errorsFound {
		require.NoError(t, err)
	}
	var taskID int64
	count := 0
	for task := range results {
		if count == 0 {
			taskID = task.ID
		}
		require.Equal(t, taskID, task.ID)
		require.NotNil(t, task.CreationRequestKey)
		require.Equal(t, first.CreationRequestKey, *task.CreationRequestKey)
		count++
	}
	require.Equal(t, 2, count)

	var rows int
	require.NoError(t, integrationDB.QueryRowContext(context.Background(),
		`SELECT COUNT(*) FROM background_task_runs WHERE creation_request_key = $1`, first.CreationRequestKey,
	).Scan(&rows))
	require.Equal(t, 1, rows)
}

func TestBackgroundTaskRepositoryPersistsCreationAliasForDedupeOwner(t *testing.T) {
	repo := NewBackgroundTaskRepository(integrationDB)
	input := backgroundTaskIntegrationInput(t, "creation-alias-owner")
	owner, created, err := repo.Create(context.Background(), input)
	require.NoError(t, err)
	require.True(t, created)

	aliasKey := input.CreationRequestKey + ":alias"
	aliasInput := *input
	aliasInput.CreationRequestKey = aliasKey
	alias, created, err := repo.Create(context.Background(), &aliasInput)
	require.NoError(t, err)
	require.False(t, created)
	require.Equal(t, owner.ID, alias.ID)

	canceled, err := repo.Cancel(context.Background(), owner.ID, 99, time.Now())
	require.NoError(t, err)
	require.Equal(t, service.BackgroundTaskStatusCanceled, canceled.Status)

	replayInput := *input
	replayInput.CreationRequestKey = aliasKey
	replayInput.DedupeKey += ":different-after-terminal"
	replayInput.ResourceID += ":different-input"
	replayed, created, err := repo.Create(context.Background(), &replayInput)
	require.NoError(t, err)
	require.False(t, created)
	require.Equal(t, owner.ID, replayed.ID)
	require.Equal(t, input.DedupeKey, replayed.DedupeKey)
	require.Equal(t, input.ResourceID, replayed.ResourceID)

	var mappedTaskID int64
	require.NoError(t, integrationDB.QueryRowContext(context.Background(),
		`SELECT task_id FROM background_task_creation_requests WHERE request_key = $1`, aliasKey,
	).Scan(&mappedTaskID))
	require.Equal(t, owner.ID, mappedTaskID)
}

func TestBackgroundTaskRepositoryConcurrentCreateGeneratesOneDurableKey(t *testing.T) {
	repo := NewBackgroundTaskRepository(integrationDB)
	base := backgroundTaskIntegrationInput(t, "concurrent-create")

	const creators = 12
	var wg sync.WaitGroup
	results := make(chan *service.BackgroundTaskRun, creators)
	createdFlags := make(chan bool, creators)
	errorsFound := make(chan error, creators)
	for i := 0; i < creators; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			input := *base
			task, created, err := repo.Create(context.Background(), &input)
			if err != nil {
				errorsFound <- err
				return
			}
			results <- task
			createdFlags <- created
		}()
	}
	wg.Wait()
	close(results)
	close(createdFlags)
	close(errorsFound)

	for err := range errorsFound {
		require.NoError(t, err)
	}
	var firstID int64
	var firstKey string
	resultCount := 0
	for task := range results {
		require.NotNil(t, task.IdempotencyKey)
		if resultCount == 0 {
			firstID = task.ID
			firstKey = *task.IdempotencyKey
		}
		require.Equal(t, firstID, task.ID)
		require.Equal(t, firstKey, *task.IdempotencyKey)
		resultCount++
	}
	require.Equal(t, creators, resultCount)
	require.NotEmpty(t, firstKey)
	createdCount := 0
	for created := range createdFlags {
		if created {
			createdCount++
		}
	}
	require.Equal(t, 1, createdCount)

	var rows int
	require.NoError(t, integrationDB.QueryRowContext(context.Background(),
		`SELECT COUNT(*) FROM background_task_runs WHERE dedupe_key = $1`, base.DedupeKey,
	).Scan(&rows))
	require.Equal(t, 1, rows)
}

func TestBackgroundTaskRepositoryLeaseReclaimFencesOldWorker(t *testing.T) {
	repo := NewBackgroundTaskRepository(integrationDB)
	input := backgroundTaskIntegrationInput(t, "lease-reclaim")
	task, created, err := repo.Create(context.Background(), input)
	require.NoError(t, err)
	require.True(t, created)

	now := time.Now()
	firstClaim, err := repo.ClaimDue(context.Background(), "worker-a", now, now.Add(time.Second), 1)
	require.NoError(t, err)
	require.Len(t, firstClaim, 1)
	require.Equal(t, task.ID, firstClaim[0].ID)

	blockedClaim, err := repo.ClaimDue(context.Background(), "worker-b", now.Add(500*time.Millisecond), now.Add(time.Minute), 1)
	require.NoError(t, err)
	require.Empty(t, blockedClaim)

	reclaimedAt := now.Add(2 * time.Second)
	secondClaim, err := repo.ClaimDue(context.Background(), "worker-b", reclaimedAt, reclaimedAt.Add(time.Minute), 1)
	require.NoError(t, err)
	require.Len(t, secondClaim, 1)
	require.Equal(t, firstClaim[0].ClaimVersion+1, secondClaim[0].ClaimVersion)

	err = repo.BeginDispatch(context.Background(), task.ID, "worker-a", firstClaim[0].ClaimVersion, reclaimedAt)
	require.ErrorIs(t, err, service.ErrBackgroundTaskLeaseLost)
	require.NoError(t, repo.BeginDispatch(context.Background(), task.ID, "worker-b", secondClaim[0].ClaimVersion, reclaimedAt))

	err = repo.Finish(context.Background(), task.ID, "worker-a", firstClaim[0].ClaimVersion, service.BackgroundTaskFinishInput{
		Status: service.BackgroundTaskStatusSucceeded, ResultCode: "stale-success",
	})
	require.ErrorIs(t, err, service.ErrBackgroundTaskLeaseLost)
	require.NoError(t, repo.Finish(context.Background(), task.ID, "worker-b", secondClaim[0].ClaimVersion, service.BackgroundTaskFinishInput{
		Status: service.BackgroundTaskStatusSucceeded, ResultCode: "current-success",
	}))

	finished, err := repo.GetByID(context.Background(), task.ID)
	require.NoError(t, err)
	require.Equal(t, service.BackgroundTaskStatusSucceeded, finished.Status)
	require.Equal(t, "current-success", *finished.ResultCode)
	require.Equal(t, 1, finished.DispatchCount)
}

func TestBackgroundTaskRepositoryCancelWinsBeforeDispatch(t *testing.T) {
	repo := NewBackgroundTaskRepository(integrationDB)
	input := backgroundTaskIntegrationInput(t, "cancel-before-dispatch")
	task, _, err := repo.Create(context.Background(), input)
	require.NoError(t, err)

	now := time.Now()
	claimed, err := repo.ClaimDue(context.Background(), "worker", now, now.Add(time.Minute), 1)
	require.NoError(t, err)
	require.Len(t, claimed, 1)
	canceled, err := repo.Cancel(context.Background(), task.ID, 99, now.Add(time.Millisecond))
	require.NoError(t, err)
	require.Equal(t, service.BackgroundTaskStatusCanceled, canceled.Status)
	require.Nil(t, canceled.FirstDispatchAt)

	err = repo.BeginDispatch(context.Background(), task.ID, "worker", claimed[0].ClaimVersion, now.Add(2*time.Millisecond))
	require.ErrorIs(t, err, service.ErrBackgroundTaskLeaseLost)

	replacementInput := *input
	replacementInput.CreationRequestKey += ":replacement"
	replacement, created, err := repo.Create(context.Background(), &replacementInput)
	require.NoError(t, err)
	require.True(t, created)
	require.NotEqual(t, task.ID, replacement.ID)
	require.NotEqual(t, *task.IdempotencyKey, *replacement.IdempotencyKey)
}

func TestBackgroundTaskRepositoryDispatchWinsAndBlocksCancelOrNewKey(t *testing.T) {
	repo := NewBackgroundTaskRepository(integrationDB)
	input := backgroundTaskIntegrationInput(t, "dispatch-before-cancel")
	task, _, err := repo.Create(context.Background(), input)
	require.NoError(t, err)

	now := time.Now()
	claimed, err := repo.ClaimDue(context.Background(), "worker", now, now.Add(time.Minute), 1)
	require.NoError(t, err)
	require.Len(t, claimed, 1)
	require.NoError(t, repo.BeginDispatch(context.Background(), task.ID, "worker", claimed[0].ClaimVersion, now.Add(time.Millisecond)))

	_, err = repo.Cancel(context.Background(), task.ID, 99, now.Add(2*time.Millisecond))
	require.ErrorIs(t, err, service.ErrBackgroundTaskCannotCancel)
	require.NoError(t, repo.Finish(context.Background(), task.ID, "worker", claimed[0].ClaimVersion, service.BackgroundTaskFinishInput{
		Status: service.BackgroundTaskStatusIndeterminate, ErrorCode: "timeout", ErrorMessage: "unknown outcome",
	}))

	duplicateInput := *input
	duplicateInput.CreationRequestKey += ":duplicate"
	existing, created, err := repo.Create(context.Background(), &duplicateInput)
	require.NoError(t, err)
	require.False(t, created)
	require.Equal(t, task.ID, existing.ID)
	require.Equal(t, *task.IdempotencyKey, *existing.IdempotencyKey)

	requeued, err := repo.RequeueIndeterminate(context.Background(), task.ID, time.Now())
	require.NoError(t, err)
	require.Equal(t, service.BackgroundTaskStatusPending, requeued.Status)
	require.Equal(t, *task.IdempotencyKey, *requeued.IdempotencyKey)
}

func TestBackgroundTaskRepositoryConcurrentCancelAndDispatchAreExclusive(t *testing.T) {
	repo := NewBackgroundTaskRepository(integrationDB)
	input := backgroundTaskIntegrationInput(t, "cancel-dispatch-race")
	task, _, err := repo.Create(context.Background(), input)
	require.NoError(t, err)
	now := time.Now()
	claimed, err := repo.ClaimDue(context.Background(), "worker", now, now.Add(time.Minute), 1)
	require.NoError(t, err)
	require.Len(t, claimed, 1)

	start := make(chan struct{})
	var dispatchErr, cancelErr error
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		<-start
		dispatchErr = repo.BeginDispatch(context.Background(), task.ID, "worker", claimed[0].ClaimVersion, time.Now())
	}()
	go func() {
		defer wg.Done()
		<-start
		_, cancelErr = repo.Cancel(context.Background(), task.ID, 99, time.Now())
	}()
	close(start)
	wg.Wait()

	switch {
	case dispatchErr == nil:
		require.ErrorIs(t, cancelErr, service.ErrBackgroundTaskCannotCancel)
	case cancelErr == nil:
		require.ErrorIs(t, dispatchErr, service.ErrBackgroundTaskLeaseLost)
	default:
		require.Failf(t, "race had no winner", "dispatch=%v cancel=%v", dispatchErr, cancelErr)
	}
	require.False(t, dispatchErr == nil && cancelErr == nil)
	require.False(t, errors.Is(dispatchErr, service.ErrBackgroundTaskLeaseLost) && errors.Is(cancelErr, service.ErrBackgroundTaskCannotCancel))
}
