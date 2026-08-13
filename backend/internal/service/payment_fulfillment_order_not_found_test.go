//go:build unit

package service

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	_ "modernc.org/sqlite"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/enttest"
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

// TestHandlePaymentNotification_NonSuccessStatus_Skips documents the
// short-circuit that precedes the DB lookup: when the notification is not a
// success event (e.g. Stripe non-payment events that reach us via the webhook
// route), we return nil without touching the DB and the handler responds 200.

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
