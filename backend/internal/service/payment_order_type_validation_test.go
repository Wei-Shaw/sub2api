//go:build unit

package service

import (
	"context"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/payment"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/stretchr/testify/require"
)

// Any order_type other than balance/subscription used to skip the
// balance-only checks (BalanceDisabled, recharge multiplier, pay_amount daily
// limit) while still being fulfilled as a balance top-up.
func TestValidateOrderInputRejectsUnknownOrderType(t *testing.T) {
	svc := &PaymentService{}
	cfg := &PaymentConfig{BalanceDisabled: true, MinAmount: 1}

	for _, orderType := range []string{"topup", "Balance", "BALANCE", " balance", "recharge"} {
		t.Run(orderType, func(t *testing.T) {
			_, err := svc.validateOrderInput(context.Background(), CreateOrderRequest{OrderType: orderType, Amount: 10}, cfg)
			require.Error(t, err)
			require.Equal(t, "INVALID_ORDER_TYPE", infraerrors.Reason(err))
		})
	}
}

func TestValidateOrderInputKeepsBalanceRules(t *testing.T) {
	svc := &PaymentService{}

	_, err := svc.validateOrderInput(context.Background(), CreateOrderRequest{OrderType: payment.OrderTypeBalance, Amount: 10}, &PaymentConfig{BalanceDisabled: true})
	require.Error(t, err)
	require.Equal(t, "BALANCE_PAYMENT_DISABLED", infraerrors.Reason(err))

	plan, err := svc.validateOrderInput(context.Background(), CreateOrderRequest{OrderType: payment.OrderTypeBalance, Amount: 10}, &PaymentConfig{MinAmount: 1, MaxAmount: 100})
	require.NoError(t, err)
	require.Nil(t, plan)
}
