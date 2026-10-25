//go:build unit

package service

import (
	"context"
	"strconv"
	"testing"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/paymentauditlog"
	"github.com/stretchr/testify/require"
)

type renewalRefundRepo struct{ *lockingRenewalRepo }

func (r *renewalRefundRepo) GetActiveByUserIDAndGroupID(ctx context.Context, _, _ int64) (*UserSubscription, error) {
	return r.GetByID(ctx, r.current.ID)
}

func TestPaymentRenewalResetIsIdempotent(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	ensurePaymentAuditOrderActionUniqueIndex(t, ctx, client)
	now := time.Date(2026, 10, 22, 13, 24, 0, 0, time.UTC)
	user, err := client.User.Create().SetEmail("renewal@example.com").SetPasswordHash("hash").Save(ctx)
	require.NoError(t, err)
	order, err := client.PaymentOrder.Create().SetUserID(user.ID).
		SetUserEmail(user.Email).SetUserName("renewal-test").SetRechargeCode("test-renewal").
		SetPaymentType("alipay").SetPaymentTradeNo("test-trade").SetClientIP("127.0.0.1").SetSrcHost("example.com").
		SetAmount(10).SetPayAmount(10).SetOrderType("subscription").
		SetSubscriptionGroupID(13).SetSubscriptionDays(30).
		SetExpiresAt(now.Add(time.Hour)).Save(ctx)
	require.NoError(t, err)
	current := UserSubscription{ID: 7, UserID: user.ID, GroupID: 13, Status: SubscriptionStatusActive,
		StartsAt: now.AddDate(0, 0, -20), ExpiresAt: now.AddDate(0, 0, 10), MonthlyUsageUSD: 999}
	repo := &lockingRenewalRepo{stale: current, current: current}
	subSvc := NewSubscriptionService(&subscriptionGroupRepoStub{group: &Group{
		ID: 13, SubscriptionType: SubscriptionTypeSubscription, ResetQuotaOnRenewal: true,
	}}, repo, nil, nil, nil)
	subSvc.now = func() time.Time { return now }
	svc := &PaymentService{entClient: client, subscriptionSvc: subSvc}

	require.NoError(t, svc.ensurePaymentSubscriptionAssigned(ctx, order, 13, 30))
	require.Equal(t, now, repo.current.StartsAt)
	require.Equal(t, now.AddDate(0, 0, 30), repo.current.ExpiresAt)
	require.Zero(t, repo.current.MonthlyUsageUSD)
	// A retry after new usage has accrued must not clear that usage or
	// extend/restart the term a second time.
	repo.current.MonthlyUsageUSD = 7
	originalStart, originalExpiry := repo.current.StartsAt, repo.current.ExpiresAt
	now = now.Add(time.Hour)
	require.NoError(t, svc.ensurePaymentSubscriptionAssigned(ctx, order, 13, 30))
	require.Equal(t, originalStart, repo.current.StartsAt)
	require.Equal(t, originalExpiry, repo.current.ExpiresAt)
	require.Equal(t, 7.0, repo.current.MonthlyUsageUSD)
	require.Equal(t, 1, repo.lockReads)
	entries, err := client.PaymentAuditLog.Query().Where(
		paymentauditlog.OrderIDEQ(strconv.FormatInt(order.ID, 10)),
		paymentauditlog.ActionEQ("SUBSCRIPTION_ASSIGNED"),
	).All(ctx)
	require.NoError(t, err)
	require.Len(t, entries, 1)
	require.Contains(t, entries[0].Detail, "subscriptionStartsAt")
}

func TestRefundOlderPurchaseDoesNotDeductReplacementTerm(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	now := time.Date(2026, 10, 22, 13, 24, 0, 0, time.UTC)
	user, err := client.User.Create().SetEmail("old-renewal@example.com").SetPasswordHash("hash").Save(ctx)
	require.NoError(t, err)
	order, err := client.PaymentOrder.Create().SetUserID(user.ID).
		SetUserEmail(user.Email).SetUserName("refund-test").SetRechargeCode("test-refund").
		SetPaymentType("alipay").SetPaymentTradeNo("test-trade").SetClientIP("127.0.0.1").SetSrcHost("example.com").
		SetAmount(10).SetPayAmount(10).SetOrderType("subscription").
		SetSubscriptionGroupID(13).SetSubscriptionDays(30).
		SetCompletedAt(now.AddDate(0, 0, -10)).SetExpiresAt(now.Add(time.Hour)).Save(ctx)
	require.NoError(t, err)
	current := UserSubscription{ID: 7, UserID: user.ID, GroupID: 13, Status: SubscriptionStatusActive,
		StartsAt: now, ExpiresAt: now.AddDate(0, 0, 30)}
	repo := &lockingRenewalRepo{stale: current, current: current}
	svc := &PaymentService{entClient: client, subscriptionSvc: NewSubscriptionService(nil, &renewalRefundRepo{repo}, nil, nil, nil)}
	plan := &RefundPlan{}
	require.Nil(t, svc.prepDeduct(ctx, order, plan, false))
	require.Zero(t, plan.SubDaysToDeduct)
	require.Equal(t, current, repo.current)
}

func TestSubscriptionPlanGroupInfoIncludesRenewalPolicy(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	group, err := client.Group.Create().SetName("Restart on renewal").
		SetSubscriptionType(SubscriptionTypeSubscription).SetResetQuotaOnRenewal(true).Save(ctx)
	require.NoError(t, err)
	plan, err := client.SubscriptionPlan.Create().SetGroupID(group.ID).
		SetName("Monthly").SetPrice(10).Save(ctx)
	require.NoError(t, err)
	svc := &PaymentConfigService{entClient: client}
	info := svc.GetGroupInfoMap(ctx, []*dbent.SubscriptionPlan{plan})
	require.True(t, info[group.ID].ResetQuotaOnRenewal)
}
