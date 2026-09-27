package service

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type evaluationHistoryStub struct {
	report               *ModelEvaluationReport
	rows                 map[int]*ModelEvaluationRound
	createErr, finishErr error
	saveCanceled         bool
	page, size           int
}

func evaluationCopy[T any](value T) T {
	payload, _ := json.Marshal(value)
	var copied T
	_ = json.Unmarshal(payload, &copied)
	return copied
}

func (r *evaluationHistoryStub) Create(_ context.Context, report *ModelEvaluationReport) error {
	if r.createErr != nil {
		return r.createErr
	}
	r.report = evaluationCopy(report)
	return nil
}
func (r *evaluationHistoryStub) Get(context.Context, string) (*ModelEvaluationReport, error) {
	report := evaluationCopy(r.report)
	for _, row := range r.rows {
		report.Results = append(report.Results, evaluationCopy(row))
	}
	return report, nil
}
func (r *evaluationHistoryStub) List(_ context.Context, _ string, _ int64, page, size int) ([]*ModelEvaluationReport, int64, error) {
	r.page, r.size = page, size
	return []*ModelEvaluationReport{evaluationCopy(r.report)}, 1, nil
}
func (r *evaluationHistoryStub) Reserve(_ context.Context, _ string, number int, initial *ModelEvaluationResult) (*ModelEvaluationRound, bool, error) {
	if row := r.rows[number]; row != nil {
		return evaluationCopy(row), false, nil
	}
	row := &ModelEvaluationRound{Round: number, Result: initial, Saved: true}
	r.rows[number] = evaluationCopy(row)
	return row, true, nil
}
func (r *evaluationHistoryStub) Finish(ctx context.Context, _ string, row *ModelEvaluationRound) error {
	r.saveCanceled = ctx.Err() != nil
	if r.finishErr != nil {
		return r.finishErr
	}
	r.rows[row.Round] = evaluationCopy(row)
	return nil
}

func newEvaluationHistoryTestService(t *testing.T) (*ModelEvaluationHistoryService, *evaluationHistoryStub, *evaluationGatewayStub) {
	t.Helper()
	evaluator, gateway, _ := newEvaluationTestService()
	repo := &evaluationHistoryStub{rows: make(map[int]*ModelEvaluationRound)}
	svc := NewModelEvaluationHistoryService(evaluator, repo)
	_, err := svc.Create(context.Background(), ModelEvaluationCreate{
		ModelEvaluationRequest: ModelEvaluationRequest{TargetType: "account", TargetID: 42, Model: "gpt-5.4", Effort: "high"}, Rounds: 5,
	}, 7)
	require.NoError(t, err)
	return svc, repo, gateway
}

func TestModelEvaluationHistoryPersistsPrivateSnapshotsAndReplaysWithoutSpending(t *testing.T) {
	s, repo, gateway := newEvaluationHistoryTestService(t)
	report := repo.report
	require.Equal(t, "private", report.Visibility)
	require.EqualValues(t, 7, report.CreatedBy)
	require.Equal(t, modelEvaluationPrompt, report.Prompt)
	require.Equal(t, "target", report.TargetName)
	row, err := s.Run(context.Background(), report.ID, 1)
	require.NoError(t, err)
	require.True(t, row.Saved)
	require.Equal(t, "correct", repo.rows[1].Result.Status)
	require.Equal(t, row.Result.Text, repo.rows[1].Result.Text)
	require.NotEmpty(t, repo.rows[1].Result.UpstreamModel)
	repeated, err := s.Run(context.Background(), report.ID, 1)
	require.NoError(t, err)
	require.Equal(t, row.Result.ID, repeated.Result.ID)
	require.Equal(t, 1, gateway.forwardCalls)
	// Reading snapshots does not need the target to still exist.
	s.evaluator.accounts = evaluationAccountsStub{}
	loaded, err := s.Get(context.Background(), report.ID)
	require.NoError(t, err)
	require.Equal(t, "target", loaded.TargetName)
	require.Len(t, loaded.Results, 1)
	_, err = s.List(context.Background(), "account", 42, 2)
	require.NoError(t, err)
	require.Equal(t, 2, repo.page)
	require.Equal(t, 20, repo.size)
}

func TestModelEvaluationHistorySaveFailureRetainsResultAndBlocksReplay(t *testing.T) {
	s, repo, gateway := newEvaluationHistoryTestService(t)
	repo.finishErr = errors.New("database-password-must-not-leak")
	row, err := s.Run(context.Background(), repo.report.ID, 1)
	require.NoError(t, err)
	require.False(t, row.Saved)
	require.Equal(t, "correct", row.Result.Status)
	require.Contains(t, row.SaveError, repo.report.ID)
	require.Contains(t, row.SaveError, "第 1 轮")
	require.NotContains(t, row.SaveError, "database-password")
	_, err = s.Run(context.Background(), repo.report.ID, 1)
	require.ErrorIs(t, err, ErrEvaluationRoundConflict)
	require.Equal(t, 1, gateway.forwardCalls)
}

func TestModelEvaluationHistoryCancellationStillSavesAndUnfinishedRoundsRemainVisible(t *testing.T) {
	s, repo, gateway := newEvaluationHistoryTestService(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	row, err := s.Run(ctx, repo.report.ID, 1)
	require.NoError(t, err)
	require.True(t, row.Saved)
	require.Equal(t, "error", repo.rows[1].Result.Status)
	require.False(t, repo.saveCanceled)
	require.Zero(t, gateway.forwardCalls)
	repo.rows[2] = &ModelEvaluationRound{Round: 2, Saved: true, Result: &ModelEvaluationResult{Status: "running", StartedAt: time.Now().Add(-5 * time.Minute)}}
	report, err := s.Get(context.Background(), repo.report.ID)
	require.NoError(t, err)
	for _, row := range report.Results {
		if row.Round == 2 {
			require.Equal(t, "interrupted", row.Result.Status)
			require.NotEmpty(t, row.Result.Error)
		}
	}
	require.Equal(t, "running", repo.rows[2].Result.Status)
}

func TestModelEvaluationHistoryRejectsInvalidAndUnavailableStorageBeforeModelCalls(t *testing.T) {
	s, repo, gateway := newEvaluationHistoryTestService(t)
	repo.createErr = errors.New("private storage error")
	_, err := s.Create(context.Background(), ModelEvaluationCreate{ModelEvaluationRequest: ModelEvaluationRequest{TargetType: "account", TargetID: 42, Model: "gpt-5.4", Effort: "high"}, Rounds: 1}, 7)
	require.ErrorContains(t, err, "尚未调用模型")
	require.NotContains(t, err.Error(), "private storage")
	for _, number := range []int{0, 6} {
		_, err = s.Run(context.Background(), repo.report.ID, number)
		require.Error(t, err)
	}
	_, err = s.Get(context.Background(), "bad-id")
	require.Error(t, err)
	_, err = s.List(context.Background(), "account", 42, -1)
	require.Error(t, err)
	repo.report.Benchmark = "old-version"
	_, err = s.Run(context.Background(), repo.report.ID, 1)
	require.ErrorContains(t, err, "题目版本")
	require.Zero(t, gateway.forwardCalls)
}
