package handler

import (
	"encoding/json"
	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestPaymentOrderResponsesPreservePurchasedPlanSnapshot(t *testing.T) {
	days := 30
	order := &dbent.PaymentOrder{Amount: 100, PayAmount: 672.98, OrderType: "subscription", SubscriptionDays: &days,
		SubscriptionPlanSnapshot: map[string]string{"name": "Purchased plan", "currency": "USD"},
		ProviderSnapshot:         map[string]any{"currency": "CNY"}}
	for _, result := range []any{sanitizePaymentOrderForResponse(order), buildPublicOrderResult(order)} {
		body, err := json.Marshal(result)
		require.NoError(t, err)
		var got map[string]any
		require.NoError(t, json.Unmarshal(body, &got))
		require.Equal(t, map[string]any{"name": "Purchased plan", "currency": "USD"}, got["subscription_plan_snapshot"])
		require.Equal(t, float64(30), got["subscription_days"])
		require.Equal(t, "CNY", got["currency"])
	}
	anonymous, err := json.Marshal(buildPublicOrderVerifyResult(order))
	require.NoError(t, err)
	require.NotContains(t, string(anonymous), "subscription_plan_snapshot")
	require.NotContains(t, string(anonymous), "subscription_days")
	require.NotContains(t, string(anonymous), "Purchased plan")
	order.SubscriptionPlanSnapshot = nil
	require.Nil(t, sanitizePaymentOrderForResponse(order).SubscriptionPlanSnapshot)
	require.Nil(t, buildPublicOrderResult(order).SubscriptionPlanSnapshot)
}
