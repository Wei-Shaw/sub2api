//go:build unit

package service

import (
	"context"
	"strconv"
	"testing"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/payment"
	"github.com/stretchr/testify/require"
)

func TestBuildPaymentOrderProviderSnapshot_ExcludesSensitiveConfig(t *testing.T) {
	t.Parallel()

	sel := &payment.InstanceSelection{
		InstanceID:     "12",
		ProviderKey:    payment.TypeGPMPay,
		SupportedTypes: payment.TypeGPMPayBankTransfer,
		PaymentMode:    "popup",
		Config: map[string]string{
			"apiToken":      "token",
			"webhookSecret": "secret",
			"bankBin":       "970422",
			"accountNumber": "0123456789",
			// GPM Pay settles in VND only; a stray currency key must not leak in.
			"currency": "USD",
		},
	}

	snapshot := buildPaymentOrderProviderSnapshot(sel, CreateOrderRequest{})
	require.Equal(t, map[string]any{
		"schema_version":       2,
		"provider_instance_id": "12",
		"provider_key":         payment.TypeGPMPay,
		"payment_mode":         "popup",
		"currency":             "VND",
	}, snapshot)
	require.NotContains(t, snapshot, "config")
	require.NotContains(t, snapshot, "apiToken")
	require.NotContains(t, snapshot, "webhookSecret")
	require.NotContains(t, snapshot, "supported_types")
	require.NotContains(t, snapshot, "instance_name")
}

func TestBuildPaymentOrderProviderSnapshot_RecordsNowPaymentsCurrency(t *testing.T) {
	t.Parallel()

	// 回归：NOWPayments 按 USD 计价，快照缺 currency 时统计会把它当成 VND。
	snapshot := buildPaymentOrderProviderSnapshot(&payment.InstanceSelection{
		InstanceID:  "7",
		ProviderKey: payment.TypeNowPayments,
		Config:      map[string]string{"apiKey": "k", "currency": "usd"},
	}, CreateOrderRequest{})
	require.Equal(t, "USD", snapshot["currency"])

	order := &dbent.PaymentOrder{ProviderSnapshot: snapshot}
	require.Equal(t, "USD", PaymentOrderCurrency(order))
}

func TestCreateOrderInTx_WritesProviderSnapshot(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)

	user, err := client.User.Create().
		SetEmail("snapshot@example.com").
		SetPasswordHash("hash").
		SetUsername("snapshot-user").
		Save(ctx)
	require.NoError(t, err)

	instance, err := client.PaymentProviderInstance.Create().
		SetProviderKey(payment.TypeGPMPayBankTransfer).
		SetName("Primary Alipay").
		SetConfig(`{"secretKey":"do-not-copy"}`).
		SetSupportedTypes("alipay,alipay_direct").
		SetPaymentMode("redirect").
		SetEnabled(true).
		Save(ctx)
	require.NoError(t, err)

	svc := &PaymentService{entClient: client}
	order, err := svc.createOrderInTx(
		ctx,
		CreateOrderRequest{
			UserID:      user.ID,
			PaymentType: payment.TypeGPMPayBankTransfer,
			OrderType:   payment.OrderTypeBalance,
			ClientIP:    "127.0.0.1",
			SrcHost:     "app.example.com",
		},
		&User{
			ID:       user.ID,
			Email:    user.Email,
			Username: user.Username,
		},
		nil,
		&PaymentConfig{
			MaxPendingOrders: 3,
			OrderTimeoutMin:  30,
		},
		88,
		0,
		88,
		0,
		&payment.InstanceSelection{
			InstanceID:     strconv.FormatInt(instance.ID, 10),
			ProviderKey:    payment.TypeGPMPayBankTransfer,
			SupportedTypes: "alipay,alipay_direct",
			PaymentMode:    "redirect",
			Config: map[string]string{
				"secretKey": "do-not-copy",
			},
		},
	)
	require.NoError(t, err)
	require.Equal(t, strconv.FormatInt(instance.ID, 10), valueOrEmpty(order.ProviderInstanceID))
	require.Equal(t, payment.TypeGPMPayBankTransfer, valueOrEmpty(order.ProviderKey))
	// createOrderInTx 不再回读订单（少一条 UPDATE），返回的是内存实体，
	// schema_version 仍是 int；回读后才会变成 JSON 解码出的 float64。
	// 生产读取方 psSnapshotIntValue 两种类型都接受，这里按值断言。
	require.Equal(t, 2, psSnapshotIntValue(order.ProviderSnapshot["schema_version"]))
	require.Equal(t, strconv.FormatInt(instance.ID, 10), order.ProviderSnapshot["provider_instance_id"])
	require.Equal(t, payment.TypeGPMPayBankTransfer, order.ProviderSnapshot["provider_key"])
	require.Equal(t, "redirect", order.ProviderSnapshot["payment_mode"])
	require.NotContains(t, order.ProviderSnapshot, "config")
	require.NotContains(t, order.ProviderSnapshot, "secretKey")
	require.NotContains(t, order.ProviderSnapshot, "supported_types")
	require.NotContains(t, order.ProviderSnapshot, "instance_name")
}

func valueOrEmpty(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}
