//go:build unit

package handler

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/enttest"
	"github.com/Wei-Shaw/sub2api/internal/payment"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"
)

var b03ErrTransientDB = errors.New("pq: server closed the connection unexpectedly")

func b03NewClient(t *testing.T, name string) *dbent.Client {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+name+"?mode=memory&cache=shared")
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	_, err = db.Exec("PRAGMA foreign_keys = ON")
	require.NoError(t, err)
	drv := entsql.OpenDB(dialect.SQLite, db)
	client := enttest.NewClient(t, enttest.WithOptions(dbent.Driver(drv)))
	t.Cleanup(func() { _ = client.Close() })
	return client
}

// b03FailWhen 让某张表的查询在 armed 置位后返回瞬时 DB 错误。
func b03FailWhen(armed *atomic.Bool) dbent.Interceptor {
	return dbent.TraverseFunc(func(context.Context, dbent.Query) error {
		if armed.Load() {
			return b03ErrTransientDB
		}
		return nil
	})
}

func b03PostNowPayments(t *testing.T, client *dbent.Client, body string) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	svc := service.NewPaymentService(client, payment.NewRegistry(), nil, nil, nil, nil, nil, nil, nil)
	h := NewPaymentWebhookHandler(svc, payment.NewRegistry())

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/payment/webhook/nowpayments", bytes.NewBufferString(body))
	c.Request.Header.Set("Content-Type", "application/json")
	h.NowPaymentsNotify(c)
	return rec
}

func b03CreateInstance(t *testing.T, client *dbent.Client, name string) *dbent.PaymentProviderInstance {
	t.Helper()
	inst, err := client.PaymentProviderInstance.Create().
		SetProviderKey(payment.TypeNowPayments).
		SetName(name).
		SetConfig("{}").
		SetSupportedTypes(payment.TypeNowPayments).
		SetEnabled(true).
		Save(context.Background())
	require.NoError(t, err)
	return inst
}

const b03FinishedIPN = `{"order_id":"sub2_b03_order","payment_status":"finished","payment_id":123}`

func TestB03WebhookReturns5xxWhenOrderLookupFails(t *testing.T) {
	client := b03NewClient(t, "b03_order_lookup_fails")
	var armed atomic.Bool
	client.PaymentOrder.Intercept(b03FailWhen(&armed))
	armed.Store(true)

	rec := b03PostNowPayments(t, client, b03FinishedIPN)
	require.GreaterOrEqual(t, rec.Code, http.StatusInternalServerError,
		"a transient order lookup error must not be acked; body=%q", rec.Body.String())
	require.NotEqual(t, "success", rec.Body.String())
}

func TestB03WebhookReturns5xxWhenFallbackCountFails(t *testing.T) {
	client := b03NewClient(t, "b03_fallback_count_fails")
	var armed atomic.Bool
	client.PaymentProviderInstance.Intercept(b03FailWhen(&armed))
	armed.Store(true)

	rec := b03PostNowPayments(t, client, b03FinishedIPN)
	require.GreaterOrEqual(t, rec.Code, http.StatusInternalServerError,
		"a failed instance count must not turn into an acked 'ambiguous'; body=%q", rec.Body.String())
}

func TestB03WebhookReturns5xxWhenPinnedInstanceLoadFails(t *testing.T) {
	ctx := context.Background()
	client := b03NewClient(t, "b03_pinned_instance_fails")
	inst := b03CreateInstance(t, client, "np-a")
	user, err := client.User.Create().
		SetEmail("b03-pinned@example.com").
		SetPasswordHash("hash").
		SetUsername("b03-pinned").
		Save(ctx)
	require.NoError(t, err)
	_, err = client.PaymentOrder.Create().
		SetUserID(user.ID).
		SetUserEmail(user.Email).
		SetUserName(user.Username).
		SetAmount(10).
		SetPayAmount(10).
		SetFeeRate(0).
		SetRechargeCode("B03-PINNED").
		SetOutTradeNo("sub2_b03_order").
		SetPaymentType(payment.TypeNowPayments).
		SetPaymentTradeNo("np-invoice-1").
		SetOrderType(payment.OrderTypeBalance).
		SetStatus(service.OrderStatusPending).
		SetExpiresAt(time.Now().Add(time.Hour)).
		SetClientIP("127.0.0.1").
		SetSrcHost("api.example.com").
		SetProviderInstanceID(strconv.FormatInt(inst.ID, 10)).
		Save(ctx)
	require.NoError(t, err)

	var armed atomic.Bool
	client.PaymentProviderInstance.Intercept(b03FailWhen(&armed))
	armed.Store(true)

	rec := b03PostNowPayments(t, client, b03FinishedIPN)
	require.GreaterOrEqual(t, rec.Code, http.StatusInternalServerError,
		"a transient pinned-instance load error must not be acked; body=%q", rec.Body.String())
}

// 确定性的「找不到服务商」仍然要 ACK，避免外部误配的回调无限重试。
func TestB03WebhookStillAcksWhenProviderNotFound(t *testing.T) {
	client := b03NewClient(t, "b03_provider_not_found")

	rec := b03PostNowPayments(t, client, b03FinishedIPN)
	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "success", rec.Body.String())
}

func TestB03WebhookStillAcksWhenFallbackAmbiguous(t *testing.T) {
	client := b03NewClient(t, "b03_fallback_ambiguous")
	b03CreateInstance(t, client, "np-a")
	b03CreateInstance(t, client, "np-b")

	rec := b03PostNowPayments(t, client, b03FinishedIPN)
	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "success", rec.Body.String())
}
