//go:build unit

package service

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/payment"
	"github.com/stretchr/testify/require"
)

func TestComputeSubscriptionValidityDays(t *testing.T) {
	t.Parallel()

	base := time.Date(2026, time.January, 31, 10, 0, 0, 0, time.UTC)

	require.Equal(t, 15, computeSubscriptionValidityDays(15, "day", base))
	require.Equal(t, 14, computeSubscriptionValidityDays(2, "week", base))
	require.Equal(t, 31, computeSubscriptionValidityDays(1, "month", base))
}

func TestComputeSubscriptionValidityDuration_AllowsFractionalDays(t *testing.T) {
	t.Parallel()

	base := time.Date(2026, time.January, 31, 10, 0, 0, 0, time.UTC)

	days, duration := computeSubscriptionValidity(0.5, "day", base)
	require.Equal(t, 0, days)
	require.Equal(t, 12*time.Hour, duration)

	days, duration = computeSubscriptionValidity(1.5, "week", base)
	require.Equal(t, 10, days)
	require.Equal(t, 12*time.Hour, duration)

	days, duration = computeSubscriptionValidity(1, "month", base)
	require.Equal(t, 31, days)
	require.Zero(t, duration)
}

func TestCreateOrderInTx_StoresFractionalSubscriptionPlanValidity(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)

	user, err := client.User.Create().
		SetEmail("halfday-plan@example.com").
		SetPasswordHash("hash").
		SetUsername("halfday-plan-user").
		SetStatus(StatusActive).
		Save(ctx)
	require.NoError(t, err)

	plan, err := client.SubscriptionPlan.Create().
		SetGroupID(123).
		SetName("Half Day Plan").
		SetDescription("test").
		SetPrice(1.9).
		SetValidityDays(0.5).
		SetValidityUnit("day").
		SetFeatures("[]").
		SetProductName("Half Day").
		SetForSale(true).
		SetSortOrder(1).
		Save(ctx)
	require.NoError(t, err)

	svc := &PaymentService{entClient: client}
	order, err := svc.createOrderInTx(
		ctx,
		CreateOrderRequest{
			UserID:      user.ID,
			PaymentType: payment.TypeAlipay,
			OrderType:   payment.OrderTypeSubscription,
			ClientIP:    "127.0.0.1",
			SrcHost:     "app.example.com",
		},
		&User{ID: user.ID, Email: user.Email, Username: user.Username},
		plan,
		&PaymentConfig{MaxPendingOrders: 3, OrderTimeoutMin: 30},
		1.9,
		1.9,
		0,
		1.9,
		nil,
		ResolvedBalanceTier{},
	)
	require.NoError(t, err)
	require.NotNil(t, order.SubscriptionDays)
	require.Equal(t, 0, *order.SubscriptionDays)
	require.EqualValues(t, 43200, order.ProviderSnapshot["subscription_validity_seconds"])
}

func TestCreateOrderInTx_UsesCalendarMonthValidityForSubscriptionPlan(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)

	user, err := client.User.Create().
		SetEmail("plan@example.com").
		SetPasswordHash("hash").
		SetUsername("plan-user").
		SetStatus(StatusActive).
		Save(ctx)
	require.NoError(t, err)

	plan, err := client.SubscriptionPlan.Create().
		SetGroupID(123).
		SetName("Monthly Plan").
		SetDescription("test").
		SetPrice(9.9).
		SetValidityDays(1).
		SetValidityUnit("month").
		SetFeatures("[]").
		SetProductName("Monthly").
		SetForSale(true).
		SetSortOrder(1).
		Save(ctx)
	require.NoError(t, err)

	svc := &PaymentService{entClient: client}
	base := time.Date(2026, time.January, 31, 10, 0, 0, 0, time.UTC)
	originalNow := paymentNow
	paymentNow = func() time.Time { return base }
	t.Cleanup(func() {
		paymentNow = originalNow
	})

	order, err := svc.createOrderInTx(
		ctx,
		CreateOrderRequest{
			UserID:      user.ID,
			PaymentType: payment.TypeAlipay,
			OrderType:   payment.OrderTypeSubscription,
			ClientIP:    "127.0.0.1",
			SrcHost:     "app.example.com",
		},
		&User{
			ID:       user.ID,
			Email:    user.Email,
			Username: user.Username,
		},
		plan,
		&PaymentConfig{
			MaxPendingOrders: 3,
			OrderTimeoutMin:  30,
		},
		9.9,
		9.9,
		0,
		9.9,
		nil,
		ResolvedBalanceTier{},
	)
	require.NoError(t, err)
	require.NotNil(t, order.SubscriptionDays)
	require.Equal(t, 31, *order.SubscriptionDays)
}
