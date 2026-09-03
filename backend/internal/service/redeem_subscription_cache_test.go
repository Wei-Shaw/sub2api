//go:build unit

package service

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

type redeemCacheOrderSubRepo struct {
	userSubRepoNoop

	sub            UserSubscription
	getActiveCalls int
}

func (r *redeemCacheOrderSubRepo) GetActiveByUserIDAndGroupID(_ context.Context, userID, groupID int64) (*UserSubscription, error) {
	r.getActiveCalls++
	if r.sub.UserID != userID || r.sub.GroupID != groupID {
		return nil, ErrSubscriptionNotFound
	}
	cp := r.sub
	return &cp, nil
}

func (r *redeemCacheOrderSubRepo) GetByUserIDAndGroupID(_ context.Context, userID, groupID int64) (*UserSubscription, error) {
	if r.sub.UserID != userID || r.sub.GroupID != groupID {
		return nil, ErrSubscriptionNotFound
	}
	cp := r.sub
	return &cp, nil
}

func (r *redeemCacheOrderSubRepo) GetByID(_ context.Context, _ int64) (*UserSubscription, error) {
	cp := r.sub
	return &cp, nil
}

func (r *redeemCacheOrderSubRepo) GetByIDForUpdate(_ context.Context, _ int64) (*UserSubscription, error) {
	cp := r.sub
	return &cp, nil
}

func (r *redeemCacheOrderSubRepo) ExtendExpiry(_ context.Context, _ int64, expiresAt time.Time) error {
	r.sub.ExpiresAt = expiresAt
	return nil
}

func (r *redeemCacheOrderSubRepo) UpdateNotes(_ context.Context, _ int64, notes string) error {
	r.sub.Notes = notes
	return nil
}

func (r *redeemCacheOrderSubRepo) UpdateStatus(_ context.Context, _ int64, status string) error {
	r.sub.Status = status
	return nil
}

// M12：兑换路径必须在事务提交后才失效订阅缓存。
// 事务进行中（deferCacheInvalidation=true）不能动 L1，否则并发请求会把
// 提交前的旧行重新灌回缓存；提交后必须真的失效，而不是只失效鉴权缓存。
func TestRedeemSubscriptionCacheInvalidatedAfterCommitNotDuringTx(t *testing.T) {
	ctx := context.Background()
	repo := &redeemCacheOrderSubRepo{sub: UserSubscription{
		ID: 7, UserID: 11, GroupID: 13,
		ExpiresAt: time.Date(2030, 6, 15, 12, 0, 0, 0, time.UTC),
		Status:    SubscriptionStatusActive,
	}}
	groupRepo := &subscriptionGroupRepoStub{group: &Group{
		ID:               13,
		SubscriptionType: SubscriptionTypeSubscription,
		Status:           StatusActive,
	}}
	subSvc := NewSubscriptionService(groupRepo, repo, nil, nil, &config.Config{
		SubscriptionCache: config.SubscriptionCacheConfig{L1Size: 4096, L1TTLSeconds: 60},
	})
	t.Cleanup(subSvc.Stop)
	require.NotNil(t, subSvc.subCacheL1)
	// ristretto 在极小容量下会直接丢弃写入，容量要给足才能真实观察失效行为。

	redeemSvc := &RedeemService{subscriptionService: subSvc}

	// 预热 L1
	_, err := subSvc.GetActiveSubscription(ctx, 11, 13)
	require.NoError(t, err)
	subSvc.subCacheL1.Wait()
	require.Equal(t, 1, repo.getActiveCalls)

	// 事务进行中：延后失效，L1 必须保持原样。
	_, _, err = subSvc.assignOrExtendSubscription(ctx, &AssignSubscriptionInput{
		UserID:       11,
		GroupID:      13,
		ValidityDays: 30,
		Notes:        "通过兑换码 CODE 兑换",
	}, true)
	require.NoError(t, err)
	subSvc.subCacheL1.Wait()

	_, err = subSvc.GetActiveSubscription(ctx, 11, 13)
	require.NoError(t, err)
	require.Equal(t, 1, repo.getActiveCalls, "事务提交前不应失效 L1")

	// 提交后：invalidateRedeemCaches 必须真正失效订阅缓存。
	groupID := int64(13)
	redeemSvc.invalidateRedeemCaches(ctx, 11, &RedeemCode{Type: RedeemTypeSubscription, GroupID: &groupID})

	_, err = subSvc.GetActiveSubscription(ctx, 11, 13)
	require.NoError(t, err)
	require.Equal(t, 2, repo.getActiveCalls, "提交后必须回源，不能继续命中旧 L1")
}
