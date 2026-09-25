//go:build unit

package service

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/enttest"
	"github.com/Wei-Shaw/sub2api/internal/payment"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"database/sql"
	_ "modernc.org/sqlite"
)

func merchantTestService(t *testing.T, repo SettingRepository) *PaymentService {
	t.Helper()
	return &PaymentService{configService: &PaymentConfigService{settingRepo: repo}}
}

func merchantTestRepoWith(t *testing.T, merchants []PaymentMerchant) *paymentConfigSettingRepoStub {
	t.Helper()
	data, err := json.Marshal(merchants)
	require.NoError(t, err)
	return &paymentConfigSettingRepoStub{values: map[string]string{SettingPaymentMerchants: string(data)}}
}

func merchantSign(secret, ts string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(ts))
	mac.Write([]byte{'.'})
	mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}

func TestVerifyMerchantSignature_RoundTrip(t *testing.T) {
	repo := merchantTestRepoWith(t, []PaymentMerchant{{ID: "b-site", Name: "B", Secret: "s3cret", Enabled: true}})
	svc := merchantTestService(t, repo)
	ctx := context.Background()

	body := []byte(`{"amount":29.9}`)
	ts := fmt.Sprintf("%d", time.Now().Unix())
	sig := merchantSign("s3cret", ts, body)

	m, err := svc.VerifyMerchantSignature(ctx, "b-site", ts, sig, body)
	require.NoError(t, err)
	require.Equal(t, "b-site", m.ID)

	// wrong secret → 401
	_, err = svc.VerifyMerchantSignature(ctx, "b-site", ts, merchantSign("wrong", ts, body), body)
	require.Error(t, err)
	require.Equal(t, 401, infraerrors.Code(err))

	// stale timestamp → 401
	stale := fmt.Sprintf("%d", time.Now().Add(-10*time.Minute).Unix())
	_, err = svc.VerifyMerchantSignature(ctx, "b-site", stale, merchantSign("s3cret", stale, body), body)
	require.Error(t, err)

	// unknown merchant → 404
	_, err = svc.VerifyMerchantSignature(ctx, "nope", ts, sig, body)
	require.Error(t, err)

	// disabled merchant → 403
	repo2 := merchantTestRepoWith(t, []PaymentMerchant{{ID: "b-site", Secret: "s3cret", Enabled: false}})
	_, err = merchantTestService(t, repo2).VerifyMerchantSignature(ctx, "b-site", ts, sig, body)
	require.Error(t, err)
}

func TestPaymentMerchantRegistry_CRUD(t *testing.T) {
	repo := &paymentConfigSettingRepoStub{values: map[string]string{}}
	svc := &PaymentConfigService{settingRepo: repo}
	ctx := context.Background()

	// empty registry
	list, err := svc.ListPaymentMerchants(ctx)
	require.NoError(t, err)
	require.Empty(t, list)

	// create with secret
	require.NoError(t, svc.SavePaymentMerchant(ctx, PaymentMerchant{ID: "b", Name: "B", Secret: "sec1", Enabled: true}))
	got, err := svc.GetPaymentMerchant(ctx, "b")
	require.NoError(t, err)
	require.Equal(t, "sec1", got.Secret)

	// update without secret keeps stored secret
	require.NoError(t, svc.SavePaymentMerchant(ctx, PaymentMerchant{ID: "b", Name: "B2", Enabled: true}))
	got, err = svc.GetPaymentMerchant(ctx, "b")
	require.NoError(t, err)
	require.Equal(t, "sec1", got.Secret)
	require.Equal(t, "B2", got.Name)

	// new merchant without secret is rejected
	err = svc.SavePaymentMerchant(ctx, PaymentMerchant{ID: "c", Name: "C", Enabled: true})
	require.Error(t, err)
}

func TestExecuteMerchantFulfillment_CompletesWithoutBalanceCredit(t *testing.T) {
	client := newMerchantEntClient(t)
	ctx := context.Background()

	u := client.User.Create().
		SetEmail("b-site@merchant.sub2api.local").
		SetUsername("merchant-b-site").
		SetPasswordHash("x").
		SetRole("user").
		SetStatus("active").
		SetBalance(0).
		SaveX(ctx)

	paidAt := time.Now().Add(-time.Minute)
	o := client.PaymentOrder.Create().
		SetUserID(u.ID).
		SetUserEmail(u.Email).
		SetUserName(u.Username).
		SetAmount(29.9).
		SetPayAmount(29.9).
		SetFeeRate(0).
		SetRechargeCode("").
		SetOutTradeNo("sub2_merchant_test_1").
		SetPaymentType(payment.TypeAlipay).
		SetPaymentTradeNo("").
		SetOrderType(payment.OrderTypeMerchant).
		SetProviderSnapshot(map[string]any{"merchant_id": "b-site", "out_order_id": "ord_123", "notify_url": "", "subject": "Ultra"}).
		SetStatus(OrderStatusPaid).
		SetPaidAt(paidAt).
		SetExpiresAt(time.Now().Add(time.Hour)).
		SetClientIP("").
		SetSrcHost("merchant-api").
		SaveX(ctx)

	svc := &PaymentService{entClient: client}
	require.NoError(t, svc.ExecuteMerchantFulfillment(ctx, o.ID))

	got, err := client.PaymentOrder.Get(ctx, o.ID)
	require.NoError(t, err)
	require.Equal(t, OrderStatusCompleted, got.Status)
	require.NotNil(t, got.CompletedAt)

	// balance untouched
	uAfter, err := client.User.Get(ctx, u.ID)
	require.NoError(t, err)
	require.Equal(t, 0.0, uAfter.Balance)

	// audit trail exists
	logs, err := client.PaymentAuditLog.Query().All(ctx)
	require.NoError(t, err)
	require.NotEmpty(t, logs)

	// idempotent re-run stays COMPLETED
	require.NoError(t, svc.ExecuteMerchantFulfillment(ctx, o.ID))
	got2, err := client.PaymentOrder.Get(ctx, o.ID)
	require.NoError(t, err)
	require.Equal(t, OrderStatusCompleted, got2.Status)
}

func TestPrepareRefund_RejectsMerchantOrders(t *testing.T) {
	client := newMerchantEntClient(t)
	ctx := context.Background()

	u := client.User.Create().
		SetEmail("b2@merchant.sub2api.local").
		SetUsername("merchant-b2").
		SetPasswordHash("x").
		SetRole("user").
		SetStatus("active").
		SetBalance(0).
		SaveX(ctx)
	o := client.PaymentOrder.Create().
		SetUserID(u.ID).
		SetUserEmail(u.Email).
		SetUserName(u.Username).
		SetAmount(9.9).
		SetPayAmount(9.9).
		SetFeeRate(0).
		SetRechargeCode("").
		SetOutTradeNo("sub2_merchant_refund_1").
		SetPaymentType(payment.TypeAlipay).
		SetPaymentTradeNo("").
		SetOrderType(payment.OrderTypeMerchant).
		SetStatus(OrderStatusCompleted).
		SetPaidAt(time.Now()).
		SetCompletedAt(time.Now()).
		SetExpiresAt(time.Now().Add(time.Hour)).
		SetClientIP("").
		SetSrcHost("merchant-api").
		SaveX(ctx)

	svc := &PaymentService{entClient: client}
	_, _, err := svc.PrepareRefund(ctx, o.ID, 9.9, "test", false, false)
	require.Error(t, err)
	require.Contains(t, err.Error(), "MERCHANT_ORDER_NOT_REFUNDABLE")
}

func newMerchantEntClient(t *testing.T) *dbent.Client {
	t.Helper()
	dbName := fmt.Sprintf("file:%s?mode=memory&cache=shared&_fk=1", t.Name())
	db, err := sql.Open("sqlite", dbName)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	if _, err := db.Exec("PRAGMA foreign_keys = ON"); err != nil {
		t.Fatalf("enable foreign keys: %v", err)
	}
	drv := entsql.OpenDB(dialect.SQLite, db)
	client := enttest.NewClient(t, enttest.WithOptions(dbent.Driver(drv)))
	t.Cleanup(func() { _ = client.Close() })
	return client
}
