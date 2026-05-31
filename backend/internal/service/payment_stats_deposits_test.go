//go:build unit

package service

import (
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/payment"
	"github.com/stretchr/testify/require"
)

func TestGetDashboardStatsIncludesDepositActivity(t *testing.T) {
	client := newPaymentOrderLifecycleTestClient(t)
	ctx := t.Context()
	now := time.Now()

	target, err := client.User.Create().
		SetEmail("deposit-target@example.com").
		SetUsername("deposit-target").
		SetPasswordHash("hash").
		Save(ctx)
	require.NoError(t, err)

	admin, err := client.User.Create().
		SetEmail("admin-depositor@example.com").
		SetUsername("admin").
		SetPasswordHash("hash").
		SetRole(RoleAdmin).
		Save(ctx)
	require.NoError(t, err)

	autoTarget, err := client.User.Create().
		SetEmail("auto-target@example.com").
		SetUsername("auto-target").
		SetPasswordHash("hash").
		Save(ctx)
	require.NoError(t, err)

	group, err := client.Group.Create().
		SetName("OpenAI Subscription").
		SetPlatform(PlatformOpenAI).
		SetSubscriptionType(SubscriptionTypeSubscription).
		Save(ctx)
	require.NoError(t, err)

	_, err = client.PaymentOrder.Create().
		SetUserID(target.ID).
		SetUserEmail(target.Email).
		SetUserName(target.Username).
		SetAmount(10).
		SetPayAmount(10).
		SetPaymentAmount(10).
		SetPaymentCurrency("USD").
		SetLedgerAmount(10).
		SetLedgerCurrency("USD").
		SetFeeRate(0).
		SetRechargeCode("BALANCE-PAID").
		SetOutTradeNo("sub2_balance_paid").
		SetPaymentType(payment.TypeSepay).
		SetPaymentTradeNo("trade-balance").
		SetClientIP("127.0.0.1").
		SetSrcHost("test.local").
		SetOrderType(payment.OrderTypeBalance).
		SetActualCredits(5000).
		SetStatus(OrderStatusCompleted).
		SetExpiresAt(now.Add(time.Hour)).
		SetPaidAt(now.Add(-4 * time.Hour)).
		Save(ctx)
	require.NoError(t, err)

	_, err = client.RedeemCode.Create().
		SetCode("ADMIN-BALANCE").
		SetType(AdjustmentTypeAdminBalance).
		SetValue(7).
		SetStatus(StatusUsed).
		SetUsedBy(target.ID).
		SetUsedAt(now.Add(-3 * time.Hour)).
		SetCreatedBy(admin.ID).
		Save(ctx)
	require.NoError(t, err)

	_, err = client.UserSubscription.Create().
		SetUserID(target.ID).
		SetGroupID(group.ID).
		SetStartsAt(now.Add(-2 * time.Hour)).
		SetExpiresAt(now.AddDate(0, 0, 30)).
		SetStatus(SubscriptionStatusActive).
		SetAssignedAt(now.Add(-2 * time.Hour)).
		SetAssignedBy(admin.ID).
		SetNotes("manual admin grant").
		Save(ctx)
	require.NoError(t, err)

	_, err = client.UserSubscription.Create().
		SetUserID(autoTarget.ID).
		SetGroupID(group.ID).
		SetStartsAt(now.Add(-1 * time.Hour)).
		SetExpiresAt(now.AddDate(0, 0, 14)).
		SetStatus(SubscriptionStatusActive).
		SetAssignedAt(now.Add(-1 * time.Hour)).
		SetNotes("auto assigned default subscription").
		Save(ctx)
	require.NoError(t, err)

	svc := &PaymentService{entClient: client}
	stats, err := svc.GetDashboardStats(ctx, 7)
	require.NoError(t, err)

	require.Equal(t, 4, stats.Deposits.TotalEvents)
	require.Equal(t, CurrencyAmounts{"USD": 17}, stats.Deposits.TotalLedgerAmount)
	require.Equal(t, 5007.0, stats.Deposits.TotalCredits)
	require.Equal(t, 1, stats.Deposits.PaidTopups)
	require.Equal(t, 1, stats.Deposits.AdminAdjustments)
	require.Equal(t, 1, stats.Deposits.ManualAssignments)
	require.Equal(t, 1, stats.Deposits.AutoAssignments)
	require.Equal(t, 2, stats.Deposits.SubscriptionAssignments)
	require.Len(t, stats.Deposits.TopRecipients, 2)
	require.Equal(t, target.ID, stats.Deposits.TopRecipients[0].UserID)
	require.Equal(t, target.Email, stats.Deposits.TopRecipients[0].Email)
	require.Len(t, stats.Deposits.RecentEvents, 4)
	require.Equal(t, depositSourceAutoSubscriptionAssign, stats.Deposits.RecentEvents[0].Source)
}

func TestDepositAccumulatorSeparatesLedgerCurrencies(t *testing.T) {
	now := time.Now()
	acc := newDepositAccumulator()

	acc.add(DepositEventStat{
		Source:       depositSourcePaidBalanceOrder,
		UserID:       11,
		LedgerAmount: 10,
		Currency:     "usd",
		OccurredAt:   now,
	})
	acc.add(DepositEventStat{
		Source:       depositSourcePaidBalanceOrder,
		UserID:       11,
		LedgerAmount: 70,
		Currency:     "CNY",
		OccurredAt:   now.Add(time.Minute),
	})
	acc.add(DepositEventStat{
		Source:       depositSourcePaidBalanceOrder,
		UserID:       22,
		LedgerAmount: 5,
		Currency:     "USD",
		OccurredAt:   now.Add(2 * time.Minute),
	})

	stats := acc.finalize()

	require.Equal(t, CurrencyAmounts{"CNY": 70, "USD": 15}, stats.TotalLedgerAmount)
	require.Len(t, stats.BySource, 1)
	require.Equal(t, CurrencyAmounts{"CNY": 70, "USD": 15}, stats.BySource[0].LedgerAmount)
	require.Len(t, stats.TopRecipients, 2)
	require.Equal(t, int64(11), stats.TopRecipients[0].UserID)
	require.Equal(t, CurrencyAmounts{"CNY": 70, "USD": 10}, stats.TopRecipients[0].LedgerAmount)
	require.Equal(t, "USD", stats.RecentEvents[2].Currency)
}
