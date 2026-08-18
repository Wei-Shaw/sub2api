//go:build unit

package service

import (
	"context"
	"database/sql"
	"strconv"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	_ "modernc.org/sqlite"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/enttest"
	"github.com/Wei-Shaw/sub2api/internal/payment"
	"github.com/stretchr/testify/require"
)

func TestPaymentRecoveryGraceWidensForInfini(t *testing.T) {
	t.Parallel()

	require.Equal(t, infiniLatePaymentGrace, paymentRecoveryGrace(payment.TypeInfini))
	require.Equal(t, infiniLatePaymentGrace, paymentRecoveryGrace(" Infini "))
	require.Equal(t, paymentGraceMinutes*time.Minute, paymentRecoveryGrace(payment.TypeAlipay))
	require.Equal(t, paymentGraceMinutes*time.Minute, paymentRecoveryGrace(""))
	require.Greater(t, paymentRecoveryGrace(payment.TypeInfini), paymentRecoveryGrace(payment.TypeStripe))
}

func TestInstanceForwardsPayerEmail(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		config map[string]string
		want   bool
	}{
		{"unset defaults to on", map[string]string{}, true},
		{"nil config defaults to on", nil, true},
		{"explicit true", map[string]string{"forwardPayerEmail": "true"}, true},
		{"explicit false", map[string]string{"forwardPayerEmail": "false"}, false},
		{"case-insensitive key", map[string]string{"forwardpayeremail": "false"}, false},
		{"padded value", map[string]string{"forwardPayerEmail": " false "}, false},
		{"unparsable value defaults to on", map[string]string{"forwardPayerEmail": "maybe"}, true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.want, instanceForwardsPayerEmail(tc.config))
		})
	}
}

func TestProviderPayerEmail(t *testing.T) {
	t.Parallel()

	order := &dbent.PaymentOrder{UserEmail: " payer@example.com "}
	sel := &payment.InstanceSelection{Config: map[string]string{}}
	require.Equal(t, "payer@example.com", providerPayerEmail(order, sel))

	off := &payment.InstanceSelection{Config: map[string]string{"forwardPayerEmail": "false"}}
	require.Empty(t, providerPayerEmail(order, off))
	require.Empty(t, providerPayerEmail(nil, sel))
	require.Empty(t, providerPayerEmail(order, nil))
}

func TestProviderExpiresInSeconds(t *testing.T) {
	t.Parallel()

	now := time.Now()
	require.Equal(t, 1800, providerExpiresInSeconds(&dbent.PaymentOrder{ExpiresAt: now.Add(30 * time.Minute)}, now))
	require.Zero(t, providerExpiresInSeconds(&dbent.PaymentOrder{ExpiresAt: now.Add(-time.Minute)}, now))
	require.Zero(t, providerExpiresInSeconds(&dbent.PaymentOrder{}, now))
	require.Zero(t, providerExpiresInSeconds(nil, now))
}

// TestRecordPaymentAnomalyLeavesOrderUntouched pins the underpayment contract:
// the order keeps its status (FAILED would expose it to the admin recharge
// retry, which credits balance for money we never fully received) and the
// event survives as an audit log.
func TestRecordPaymentAnomalyLeavesOrderUntouched(t *testing.T) {
	ctx := context.Background()
	client := newInfiniAnomalyTestClient(t)

	user, err := client.User.Create().
		SetEmail("infini-" + strconv.FormatInt(time.Now().UnixNano(), 10) + "@example.com").
		SetPasswordHash("hash").
		SetUsername("payment-infini-user").
		Save(ctx)
	require.NoError(t, err)

	outTradeNo := "sub2_infini_" + strconv.FormatInt(time.Now().UnixNano(), 10)
	order, err := client.PaymentOrder.Create().
		SetUserID(user.ID).
		SetUserEmail(user.Email).
		SetUserName(user.Username).
		SetAmount(100).
		SetPayAmount(100).
		SetFeeRate(0).
		SetRechargeCode("PAY-INFINI-" + strconv.FormatInt(time.Now().UnixNano(), 10)).
		SetOutTradeNo(outTradeNo).
		SetPaymentType(payment.TypeInfini).
		SetPaymentTradeNo("ord_1").
		SetProviderKey(payment.TypeInfini).
		SetOrderType(payment.OrderTypeBalance).
		SetStatus(OrderStatusExpired).
		SetExpiresAt(time.Now().Add(-time.Hour)).
		SetClientIP("127.0.0.1").
		SetSrcHost("api.example.com").
		Save(ctx)
	require.NoError(t, err)

	svc := &PaymentService{entClient: client, providersLoaded: true}
	notification := &payment.PaymentNotification{
		OrderID: outTradeNo,
		TradeNo: "ord_1",
		Status:  payment.ProviderStatusFailed,
		Amount:  100,
		Anomaly: payment.NotificationAnomalyPartialPaid,
		Metadata: map[string]string{
			"currency":         "USD",
			"status":           "partial_paid",
			"amount_confirmed": "40",
			"exception_tags":   "late",
		},
	}
	require.NoError(t, svc.HandlePaymentNotification(ctx, notification, payment.TypeInfini))

	reloaded, err := client.PaymentOrder.Get(ctx, order.ID)
	require.NoError(t, err)
	require.Equal(t, OrderStatusExpired, reloaded.Status, "an underpaid order must not become retryable")

	logs, err := svc.GetOrderAuditLogs(ctx, order.ID)
	require.NoError(t, err)
	require.Len(t, logs, 1)
	require.Equal(t, "PAYMENT_PARTIAL_PAID", logs[0].Action)
	require.Contains(t, logs[0].Detail, "amount_confirmed")
	require.Contains(t, logs[0].Detail, "40")
}

// TestRecordPaymentAnomalyUnknownOrderReturnsSentinel keeps unknown orders on
// the ack-and-stop-retrying path the webhook handler relies on.
func TestRecordPaymentAnomalyUnknownOrderReturnsSentinel(t *testing.T) {
	ctx := context.Background()
	client := newInfiniAnomalyTestClient(t)

	svc := &PaymentService{entClient: client, providersLoaded: true}
	err := svc.HandlePaymentNotification(ctx, &payment.PaymentNotification{
		OrderID: "sub2_infini_missing",
		Status:  payment.ProviderStatusFailed,
		Anomaly: payment.NotificationAnomalyPartialPaid,
	}, payment.TypeInfini)
	require.ErrorIs(t, err, ErrOrderNotFound)
}

// TestHandlePaymentNotificationIgnoresFailureWithoutAnomaly guards the existing
// short-circuit: plain failures (e.g. Airwallex cancellations) still skip the
// DB entirely.
func TestHandlePaymentNotificationIgnoresFailureWithoutAnomaly(t *testing.T) {
	ctx := context.Background()
	client := newInfiniAnomalyTestClient(t)

	svc := &PaymentService{entClient: client, providersLoaded: true}
	require.NoError(t, svc.HandlePaymentNotification(ctx, &payment.PaymentNotification{
		OrderID: "sub2_infini_missing",
		Status:  payment.ProviderStatusFailed,
	}, payment.TypeAirwallex))
}

func TestPaymentProviderConfigCurrencyForInfini(t *testing.T) {
	t.Parallel()

	require.Equal(t, "USD", paymentProviderConfigCurrency(payment.TypeInfini, map[string]string{"currency": "USD"}))
	require.Equal(t, "HKD", paymentProviderConfigCurrency(payment.TypeInfini, map[string]string{"currency": "hkd"}))
	// An unset currency falls back to the platform default here; NewInfini is
	// what normalises a stored instance to USD.
	require.Equal(t, payment.DefaultPaymentCurrency, paymentProviderConfigCurrency(payment.TypeInfini, map[string]string{}))
}

func TestProviderSupportsRefund(t *testing.T) {
	t.Parallel()

	require.False(t, payment.ProviderSupportsRefund(payment.TypeInfini))
	require.False(t, payment.ProviderSupportsRefund("  Infini  "))
	require.True(t, payment.ProviderSupportsRefund(payment.TypeStripe))
	require.True(t, payment.ProviderSupportsRefund(payment.TypeAirwallex))
	require.True(t, payment.ProviderSupportsRefund(payment.TypeEasyPay))
}

// TestPrepareRefundRejectsInfiniEvenWhenEnabled covers the stored-state case:
// an instance whose refund_enabled was switched on before the capability check
// existed must still be refused, before any order state changes.
func TestPrepareRefundRejectsInfiniEvenWhenEnabled(t *testing.T) {
	ctx := context.Background()
	client := newInfiniAnomalyTestClient(t)

	inst, err := client.PaymentProviderInstance.Create().
		SetProviderKey(payment.TypeInfini).
		SetName("infini").
		SetConfig("{}").
		SetSupportedTypes(payment.TypeInfini).
		SetEnabled(true).
		SetRefundEnabled(true).
		SetAllowUserRefund(true).
		Save(ctx)
	require.NoError(t, err)

	user, err := client.User.Create().
		SetEmail("refund-" + strconv.FormatInt(time.Now().UnixNano(), 10) + "@example.com").
		SetPasswordHash("hash").
		SetUsername("payment-infini-refund").
		Save(ctx)
	require.NoError(t, err)

	order, err := client.PaymentOrder.Create().
		SetUserID(user.ID).
		SetUserEmail(user.Email).
		SetUserName(user.Username).
		SetAmount(10).
		SetPayAmount(10).
		SetFeeRate(0).
		SetRechargeCode("PAY-INFINI-R-" + strconv.FormatInt(time.Now().UnixNano(), 10)).
		SetOutTradeNo("sub2_infini_refund_" + strconv.FormatInt(time.Now().UnixNano(), 10)).
		SetPaymentType(payment.TypeInfini).
		SetPaymentTradeNo("ord_r1").
		SetProviderKey(payment.TypeInfini).
		SetProviderInstanceID(strconv.FormatInt(inst.ID, 10)).
		SetOrderType(payment.OrderTypeBalance).
		SetStatus(OrderStatusCompleted).
		SetExpiresAt(time.Now().Add(time.Hour)).
		SetClientIP("127.0.0.1").
		SetSrcHost("api.example.com").
		Save(ctx)
	require.NoError(t, err)

	// The user-facing refund button is driven by this list; an Infini instance
	// must never appear in it, whatever its stored flags say.
	cfgSvc := &PaymentConfigService{entClient: client}
	ids, err := cfgSvc.GetUserRefundEligibleInstanceIDs(ctx)
	require.NoError(t, err)
	require.NotContains(t, ids, strconv.FormatInt(inst.ID, 10))

	svc := &PaymentService{entClient: client, providersLoaded: true}

	_, _, err = svc.PrepareRefund(ctx, order.ID, 10, "test", false, false)
	require.ErrorContains(t, err, "PROVIDER_REFUND_UNSUPPORTED")

	_, err = svc.validateRefundRequest(ctx, order.ID, user.ID)
	require.ErrorContains(t, err, "PROVIDER_REFUND_UNSUPPORTED")

	reloaded, err := client.PaymentOrder.Get(ctx, order.ID)
	require.NoError(t, err)
	require.Equal(t, OrderStatusCompleted, reloaded.Status, "a rejected refund must not touch the order")
}

func newInfiniAnomalyTestClient(t *testing.T) *dbent.Client {
	t.Helper()

	db, err := sql.Open("sqlite", "file:payment_infini_anomaly?mode=memory&cache=shared&_fk=1")
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	_, err = db.Exec("PRAGMA foreign_keys = ON")
	require.NoError(t, err)

	drv := entsql.OpenDB(dialect.SQLite, db)
	client := enttest.NewClient(t, enttest.WithOptions(dbent.Driver(drv)))
	t.Cleanup(func() { _ = client.Close() })
	return client
}
