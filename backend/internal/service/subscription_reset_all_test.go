//go:build unit

package service

import (
	"context"
	"database/sql"
	"fmt"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/enttest"
	"github.com/Wei-Shaw/sub2api/ent/usersubscription"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	_ "modernc.org/sqlite"
)

// resetAllQuotaRepoStub 记录被重置的订阅，并对指定 ID 返回失败。
type resetAllQuotaRepoStub struct {
	UserSubscriptionRepository
	resetIDs   []int64
	failIDs    map[int64]error
	resetCalls int
}

func (r *resetAllQuotaRepoStub) GetByID(_ context.Context, id int64) (*UserSubscription, error) {
	return &UserSubscription{ID: id}, nil
}

func (r *resetAllQuotaRepoStub) ResetUsageWindows(_ context.Context, id int64, daily, weekly, monthly bool, dailyStart, periodicStart time.Time) error {
	r.resetCalls++
	if err, ok := r.failIDs[id]; ok {
		return err
	}
	r.resetIDs = append(r.resetIDs, id)
	return nil
}

func (r *resetAllQuotaRepoStub) Update(_ context.Context, sub *UserSubscription) error {
	return nil
}

func newResetAllQuotaTestClient(t *testing.T) *dbent.Client {
	t.Helper()
	db, err := sql.Open("sqlite", "file:reset_all_quota?mode=memory&cache=shared&_fk=1")
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	_, err = db.Exec("PRAGMA foreign_keys = ON")
	require.NoError(t, err)

	drv := entsql.OpenDB(dialect.SQLite, db)
	client := enttest.NewClient(t, enttest.WithOptions(dbent.Driver(drv)))
	t.Cleanup(func() { _ = client.Close() })
	return client
}

// seedSub 造一条订阅（连同其分组）。三个窗口的用量分别给值。
func seedSub(t *testing.T, client *dbent.Client, id int64, status string, expiresAt time.Time, daily, weekly, monthly float64) {
	t.Helper()
	ctx := context.Background()
	u, err := client.User.Create().
		SetEmail(fmt.Sprintf("user-%d@example.com", id)).
		SetPasswordHash("hash").
		Save(ctx)
	require.NoError(t, err)
	g, err := client.Group.Create().
		SetName(fmt.Sprintf("group-%d", id)).
		Save(ctx)
	require.NoError(t, err)
	_, err = client.UserSubscription.Create().
		SetUserID(u.ID).
		SetGroupID(g.ID).
		SetStatus(status).
		SetStartsAt(time.Now().Add(-time.Hour)).
		SetExpiresAt(expiresAt).
		SetDailyUsageUsd(daily).
		SetWeeklyUsageUsd(weekly).
		SetMonthlyUsageUsd(monthly).
		Save(ctx)
	require.NoError(t, err)
}

type ResetAllQuotaScopeTestSuite struct {
	suite.Suite
	client *dbent.Client
}

func TestResetAllQuotaScopeTestSuite(t *testing.T) {
	suite.Run(t, new(ResetAllQuotaScopeTestSuite))
}

func (s *ResetAllQuotaScopeTestSuite) SetupTest() {
	s.client = newResetAllQuotaTestClient(s.T())
}

func (s *ResetAllQuotaScopeTestSuite) newService() *SubscriptionService {
	svc := NewSubscriptionService(nil, nil, nil, s.client, nil)
	s.T().Cleanup(svc.Stop)
	return svc
}

// 口径：只统计有效订阅（active 且未到期、未软删），且所选窗口中至少一个窗口有用量。
func (s *ResetAllQuotaScopeTestSuite) TestPreviewScopeSemantics() {
	ctx := context.Background()
	future := time.Now().Add(24 * time.Hour)
	past := time.Now().Add(-time.Hour)

	// 1) 有效 + 日窗口有用量 → 命中
	seedSub(s.T(), s.client, 1, SubscriptionStatusActive, future, 1.5, 0, 0)
	// 2) 有效但三个窗口都无用量 → 不命中
	seedSub(s.T(), s.client, 2, SubscriptionStatusActive, future, 0, 0, 0)
	// 3) 已到期（status 仍是 active）→ 不命中
	seedSub(s.T(), s.client, 3, SubscriptionStatusActive, past, 2.5, 0, 0)
	// 4) 已撤销 → 不命中
	seedSub(s.T(), s.client, 4, SubscriptionStatusRevoked, future, 3.5, 0, 0)
	// 5) 有效 + 仅周窗口有用量 → 命中（勾选 weekly 时）
	seedSub(s.T(), s.client, 5, SubscriptionStatusActive, future, 0, 4.5, 0)
	// 6) 有效 + 仅月窗口有用量 → 命中（勾选 monthly 时）
	seedSub(s.T(), s.client, 6, SubscriptionStatusActive, future, 0, 0, 5.5)

	svc := s.newService()

	// 勾选日窗口：只有 1 号有用量
	n, err := svc.ResetAllQuotaPreview(ctx, true, false, false)
	require.NoError(s.T(), err)
	require.Equal(s.T(), 1, n)

	// 勾选周窗口：1 号的 weekly 为 0，故只有 5 号
	n, err = svc.ResetAllQuotaPreview(ctx, false, true, false)
	require.NoError(s.T(), err)
	require.Equal(s.T(), 1, n)

	// 勾选月窗口：只有 6 号
	n, err = svc.ResetAllQuotaPreview(ctx, false, false, true)
	require.NoError(s.T(), err)
	require.Equal(s.T(), 1, n)

	// 三个都勾选：1、5、6
	n, err = svc.ResetAllQuotaPreview(ctx, true, true, true)
	require.NoError(s.T(), err)
	require.Equal(s.T(), 3, n)

	// 一个都不勾选 → 报错
	_, err = svc.ResetAllQuotaPreview(ctx, false, false, false)
	require.Error(s.T(), err)
}

// 分批执行：只取命中口径的订阅。
func (s *ResetAllQuotaScopeTestSuite) TestBatchExecutesOnlyMatchingSubscriptions() {
	ctx := context.Background()
	future := time.Now().Add(24 * time.Hour)
	seedSub(s.T(), s.client, 1, SubscriptionStatusActive, future, 1, 0, 0)
	seedSub(s.T(), s.client, 2, SubscriptionStatusActive, future, 0, 0, 0) // 无用量 → 不取
	seedSub(s.T(), s.client, 3, SubscriptionStatusActive, future, 2, 0, 0)

	repo := &resetAllQuotaRepoStub{}
	svc := NewSubscriptionService(nil, repo, nil, s.client, nil)
	s.T().Cleanup(svc.Stop)

	res, err := svc.ResetAllQuotaBatch(ctx, true, false, false, 100)
	require.NoError(s.T(), err)
	require.Equal(s.T(), 2, res.Processed)
	require.Equal(s.T(), 2, res.SuccessCount)
	require.Equal(s.T(), 0, res.FailedCount)
	require.Empty(s.T(), res.Failures)
	// 1 号与 3 号都被重置，2 号（无用量）不参与
	require.Len(s.T(), repo.resetIDs, 2)
}

// 单条重置失败时记入失败明细，不中断整批。
func TestResetAllQuotaBatchByIDs_RecordsFailures(t *testing.T) {
	repo := &resetAllQuotaRepoStub{
		failIDs: map[int64]error{2: context.DeadlineExceeded},
	}
	svc := NewSubscriptionService(nil, repo, nil, nil, nil)
	t.Cleanup(svc.Stop)

	res, err := svc.ResetAllQuotaBatchByIDs(context.Background(), []int64{1, 2, 3}, true, false, false)
	require.NoError(t, err)
	require.Equal(t, 3, res.Processed)
	require.Equal(t, 2, res.SuccessCount)
	require.Equal(t, 1, res.FailedCount)
	require.Len(t, res.Failures, 1)
	require.Equal(t, int64(2), res.Failures[0].SubscriptionID)
	require.NotEmpty(t, res.Failures[0].Error)
	// 失败项不计入成功
	require.Len(t, repo.resetIDs, 2)
}

// 边界：空 ID 集合、超过单次上限、全不勾选 → 均报错。
func TestResetAllQuotaBatchByIDs_Validation(t *testing.T) {
	repo := &resetAllQuotaRepoStub{}
	svc := NewSubscriptionService(nil, repo, nil, nil, nil)
	t.Cleanup(svc.Stop)

	_, err := svc.ResetAllQuotaBatchByIDs(context.Background(), nil, true, false, false)
	require.Error(t, err)

	tooMany := make([]int64, MaxBulkSubscriptionActions+1)
	for i := range tooMany {
		tooMany[i] = int64(i + 1)
	}
	_, err = svc.ResetAllQuotaBatchByIDs(context.Background(), tooMany, true, false, false)
	require.Error(t, err)

	_, err = svc.ResetAllQuotaBatchByIDs(context.Background(), []int64{1}, false, false, false)
	require.Error(t, err)
}

// 分批上限：limit 传 0 时收敛到 MaxBulkSubscriptionActions，仍能正常执行。
func (s *ResetAllQuotaScopeTestSuite) TestBatchClampsLimit() {
	ctx := context.Background()
	future := time.Now().Add(24 * time.Hour)
	seedSub(s.T(), s.client, 1, SubscriptionStatusActive, future, 1, 0, 0)

	repo := &resetAllQuotaRepoStub{}
	svc := NewSubscriptionService(nil, repo, nil, s.client, nil)
	s.T().Cleanup(svc.Stop)

	res, err := svc.ResetAllQuotaBatch(ctx, true, false, false, 0)
	require.NoError(s.T(), err)
	require.Equal(s.T(), 1, res.Processed)
}

// 软删（revoked 展示态）的订阅不命中口径。
func (s *ResetAllQuotaScopeTestSuite) TestSoftDeletedExcluded() {
	ctx := context.Background()
	future := time.Now().Add(24 * time.Hour)
	seedSub(s.T(), s.client, 8, SubscriptionStatusActive, future, 7.5, 0, 0)

	sub, err := s.client.UserSubscription.Query().Only(ctx)
	require.NoError(s.T(), err)
	_, err = s.client.UserSubscription.UpdateOne(sub).SetDeletedAt(time.Now()).Save(ctx)
	require.NoError(s.T(), err)

	svc := s.newService()
	n, err := svc.ResetAllQuotaPreview(ctx, true, false, false)
	require.NoError(s.T(), err)
	require.Equal(s.T(), 0, n)
}

// 编译期保证：ent 生成的 usersubscription 谓词确实存在。
var _ = usersubscription.DailyUsageUsdGT(0)
