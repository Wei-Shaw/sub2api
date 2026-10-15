//go:build integration

package repository

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	dbmigrations "github.com/Wei-Shaw/sub2api/migrations"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestModelEvaluationRepositoryDurableHistory(t *testing.T) {
	ctx := context.Background()
	repo := NewModelEvaluationRepository(integrationDB)
	report := &service.ModelEvaluationReport{
		ID: uuid.NewString(), TargetType: "account", TargetID: time.Now().UnixNano(), TargetName: "retained snapshot",
		Model: "gpt-5.4", Effort: "high", Rounds: 2, Benchmark: "candy-shape-v1", Prompt: "versioned prompt",
		Visibility: "public", CreatedAt: time.Now().UTC(),
	}
	t.Cleanup(func() {
		_, _ = integrationDB.ExecContext(ctx, `DELETE FROM model_evaluation_rounds WHERE report_id=$1`, report.ID)
		_, _ = integrationDB.ExecContext(ctx, `DELETE FROM model_evaluation_reports WHERE id=$1`, report.ID)
	})
	require.NoError(t, repo.Create(ctx, report))
	loaded, err := repo.Get(ctx, report.ID)
	require.NoError(t, err)
	require.Equal(t, "private", loaded.Visibility)
	require.Equal(t, report.Prompt, loaded.Prompt)
	require.Empty(t, loaded.Results)

	initial := &service.ModelEvaluationResult{ID: uuid.NewString(), Status: "running", StartedAt: time.Now().UTC()}
	var acquired atomic.Int32
	var wg sync.WaitGroup
	errors := make(chan error, 8)
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, created, err := repo.Reserve(ctx, report.ID, 1, initial)
			if created {
				acquired.Add(1)
			}
			errors <- err
		}()
	}
	wg.Wait()
	close(errors)
	for err := range errors {
		require.NoError(t, err)
	}
	require.EqualValues(t, 1, acquired.Load())
	_, _, err = repo.Reserve(ctx, report.ID, 2, initial)
	require.ErrorIs(t, err, service.ErrEvaluationRoundConflict)

	answer := 21
	initial.Status, initial.Text, initial.Answer = "correct", "FINAL_ANSWER: 21", &answer
	require.NoError(t, repo.Finish(ctx, report.ID, &service.ModelEvaluationRound{Round: 1, Result: initial}))
	// A fresh repository instance and migration replay retain the immutable result.
	migration, err := dbmigrations.FS.ReadFile("242_model_evaluation_reports.sql")
	require.NoError(t, err)
	_, err = integrationDB.ExecContext(ctx, string(migration))
	require.NoError(t, err)
	repo = NewModelEvaluationRepository(integrationDB)
	loaded, err = repo.Get(ctx, report.ID)
	require.NoError(t, err)
	require.Equal(t, 1, loaded.Completed)
	require.Equal(t, 1, loaded.Correct)
	require.Equal(t, initial.Text, loaded.Results[0].Result.Text)
	require.Nil(t, loaded.Results[0].Result.ReasoningTokens)
	items, total, err := repo.List(ctx, "account", report.TargetID, 1, 20)
	require.NoError(t, err)
	require.EqualValues(t, 1, total)
	require.Len(t, items, 1)
	require.Empty(t, items[0].Results)
	require.Empty(t, items[0].Prompt)
	items, total, err = repo.List(ctx, "group", report.TargetID, 1, 20)
	require.NoError(t, err)
	require.Zero(t, total)
	require.Empty(t, items)
}
