package service

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// reviewRunRepoStub 是 IntelligenceCheckRunRepository 的最小测试桩：
// 嵌入接口，未实现的方法一旦被调用会 panic，正好把「测试里不该走到别的仓储方法」暴露出来。
type reviewRunRepoStub struct {
	IntelligenceCheckRunRepository

	run        *IntelligenceCheckRun
	getErr     error
	updateErr  error
	reviewCall int
}

func (r *reviewRunRepoStub) GetByID(_ context.Context, _ int64) (*IntelligenceCheckRun, error) {
	if r.getErr != nil {
		return nil, r.getErr
	}
	return r.run, nil
}

func (r *reviewRunRepoStub) UpdateReview(_ context.Context, run *IntelligenceCheckRun) error {
	r.reviewCall++
	r.run = run
	return r.updateErr
}

// newReviewService 构造一个只带跑测仓储的评审服务。
// settingSvc 传 nil 表示联动开关不可用 —— syncAccountStatus 会直接短路，
// 因此这些用例覆盖的是「评审本身」，不掺入账号状态联动。
func newReviewService(repo *reviewRunRepoStub) *IntelligenceCheckService {
	return NewIntelligenceCheckService(nil, repo, nil, nil, nil, nil, nil)
}

func reviewableRun() *IntelligenceCheckRun {
	finished := time.Now().Add(-time.Minute)
	return &IntelligenceCheckRun{
		ID:         7,
		AccountID:  42,
		Status:     IntelligenceCheckStatusCompleted,
		Verdict:    IntelligenceCheckVerdictUnknown,
		HasHTML:    true,
		HTMLBytes:  1024,
		FinishedAt: &finished,
	}
}

func TestIntelligenceCheckReviewRunRejectsInvalidVerdict(t *testing.T) {
	for _, verdict := range []string{"", "  ", "unknown", "PASS", "approved"} {
		t.Run("verdict="+verdict, func(t *testing.T) {
			repo := &reviewRunRepoStub{run: reviewableRun()}
			svc := newReviewService(repo)

			run, sync, err := svc.ReviewRun(context.Background(), 7, verdict, 1)

			require.ErrorIs(t, err, ErrIntelligenceCheckInvalidVerdict)
			require.Nil(t, run)
			require.Nil(t, sync)
			// 非法结论绝不能落库。
			require.Zero(t, repo.reviewCall)
		})
	}
}

func TestIntelligenceCheckReviewRunRejectsNotReviewable(t *testing.T) {
	queued := reviewableRun()
	queued.Status = IntelligenceCheckStatusQueued
	queued.HasHTML = false

	failed := reviewableRun()
	failed.Status = IntelligenceCheckStatusFailed
	failed.HasHTML = false
	failed.ErrorCode = IntelligenceCheckErrTimeout

	noArtifact := reviewableRun()
	noArtifact.HasHTML = false

	cases := map[string]*IntelligenceCheckRun{
		"queued":      queued,
		"failed":      failed,
		"no artifact": noArtifact,
	}
	for name, run := range cases {
		t.Run(name, func(t *testing.T) {
			repo := &reviewRunRepoStub{run: run}
			svc := newReviewService(repo)

			_, _, err := svc.ReviewRun(context.Background(), 7, IntelligenceCheckVerdictPass, 1)

			require.ErrorIs(t, err, ErrIntelligenceCheckNotReviewable)
			require.Zero(t, repo.reviewCall)
		})
	}
}

func TestIntelligenceCheckReviewRunPropagatesLookupError(t *testing.T) {
	repo := &reviewRunRepoStub{getErr: sql.ErrNoRows}
	svc := newReviewService(repo)

	_, _, err := svc.ReviewRun(context.Background(), 7, IntelligenceCheckVerdictPass, 1)

	require.ErrorIs(t, err, sql.ErrNoRows)
	require.Zero(t, repo.reviewCall)
}

func TestIntelligenceCheckReviewRunPersistsVerdictAndReviewer(t *testing.T) {
	for _, verdict := range []string{IntelligenceCheckVerdictPass, IntelligenceCheckVerdictFail} {
		t.Run(verdict, func(t *testing.T) {
			repo := &reviewRunRepoStub{run: reviewableRun()}
			svc := newReviewService(repo)
			before := time.Now()

			run, sync, err := svc.ReviewRun(context.Background(), 7, "  "+verdict+"  ", 99)

			require.NoError(t, err)
			require.Equal(t, 1, repo.reviewCall)
			require.Equal(t, verdict, run.Verdict)
			require.EqualValues(t, 99, run.ReviewedBy)
			require.NotNil(t, run.ReviewedAt)
			require.False(t, run.ReviewedAt.Before(before))
			// 评审不得触碰跑测结果字段。
			require.Equal(t, IntelligenceCheckStatusCompleted, run.Status)
			require.True(t, run.HasHTML)

			// settingSvc 为 nil：联动必须完全不动账号状态。
			require.NotNil(t, sync)
			require.False(t, sync.Enabled)
			require.False(t, sync.Applied)
			require.EqualValues(t, 42, sync.AccountID)
		})
	}
}

func TestIntelligenceCheckReviewRunPropagatesReviewWriteError(t *testing.T) {
	writeErr := errors.New("db down")
	repo := &reviewRunRepoStub{run: reviewableRun(), updateErr: writeErr}
	svc := newReviewService(repo)

	run, _, err := svc.ReviewRun(context.Background(), 7, IntelligenceCheckVerdictFail, 1)

	require.ErrorIs(t, err, writeErr)
	require.Nil(t, run)
}
