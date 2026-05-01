//go:build unit

package service

import (
	"context"
	"database/sql"
	"errors"
	"strconv"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	_ "modernc.org/sqlite"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/enttest"
	"github.com/Wei-Shaw/sub2api/ent/paymentauditlog"
	"github.com/Wei-Shaw/sub2api/internal/payment"
	"github.com/stretchr/testify/require"
)

// newOrderNotFoundTestClient wires an in-memory sqlite-backed ent.Client so
// tests can exercise HandlePaymentNotification's real DB lookup path without
// standing up a service stack.
func newOrderNotFoundTestClient(t *testing.T) *dbent.Client {
	t.Helper()

	db, err := sql.Open("sqlite", "file:payment_order_not_found?mode=memory&cache=shared&_fk=1")
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	_, err = db.Exec("PRAGMA foreign_keys = ON")
	require.NoError(t, err)

	drv := entsql.OpenDB(dialect.SQLite, db)
	client := enttest.NewClient(t, enttest.WithOptions(dbent.Driver(drv)))
	t.Cleanup(func() { _ = client.Close() })
	return client
}

// TestHandlePaymentNotification_UnknownOrder_ReturnsSentinel exercises the
// happy-path of the webhook 404 fix: when the notification references an
// out_trade_no that does not exist in our DB, HandlePaymentNotification must
// return an error that errors.Is(err, ErrOrderNotFound) recognizes. The
// webhook handler relies on that contract to ack with a 2xx so the provider
// stops retrying.
func TestHandlePaymentNotification_UnknownOrder_ReturnsSentinel(t *testing.T) {
	ctx := context.Background()
	client := newOrderNotFoundTestClient(t)

	svc := &PaymentService{
		entClient:       client,
		providersLoaded: true,
	}

	notification := &payment.PaymentNotification{
		OrderID: "sub2_does_not_exist_12345",
		TradeNo: "stripe_evt_test_xyz",
		Status:  payment.NotificationStatusSuccess,
		Amount:  1000,
	}

	err := svc.HandlePaymentNotification(ctx, notification, payment.TypeStripe)
	require.Error(t, err, "unknown out_trade_no should surface an error")
	require.ErrorIs(t, err, ErrOrderNotFound,
		"webhook handler relies on errors.Is(err, ErrOrderNotFound) to downgrade to 200")

	// Sanity: the wrapped error message should still include the out_trade_no
	// for operator diagnostics.
	require.Contains(t, err.Error(), notification.OrderID)
}

func TestHandlePaymentNotification_AmountMismatch_ReturnsTerminalRejectedAndKeepsOrderPending(t *testing.T) {
	ctx := context.Background()
	client := newOrderNotFoundTestClient(t)

	user, err := client.User.Create().
		SetEmail("payment-amount-mismatch@example.com").
		SetPasswordHash("hash").
		SetUsername("payment-amount-mismatch-user").
		Save(ctx)
	require.NoError(t, err)

	order, err := client.PaymentOrder.Create().
		SetUserID(user.ID).
		SetUserEmail(user.Email).
		SetUserName(user.Username).
		SetAmount(130000).
		SetPayAmount(130000).
		SetPaymentAmount(130000).
		SetPaymentCurrency("VND").
		SetLedgerAmount(5).
		SetLedgerCurrency("USD").
		SetFeeRate(0).
		SetRechargeCode("AMOUNT-MISMATCH-TEST").
		SetOutTradeNo("sub2_amount_mismatch_test").
		SetPaymentType(payment.TypeSepay).
		SetPaymentTradeNo("").
		SetOrderType(payment.OrderTypeBalance).
		SetStatus(OrderStatusPending).
		SetExpiresAt(time.Now().Add(time.Hour)).
		SetClientIP("127.0.0.1").
		SetSrcHost("api.example.com").
		Save(ctx)
	require.NoError(t, err)

	svc := &PaymentService{
		entClient:       client,
		providersLoaded: true,
	}
	notification := &payment.PaymentNotification{
		OrderID:  order.OutTradeNo,
		TradeNo:  "FT26121828344965",
		Status:   payment.NotificationStatusSuccess,
		Amount:   2000,
		Currency: "VND",
	}

	err = svc.HandlePaymentNotification(ctx, notification, payment.TypeSepay)
	require.Error(t, err, "amount mismatch must reject fulfillment")
	require.ErrorIs(t, err, ErrPaymentNotificationRejected,
		"webhook handler relies on this sentinel to return 200 instead of retry-looping with 500")
	require.Contains(t, err.Error(), "amount mismatch")

	reloaded, err := client.PaymentOrder.Get(ctx, order.ID)
	require.NoError(t, err)
	require.Equal(t, OrderStatusPending, reloaded.Status, "mismatched payment must not mark the order paid")
	require.Empty(t, reloaded.PaymentTradeNo)

	audit, err := client.PaymentAuditLog.Query().
		Where(paymentauditlog.OrderID(strconv.FormatInt(order.ID, 10)), paymentauditlog.ActionEQ("PAYMENT_AMOUNT_MISMATCH")).
		Only(ctx)
	require.NoError(t, err, "amount mismatch should remain visible in audit logs")
	require.Contains(t, audit.Detail, "130000")
	require.Contains(t, audit.Detail, "2000")
	require.Contains(t, audit.Detail, notification.TradeNo)
}

// TestHandlePaymentNotification_NonSuccessStatus_Skips documents the
// short-circuit that precedes the DB lookup: when the notification is not a
// success event (e.g. Stripe non-payment events that reach us via the webhook
// route), we return nil without touching the DB and the handler responds 200.
func TestHandlePaymentNotification_NonSuccessStatus_Skips(t *testing.T) {
	ctx := context.Background()
	client := newOrderNotFoundTestClient(t)

	svc := &PaymentService{
		entClient:       client,
		providersLoaded: true,
	}

	notification := &payment.PaymentNotification{
		OrderID: "sub2_does_not_exist_12345",
		Status:  "failed", // any value other than NotificationStatusSuccess
	}

	err := svc.HandlePaymentNotification(ctx, notification, payment.TypeStripe)
	require.NoError(t, err,
		"non-success notifications must short-circuit before the DB lookup")
}

// TestErrOrderNotFound_DistinctFromOtherErrors guards against an accidental
// collapse where a generic wrapped error would start matching ErrOrderNotFound
// (which would silently mask real DB failures).
func TestErrOrderNotFound_DistinctFromOtherErrors(t *testing.T) {
	genericErr := errors.New("some other failure")
	require.False(t, errors.Is(genericErr, ErrOrderNotFound))
	require.False(t, errors.Is(ErrOrderNotFound, genericErr))

	wrappedLookupErr := errors.New("lookup order failed for out_trade_no sub2_42: connection refused")
	require.False(t, errors.Is(wrappedLookupErr, ErrOrderNotFound),
		"DB connection failures must not masquerade as order-not-found")
}
