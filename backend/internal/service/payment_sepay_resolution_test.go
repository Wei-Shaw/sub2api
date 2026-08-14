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

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/enttest"
	"github.com/Wei-Shaw/sub2api/internal/payment"

	"github.com/stretchr/testify/require"
)

func newSepayResolutionTestClient(t *testing.T) *dbent.Client {
	t.Helper()
	db, err := sql.Open("sqlite", "file:sepay_resolution_"+strconv.FormatInt(time.Now().UnixNano(), 10)+"?mode=memory&_fk=1")
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	_, err = db.Exec("PRAGMA foreign_keys = ON")
	require.NoError(t, err)
	drv := entsql.OpenDB(dialect.SQLite, db)
	client := enttest.NewClient(t, enttest.WithOptions(dbent.Driver(drv)))
	t.Cleanup(func() { _ = client.Close() })
	return client
}

func createSepayTestOrder(t *testing.T, ctx context.Context, client *dbent.Client, outTradeNo string) *dbent.PaymentOrder {
	t.Helper()
	user, err := client.User.Create().
		SetEmail("sepay-" + strconv.FormatInt(time.Now().UnixNano(), 10) + "@example.com").
		SetPasswordHash("hash").
		SetUsername("sepay-user").
		Save(ctx)
	require.NoError(t, err)
	order, err := client.PaymentOrder.Create().
		SetUserID(user.ID).
		SetUserEmail(user.Email).
		SetUserName(user.Username).
		SetAmount(50000).
		SetPayAmount(50000).
		SetFeeRate(0).
		SetRechargeCode("PAY-SEPAY-" + strconv.FormatInt(time.Now().UnixNano(), 10)).
		SetOutTradeNo(outTradeNo).
		SetPaymentTradeNo(outTradeNo).
		SetPaymentType(payment.TypeSePay).
		SetOrderType(payment.OrderTypeBalance).
		SetStatus(OrderStatusPending).
		SetExpiresAt(time.Now().Add(time.Hour)).
		SetClientIP("127.0.0.1").
		SetSrcHost("api.example.com").
		SetSrcURL("/api/v1/payment/orders").
		Save(ctx)
	require.NoError(t, err)
	return order
}

// TestResolveSepayOutTradeNo verifies bank-side mutations of the transfer code:
// exact, uppercased, prefix-stripped and prefix-stripped+uppercased variants
// all resolve to the canonical out_trade_no.
func TestResolveSepayOutTradeNo(t *testing.T) {
	ctx := context.Background()
	client := newSepayResolutionTestClient(t)
	svc := &PaymentService{entClient: client, providersLoaded: true}

	const canonical = "sub2_20260814aB3kX9mQ"
	order := createSepayTestOrder(t, ctx, client, canonical)

	for _, code := range []string{
		"sub2_20260814aB3kX9mQ",
		"SUB2_20260814AB3KX9MQ",
		"20260814aB3kX9mQ",
		"20260814AB3KX9MQ",
	} {
		require.Equal(t, canonical, svc.resolveSepayOutTradeNo(ctx, code), "code %q", code)
	}
	require.Equal(t, "sub2_19990101zzzzzzzz",
		svc.resolveSepayOutTradeNo(ctx, "sub2_19990101zzzzzzzz"), "unknown code round-trips unchanged")
	require.Equal(t, "", svc.resolveSepayOutTradeNo(ctx, "  "))

	oid, ok := svc.resolveSepayNotificationOrderID(ctx, payment.TypeSePay, "20260814AB3KX9MQ")
	require.True(t, ok)
	require.Equal(t, order.ID, oid)

	_, ok = svc.resolveSepayNotificationOrderID(ctx, payment.TypeAlipay, canonical)
	require.False(t, ok, "non-sepay provider must not use sepay resolution")
}

func TestPaymentOrderQueryReferenceSePay(t *testing.T) {
	order := &dbent.PaymentOrder{OutTradeNo: "sub2_20260814aB3kX9mQ", PaymentType: payment.TypeSePay, PaymentTradeNo: "sepay-upstream-trade-no"}
	require.Equal(t, "sub2_20260814aB3kX9mQ", paymentOrderQueryReference(order, nil),
		"sepay must query by out_trade_no (no upstream tradeNo exists while pending)")
}
