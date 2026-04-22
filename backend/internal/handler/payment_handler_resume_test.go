//go:build unit

package handler

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/enttest"
	"github.com/Wei-Shaw/sub2api/internal/payment"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	_ "modernc.org/sqlite"
)

func TestApplyWeChatPaymentResumeClaims(t *testing.T) {
	t.Parallel()

	req := CreateOrderRequest{
		Amount:      0,
		PaymentType: payment.TypeWxpay,
		OrderType:   payment.OrderTypeBalance,
REDACTED

	err := applyWeChatPaymentResumeClaims(&req, &service.WeChatPaymentResumeClaims{
		OpenID:      "openid-123",
		PaymentType: payment.TypeWxpay,
		Amount:      "12.50",
		OrderType:   payment.OrderTypeSubscription,
		PlanID:      7,
REDACTED)
	if err != nil {
		t.Fatalf("applyWeChatPaymentResumeClaims returned error: %v", err)
REDACTED
	if req.OpenID != "openid-123" {
		t.Fatalf("openid = %q, want %q", req.OpenID, "openid-123")
REDACTED
	if req.Amount != 12.5 {
		t.Fatalf("amount = %v, want 12.5", req.Amount)
REDACTED
	if req.OrderType != payment.OrderTypeSubscription {
		t.Fatalf("order_type = %q, want %q", req.OrderType, payment.OrderTypeSubscription)
REDACTED
	if req.PlanID != 7 {
		t.Fatalf("plan_id = %d, want 7", req.PlanID)
REDACTED
REDACTED

func TestApplyWeChatPaymentResumeClaimsRejectsPaymentTypeMismatch(t *testing.T) {
	t.Parallel()

	req := CreateOrderRequest{
		PaymentType: payment.TypeAlipay,
REDACTED

	err := applyWeChatPaymentResumeClaims(&req, &service.WeChatPaymentResumeClaims{
		OpenID:      "openid-123",
		PaymentType: payment.TypeWxpay,
		Amount:      "12.50",
		OrderType:   payment.OrderTypeBalance,
REDACTED)
	if err == nil {
		t.Fatal("applyWeChatPaymentResumeClaims should reject mismatched payment types")
REDACTED
REDACTED

func TestVerifyOrderPublicReturnsGone(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)

	db, err := sql.Open("sqlite", "file:payment_handler_public_verify?mode=memory&cache=shared")
REDACTED
	t.Cleanup(func() { _ = db.Close() REDACTED)

	_, err = db.Exec("PRAGMA foreign_keys = ON")
REDACTED

	drv := entsql.OpenDB(dialect.SQLite, db)
	client := enttest.NewClient(t, enttest.WithOptions(dbent.Driver(drv)))
	t.Cleanup(func() { _ = client.Close() REDACTED)

	paymentSvc := service.NewPaymentService(client, payment.NewRegistry(), nil, nil, nil, nil, nil, nil)
	h := NewPaymentHandler(paymentSvc, nil, nil)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(
		http.MethodPost,
		"/api/v1/payment/public/orders/verify",
		bytes.NewBufferString(`{"out_trade_no":"legacy-order-no"REDACTED`),
	)
	ctx.Request.Header.Set("Content-Type", "application/json")

	h.VerifyOrderPublic(ctx)

	require.Equal(t, http.StatusGone, recorder.Code)

	var resp response.Response
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &resp))
	require.Equal(t, http.StatusGone, resp.Code)
	require.Equal(t, "PAYMENT_PUBLIC_ORDER_VERIFY_REMOVED", resp.Reason)
	require.Contains(t, resp.Message, "removed")
REDACTED
