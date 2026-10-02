//go:build unit

package provider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/payment"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const nowPaymentsTestIPNSecret = "ipn-secret"

// nowPaymentsTestBody 的键故意乱序，且包含一个小数与一个带尖括号的字符串——
// 这三点正是签名规范化最容易出错的地方。
const nowPaymentsTestBody = `{"payment_status":"finished","order_id":"sub2_20240101_0001",` +
	`"price_amount":10.5,"payment_id":5745459419,"actually_paid":0.00034,` +
	`"price_currency":"usd","pay_currency":"btc","order_description":"a<b&c","invoice_id":4522625843}`

// nowPaymentsTestCanonical 是上面这段回调体排序后的规范形式，逐字节手写：
// 键名按字典序、数字保持字面量、尖括号与 & 不转义。签名就是对这串字节算的。
const nowPaymentsTestCanonical = `{"actually_paid":0.00034,"invoice_id":4522625843,` +
	`"order_description":"a<b&c","order_id":"sub2_20240101_0001","pay_currency":"btc",` +
	`"payment_id":5745459419,"payment_status":"finished","price_amount":10.5,"price_currency":"usd"}`

func newTestNowPayments(t *testing.T, overrides map[string]string) *NowPayments {
	t.Helper()
	cfg := map[string]string{
		"apiKey":       "np_test_api_key",
		"ipnSecretKey": nowPaymentsTestIPNSecret,
		"env":          "sandbox",
		"currency":     "USD",
	}
	for k, v := range overrides {
		cfg[k] = v
	}
	provider, err := NewNowPayments("1", cfg)
	if err != nil {
		t.Fatalf("NewNowPayments: %v", err)
	}
	return provider
}

type rewriteHostTransport struct {
	target *url.URL
}

func (rt rewriteHostTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	clone := req.Clone(req.Context())
	clone.URL.Scheme = rt.target.Scheme
	clone.URL.Host = rt.target.Host
	clone.Host = rt.target.Host
	return http.DefaultTransport.RoundTrip(clone)
}

// pointNowPaymentsAtServer redirects the provider's fixed upstream host at a
// test server; the base URL is derived from env, so this is the only seam.
func pointNowPaymentsAtServer(t *testing.T, p *NowPayments, srv *httptest.Server) {
	t.Helper()
	target, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatalf("parse test server url: %v", err)
	}
	p.client = &http.Client{Transport: rewriteHostTransport{target: target}}
}

func TestNowPaymentsCanonicalJSON(t *testing.T) {
	canonical, err := nowPaymentsCanonicalJSON(nowPaymentsTestBody)
	if err != nil {
		t.Fatalf("canonicalize: %v", err)
	}
	if string(canonical) != nowPaymentsTestCanonical {
		t.Fatalf("canonical form mismatch:\n got: %s\nwant: %s", canonical, nowPaymentsTestCanonical)
	}
}

func TestNowPaymentsVerifyNotification(t *testing.T) {
	provider := newTestNowPayments(t, nil)
	signature := nowPaymentsSign([]byte(nowPaymentsTestCanonical), nowPaymentsTestIPNSecret)

	notification, err := provider.VerifyNotification(context.Background(), nowPaymentsTestBody, map[string]string{
		"x-nowpayments-sig": signature,
	})
	if err != nil {
		t.Fatalf("VerifyNotification: %v", err)
	}
	if notification == nil {
		t.Fatal("VerifyNotification returned no notification for a finished payment")
	}
	if notification.Status != payment.NotificationStatusSuccess {
		t.Fatalf("status = %q, want %q", notification.Status, payment.NotificationStatusSuccess)
	}
	if notification.OrderID != "sub2_20240101_0001" {
		t.Fatalf("order id = %q", notification.OrderID)
	}
	if notification.TradeNo != "5745459419" {
		t.Fatalf("trade no = %q, want the payment_id", notification.TradeNo)
	}
	// 金额必须取法币计价的 price_amount，而不是链上实付的 actually_paid。
	if got := notification.Amount.String(); got != "10.5" {
		t.Fatalf("amount = %s, want 10.5", got)
	}
}

func TestNowPaymentsVerifyNotificationRejectsBadSignature(t *testing.T) {
	provider := newTestNowPayments(t, nil)

	cases := map[string]map[string]string{
		"missing header": {},
		"wrong signature": {
			"x-nowpayments-sig": nowPaymentsSign([]byte(nowPaymentsTestCanonical), "other-secret"),
		},
		"signature over unsorted body": {
			"x-nowpayments-sig": nowPaymentsSign([]byte(nowPaymentsTestBody), nowPaymentsTestIPNSecret),
		},
	}
	for name, headers := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := provider.VerifyNotification(context.Background(), nowPaymentsTestBody, headers); err == nil {
				t.Fatal("expected signature verification to fail")
			}
		})
	}
}

func TestNowPaymentsVerifyNotificationIgnoresIntermediateStatus(t *testing.T) {
	provider := newTestNowPayments(t, nil)
	// partially_paid 是「收到钱但不够」，必须当作未结算事件放过，
	// 不能按不足额的金额履约。
	body := `{"order_id":"sub2_20240101_0001","payment_status":"partially_paid","price_amount":10.5,"price_currency":"usd"}`
	canonical, err := nowPaymentsCanonicalJSON(body)
	if err != nil {
		t.Fatalf("canonicalize: %v", err)
	}

	notification, err := provider.VerifyNotification(context.Background(), body, map[string]string{
		"x-nowpayments-sig": nowPaymentsSign(canonical, nowPaymentsTestIPNSecret),
	})
	if err != nil {
		t.Fatalf("VerifyNotification: %v", err)
	}
	if notification != nil {
		t.Fatalf("expected no notification for partially_paid, got %+v", notification)
	}
}

func TestNowPaymentsVerifyNotificationRejectsCurrencyMismatch(t *testing.T) {
	provider := newTestNowPayments(t, nil)
	body := `{"order_id":"sub2_20240101_0001","payment_status":"finished","price_amount":10.5,"price_currency":"vnd"}`
	canonical, err := nowPaymentsCanonicalJSON(body)
	if err != nil {
		t.Fatalf("canonicalize: %v", err)
	}

	if _, err := provider.VerifyNotification(context.Background(), body, map[string]string{
		"x-nowpayments-sig": nowPaymentsSign(canonical, nowPaymentsTestIPNSecret),
	}); err == nil {
		t.Fatal("expected a currency mismatch to be rejected")
	}
}

func TestNowPaymentsRequiresIPNSecret(t *testing.T) {
	if _, err := NewNowPayments("1", map[string]string{"apiKey": "test-api-key"}); err == nil {
		t.Fatal("expected a missing ipnSecretKey to be rejected")
	}
}

func TestNowPaymentsStatusMapping(t *testing.T) {
	cases := map[string]string{
		"finished":       payment.ProviderStatusPaid,
		"waiting":        payment.ProviderStatusPending,
		"confirming":     payment.ProviderStatusPending,
		"sending":        payment.ProviderStatusPending,
		"partially_paid": payment.ProviderStatusPending,
		"something_new":  payment.ProviderStatusPending,
		"failed":         payment.ProviderStatusFailed,
		"expired":        payment.ProviderStatusFailed,
		"refunded":       payment.ProviderStatusRefunded,
	}
	for status, want := range cases {
		if got := nowPaymentsStatusToProviderStatus(status); got != want {
			t.Errorf("status %q = %q, want %q", status, got, want)
		}
	}
}

func TestGetBasePaymentTypeCoversNowPayments(t *testing.T) {
	if got := payment.GetBasePaymentType(payment.TypeNowPaymentsCrypto); got != payment.TypeNowPayments {
		t.Fatalf("base type = %q, want %q", got, payment.TypeNowPayments)
	}
	if got := payment.GetBasePaymentType(payment.TypeGPMPayBankTransfer); got != payment.TypeGPMPay {
		t.Fatalf("base type = %q, want %q", got, payment.TypeGPMPay)
	}
	// Historical orders from the removed SePay gateway keep their stored type.
	if got := payment.GetBasePaymentType("sepay_bank_transfer"); got != "sepay_bank_transfer" {
		t.Fatalf("base type = %q, want legacy value unchanged", got)
	}
}

func TestNowPaymentsCreatePaymentSurfacesUpstreamRejection(t *testing.T) {
	t.Parallel()

	// The upstream answers an invalid price_currency with a bare INTERNAL_ERROR
	// that names no field. Whatever we send has to come back with the error, or
	// the only way to find the bad value is to guess.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"status":false,"statusCode":500,"code":"INTERNAL_ERROR","message":"The server encountered an internal error"}`))
	}))
	defer srv.Close()

	p := newTestNowPayments(t, map[string]string{"currency": "VND"})
	pointNowPaymentsAtServer(t, p, srv)

	_, err := p.CreatePayment(context.Background(), payment.CreatePaymentRequest{
		OrderID:     "sub2_20260904abcd1234",
		Amount:      "250000",
		PaymentType: payment.TypeNowPaymentsCrypto,
		ReturnURL:   "https://panel.example.com/payment/result",
		NotifyURL:   "https://panel.example.com/api/v1/payment/webhook/nowpayments",
	})
	require.Error(t, err)
	assert.Equal(t, "PAYMENT_GATEWAY_ERROR", infraerrors.Reason(err))
	assert.Contains(t, err.Error(), "INTERNAL_ERROR")

	var appErr *infraerrors.ApplicationError
	require.ErrorAs(t, err, &appErr)
	meta := appErr.Metadata
	assert.Contains(t, meta["request_body"], `"price_currency":"vnd"`)
	assert.Contains(t, meta["request_body"], `"price_amount":250000`)
	assert.Contains(t, meta["request_body"], `"order_id":"sub2_20260904abcd1234"`)
	// The API key must never ride along in an error the user can see.
	for _, value := range meta {
		assert.NotContains(t, value, "np_test_api_key")
	}
}

func TestNowPaymentsCreatePaymentDoesNotSwallowBadRequest(t *testing.T) {
	t.Parallel()

	// A 400 on /invoice is a parameter error. Reading it as "resource not found"
	// — which is only ever true for a payment lookup — throws away what the
	// gateway actually said.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"message":"price_currency is not supported"}`))
	}))
	defer srv.Close()

	p := newTestNowPayments(t, nil)
	pointNowPaymentsAtServer(t, p, srv)

	_, err := p.CreatePayment(context.Background(), payment.CreatePaymentRequest{
		OrderID:     "sub2_20260904abcd1234",
		Amount:      "10",
		PaymentType: payment.TypeNowPaymentsCrypto,
		ReturnURL:   "https://panel.example.com/payment/result",
		NotifyURL:   "https://panel.example.com/api/v1/payment/webhook/nowpayments",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "price_currency is not supported")
	assert.NotContains(t, err.Error(), "not found")
}

func TestNowPaymentsCreatePaymentRequiresNotifyURL(t *testing.T) {
	t.Parallel()

	// IPN is the only proof of payment this gateway offers — there is no order
	// lookup to fall back on. Opening an invoice without a callback URL yields
	// an order the customer can pay and we can never credit, which is worse
	// than refusing up front.
	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"1","invoice_url":"https://nowpayments.io/invoice/1"}`))
	}))
	defer srv.Close()

	p := newTestNowPayments(t, nil)
	pointNowPaymentsAtServer(t, p, srv)

	_, err := p.CreatePayment(context.Background(), payment.CreatePaymentRequest{
		OrderID:     "sub2_20260904abcd1234",
		Amount:      "5",
		PaymentType: payment.TypeNowPaymentsCrypto,
		ReturnURL:   "https://panel.example.com/payment/result",
	})
	require.Error(t, err)
	assert.Equal(t, "NOWPAYMENTS_NOTIFY_URL_REQUIRED", infraerrors.Reason(err))
	assert.False(t, called, "must not reach the gateway at all")
}

func TestNowPaymentsCreatePaymentSendsTheCallbackURL(t *testing.T) {
	t.Parallel()

	var sent nowPaymentsInvoiceRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&sent)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"1","invoice_url":"https://nowpayments.io/invoice/1"}`))
	}))
	defer srv.Close()

	p := newTestNowPayments(t, nil)
	pointNowPaymentsAtServer(t, p, srv)

	resp, err := p.CreatePayment(context.Background(), payment.CreatePaymentRequest{
		OrderID:     "sub2_20260904abcd1234",
		Amount:      "5",
		PaymentType: payment.TypeNowPaymentsCrypto,
		ReturnURL:   "https://panel.example.com/payment/result",
		NotifyURL:   "https://panel.example.com/api/v1/payment/webhook/nowpayments",
	})
	require.NoError(t, err)
	assert.Equal(t, "https://nowpayments.io/invoice/1", resp.PayURL)
	assert.Equal(t, "https://panel.example.com/api/v1/payment/webhook/nowpayments", sent.IPNCallbackURL)
}

func TestNowPaymentsRedirectURLDropsResumeTokenWhenTooLong(t *testing.T) {
	t.Parallel()

	base := "https://panel.example.com/payment/result?order_id=31&out_trade_no=sub2_202609045rC1GhW0&status=success"
	// Short enough already: leave it exactly as the service built it.
	assert.Equal(t, base, nowPaymentsRedirectURL(base))

	long := base + "&resume_token=" + strings.Repeat("a", 300)
	got := nowPaymentsRedirectURL(long)
	assert.NotContains(t, got, "resume_token")
	assert.LessOrEqual(t, len(got), nowPaymentsMaxRedirectURLLength)
	// The order must still be identifiable, or the customer lands on a result
	// page that cannot say which payment it is about.
	assert.Contains(t, got, "order_id=31")
	assert.Contains(t, got, "out_trade_no=sub2_202609045rC1GhW0")
	assert.Contains(t, got, "status=success")
}

func TestNowPaymentsCreatePaymentSendsAShortRedirectURL(t *testing.T) {
	t.Parallel()

	var sent nowPaymentsInvoiceRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&sent)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"1","invoice_url":"https://nowpayments.io/invoice/1"}`))
	}))
	defer srv.Close()

	p := newTestNowPayments(t, nil)
	pointNowPaymentsAtServer(t, p, srv)

	_, err := p.CreatePayment(context.Background(), payment.CreatePaymentRequest{
		OrderID:     "sub2_20260904abcd1234",
		Amount:      "50",
		PaymentType: payment.TypeNowPaymentsCrypto,
		ReturnURL:   "https://panel.example.com/payment/result?order_id=31&status=success&resume_token=" + strings.Repeat("a", 300),
		NotifyURL:   "https://panel.example.com/api/v1/payment/webhook/nowpayments",
	})
	require.NoError(t, err)
	assert.LessOrEqual(t, len(sent.SuccessURL), nowPaymentsMaxRedirectURLLength)
	assert.LessOrEqual(t, len(sent.CancelURL), nowPaymentsMaxRedirectURLLength)
	assert.Contains(t, sent.SuccessURL, "order_id=31")
}
