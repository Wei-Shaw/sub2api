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
	sepay "github.com/emizuki/sepay-go-sdk"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testSePayConfig() map[string]string {
	return map[string]string{
		"merchantId": "MERCHANT_TEST",
		"secretKey":  "sk_test_secret",
		"env":        "sandbox",
		"currency":   "VND",
	}
}

func newTestSePay(t *testing.T, overrides map[string]string) *SePay {
	t.Helper()

	cfg := testSePayConfig()
	for k, v := range overrides {
		cfg[k] = v
	}
	p, err := NewSePay("7", cfg)
	require.NoError(t, err)
	return p
}

// rewriteHostTransport points the SDK's fixed upstream hosts at a test server.
// The SDK hard-codes pay.sepay.vn / pgapi.sepay.vn, so this is the only way to
// exercise QueryOrder and VerifyNotification without touching production code.
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

func pointSePayAtServer(t *testing.T, p *SePay, srv *httptest.Server) {
	t.Helper()

	target, err := url.Parse(srv.URL)
	require.NoError(t, err)
	p.client.SetHTTPClient(&http.Client{Transport: rewriteHostTransport{target: target}})
}

func TestNewSePayRejectsIncompleteConfig(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		overrides  map[string]string
		wantReason string
	}{
		{name: "missing merchantId", overrides: map[string]string{"merchantId": ""}, wantReason: "SEPAY_CONFIG_MISSING_KEY"},
		{name: "missing secretKey", overrides: map[string]string{"secretKey": " "}, wantReason: "SEPAY_CONFIG_MISSING_KEY"},
		{name: "unknown env", overrides: map[string]string{"env": "staging"}, wantReason: "SEPAY_CONFIG_INVALID_ENV"},
		{name: "invalid currency", overrides: map[string]string{"currency": "VNDX"}, wantReason: "SEPAY_CONFIG_INVALID_CURRENCY"},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			cfg := testSePayConfig()
			for k, v := range tc.overrides {
				cfg[k] = v
			}
			_, err := NewSePay("1", cfg)
			require.Error(t, err)
			assert.Equal(t, tc.wantReason, infraerrors.Reason(err))
		})
	}
}

func TestNewSePayNormalizesEnvAndCurrency(t *testing.T) {
	t.Parallel()

	p := newTestSePay(t, map[string]string{"env": "", "currency": ""})
	assert.Equal(t, sepayEnvProduction, p.env())
	assert.Equal(t, payment.DefaultPaymentCurrency, p.currency())

	p = newTestSePay(t, map[string]string{"env": "SANDBOX", "currency": "usd"})
	assert.Equal(t, sepayEnvSandbox, p.env())
	assert.Equal(t, "USD", p.currency())
}

func TestSePayDoesNotMutateCallerConfig(t *testing.T) {
	t.Parallel()

	cfg := testSePayConfig()
	cfg["env"] = "PRODUCTION"
	_, err := NewSePay("1", cfg)
	require.NoError(t, err)
	assert.Equal(t, "PRODUCTION", cfg["env"], "provider must not normalise the caller's map in place")
}

func TestSePaySupportedTypesAndIdentity(t *testing.T) {
	t.Parallel()

	p := newTestSePay(t, nil)
	assert.Equal(t, payment.TypeSePay, p.ProviderKey())
	assert.ElementsMatch(t,
		[]payment.PaymentType{payment.TypeSePayBankTransfer, payment.TypeSePayNapas, payment.TypeSePayCard},
		p.SupportedTypes(),
	)
	assert.Equal(t, map[string]string{
		"merchant_id": "MERCHANT_TEST",
		"currency":    "VND",
		"env":         "sandbox",
	}, p.MerchantIdentityMetadata())
}

func TestSePayMethodForPaymentType(t *testing.T) {
	t.Parallel()

	for paymentType, want := range map[string]sepay.PaymentMethod{
		payment.TypeSePayBankTransfer: sepay.BankTransfer,
		payment.TypeSePayNapas:        sepay.NapasBankTransfer,
		payment.TypeSePayCard:         sepay.Card,
		payment.TypeSePay:             sepay.BankTransfer,
		"":                            sepay.BankTransfer,
	} {
		got, err := sepayMethodForPaymentType(paymentType)
		require.NoErrorf(t, err, "payment type %q", paymentType)
		assert.Equalf(t, want, got, "payment type %q", paymentType)
	}

	_, err := sepayMethodForPaymentType("alipay")
	require.Error(t, err)
	assert.Equal(t, "SEPAY_UNSUPPORTED_PAYMENT_TYPE", infraerrors.Reason(err))
}

func TestSePayCreatePaymentReturnsSignedForm(t *testing.T) {
	t.Parallel()

	p := newTestSePay(t, nil)
	resp, err := p.CreatePayment(context.Background(), payment.CreatePaymentRequest{
		OrderID:     "sub2_20260903abcd1234",
		Amount:      "250000",
		PaymentType: payment.TypeSePayNapas,
		Subject:     "Sub2API 250000 VND",
		ReturnURL:   "https://panel.example.com/payment/result",
	})
	require.NoError(t, err)

	assert.Equal(t, payment.CreatePaymentResultFormPost, resp.ResultType)
	assert.Equal(t, "https://pay-sandbox.sepay.vn/v1/checkout/init", resp.FormAction)
	assert.Equal(t, "sub2_20260903abcd1234", resp.TradeNo)
	assert.Equal(t, "VND", resp.Currency)
	assert.Equal(t, sepayEnvSandbox, resp.PaymentEnv)
	assert.Empty(t, resp.PayURL, "the service layer supplies the bridge URL, not the provider")

	fields := resp.FormFields
	assert.Equal(t, "MERCHANT_TEST", fields["merchant"])
	assert.Equal(t, "PURCHASE", fields["operation"])
	assert.Equal(t, "NAPAS_BANK_TRANSFER", fields["payment_method"])
	assert.Equal(t, "sub2_20260903abcd1234", fields["order_invoice_number"])
	assert.Equal(t, "250000", fields["order_amount"])
	assert.Equal(t, "VND", fields["currency"])
	assert.Equal(t, "https://panel.example.com/payment/result", fields["success_url"])

	// The provider must sign with exactly the SDK's canonical field order, or
	// the gateway rejects the checkout.
	assert.Equal(t, sepaySignFields(fields, "sk_test_secret"), fields["signature"])
	assert.NotEmpty(t, fields["signature"])
}

func TestSePayCreatePaymentIsDeterministic(t *testing.T) {
	t.Parallel()

	// The checkout bridge page re-derives the form on every visit, so two calls
	// with the same inputs must produce byte-identical fields.
	p := newTestSePay(t, nil)
	req := payment.CreatePaymentRequest{
		OrderID:     "sub2_repeat",
		Amount:      "10000",
		PaymentType: payment.TypeSePayBankTransfer,
		Subject:     "Sub2API 10000 VND",
		ReturnURL:   "https://panel.example.com/payment/result",
	}
	first, err := p.CreatePayment(context.Background(), req)
	require.NoError(t, err)
	second, err := p.CreatePayment(context.Background(), req)
	require.NoError(t, err)
	assert.Equal(t, first.FormFields, second.FormFields)
}

func TestSePayCreatePaymentRejectsAmountsBelowCurrencyPrecision(t *testing.T) {
	t.Parallel()

	p := newTestSePay(t, nil)
	_, err := p.CreatePayment(context.Background(), payment.CreatePaymentRequest{
		OrderID:     "sub2_fraction",
		Amount:      "100.50",
		PaymentType: payment.TypeSePayBankTransfer,
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "whole number")
}

func TestSePayQueryOrderParsesUpstreamStatus(t *testing.T) {
	t.Parallel()

	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		assert.True(t, strings.HasPrefix(r.Header.Get("Authorization"), "Basic "))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"order_invoice_number":"sub2_1","order_id":"SP-9","order_status":"COMPLETED","order_amount":"250000","currency":"VND","paid_at":"2026-09-03T10:00:00Z"}}`))
	}))
	defer srv.Close()

	p := newTestSePay(t, nil)
	pointSePayAtServer(t, p, srv)

	got, err := p.QueryOrder(context.Background(), "sub2_1")
	require.NoError(t, err)
	assert.Equal(t, "/v1/order/detail/sub2_1", gotPath)
	assert.Equal(t, "SP-9", got.TradeNo)
	assert.Equal(t, payment.ProviderStatusPaid, got.Status)
	assert.True(t, decimal.NewFromInt(250000).Equal(got.Amount))
	assert.Equal(t, "2026-09-03T10:00:00Z", got.PaidAt)
	assert.Equal(t, "MERCHANT_TEST", got.Metadata["merchant_id"])
	assert.Equal(t, "VND", got.Metadata["currency"])
}

func TestSePayQueryOrderRejectsBlankTradeNo(t *testing.T) {
	t.Parallel()

	p := newTestSePay(t, nil)
	_, err := p.QueryOrder(context.Background(), "  ")
	require.Error(t, err)
}

func TestSePayStatusToProviderStatus(t *testing.T) {
	t.Parallel()

	for status, want := range map[string]string{
		"COMPLETED": payment.ProviderStatusPaid,
		"paid":      payment.ProviderStatusPaid,
		"SETTLED":   payment.ProviderStatusPaid,
		"FAILED":    payment.ProviderStatusFailed,
		"CANCELLED": payment.ProviderStatusFailed,
		"EXPIRED":   payment.ProviderStatusFailed,
		"REFUNDED":  payment.ProviderStatusRefunded,
		"PENDING":   payment.ProviderStatusPending,
		// An unrecognised status must never be read as a terminal failure:
		// the order keeps waiting for the next callback or poll instead.
		"SOMETHING_NEW": payment.ProviderStatusPending,
		"":              payment.ProviderStatusPending,
	} {
		assert.Equalf(t, want, sepayStatusToProviderStatus(status), "status %q", status)
	}
}

func TestParseSePayNotificationFields(t *testing.T) {
	t.Parallel()

	fields, err := parseSePayNotificationFields(`{"merchant":"M1","order_invoice_number":"sub2_1","order_amount":250000,"paid":true}`)
	require.NoError(t, err)
	assert.Equal(t, "M1", fields["merchant"])
	assert.Equal(t, "sub2_1", fields["order_invoice_number"])
	assert.Equal(t, "250000", fields["order_amount"])
	assert.Equal(t, "true", fields["paid"])

	// A nested payload must still surface the order reference, but top-level
	// values win so a nested echo cannot shadow them.
	fields, err = parseSePayNotificationFields(`{"merchant":"M1","data":{"order_invoice_number":"sub2_2","merchant":"OTHER"}}`)
	require.NoError(t, err)
	assert.Equal(t, "sub2_2", fields["order_invoice_number"])
	assert.Equal(t, "M1", fields["merchant"])

	fields, err = parseSePayNotificationFields("order_invoice_number=sub2_3&order_status=COMPLETED")
	require.NoError(t, err)
	assert.Equal(t, "sub2_3", fields["order_invoice_number"])

	_, err = parseSePayNotificationFields("   ")
	require.Error(t, err)

	_, err = parseSePayNotificationFields("{not json")
	require.Error(t, err)
}

func newSePayNotificationServer(t *testing.T, status string, amount string) *httptest.Server {
	t.Helper()

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		body := map[string]any{
			"order_invoice_number": "sub2_1",
			"order_id":             "SP-1",
			"order_status":         status,
			"order_amount":         amount,
			"currency":             "VND",
		}
		_ = json.NewEncoder(w).Encode(body)
	}))
}

func TestSePayVerifyNotificationTrustsUpstreamOverCallbackBody(t *testing.T) {
	t.Parallel()

	// The callback claims a much larger amount than the gateway actually holds.
	// Only the authenticated upstream query may decide the amount, so the
	// forged number must not survive.
	srv := newSePayNotificationServer(t, "COMPLETED", "250000")
	defer srv.Close()

	p := newTestSePay(t, nil)
	pointSePayAtServer(t, p, srv)

	notification, err := p.VerifyNotification(context.Background(),
		`{"merchant":"MERCHANT_TEST","order_invoice_number":"sub2_1","order_amount":"999999999","order_status":"COMPLETED"}`,
		nil)
	require.NoError(t, err)
	require.NotNil(t, notification)
	assert.Equal(t, payment.NotificationStatusSuccess, notification.Status)
	assert.Equal(t, "sub2_1", notification.OrderID)
	assert.Equal(t, "SP-1", notification.TradeNo)
	assert.True(t, decimal.NewFromInt(250000).Equal(notification.Amount))
}

func TestSePayVerifyNotificationReturnsNilWhileUpstreamStillPending(t *testing.T) {
	t.Parallel()

	srv := newSePayNotificationServer(t, "PENDING", "250000")
	defer srv.Close()

	p := newTestSePay(t, nil)
	pointSePayAtServer(t, p, srv)

	notification, err := p.VerifyNotification(context.Background(),
		`{"merchant":"MERCHANT_TEST","order_invoice_number":"sub2_1"}`, nil)
	require.NoError(t, err)
	assert.Nil(t, notification, "an unsettled order must be acked without fulfilling")
}

func TestSePayVerifyNotificationMarksFailedUpstreamOrder(t *testing.T) {
	t.Parallel()

	srv := newSePayNotificationServer(t, "CANCELLED", "250000")
	defer srv.Close()

	p := newTestSePay(t, nil)
	pointSePayAtServer(t, p, srv)

	notification, err := p.VerifyNotification(context.Background(),
		`{"merchant":"MERCHANT_TEST","order_invoice_number":"sub2_1"}`, nil)
	require.NoError(t, err)
	require.NotNil(t, notification)
	assert.Equal(t, payment.ProviderStatusFailed, notification.Status)
}

func TestSePayVerifyNotificationRejectsBadSignature(t *testing.T) {
	t.Parallel()

	srv := newSePayNotificationServer(t, "COMPLETED", "250000")
	defer srv.Close()

	p := newTestSePay(t, nil)
	pointSePayAtServer(t, p, srv)

	_, err := p.VerifyNotification(context.Background(),
		`{"merchant":"MERCHANT_TEST","order_invoice_number":"sub2_1","signature":"not-the-right-signature"}`,
		nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "signature mismatch")
}

func TestSePayVerifyNotificationAcceptsValidSignature(t *testing.T) {
	t.Parallel()

	srv := newSePayNotificationServer(t, "COMPLETED", "250000")
	defer srv.Close()

	p := newTestSePay(t, nil)
	pointSePayAtServer(t, p, srv)

	fields := map[string]string{
		"merchant":             "MERCHANT_TEST",
		"operation":            "PURCHASE",
		"order_amount":         "250000",
		"currency":             "VND",
		"order_invoice_number": "sub2_1",
	}
	fields["signature"] = sepaySignFields(fields, "sk_test_secret")
	body, err := json.Marshal(fields)
	require.NoError(t, err)

	notification, err := p.VerifyNotification(context.Background(), string(body), nil)
	require.NoError(t, err)
	require.NotNil(t, notification)
	assert.Equal(t, payment.NotificationStatusSuccess, notification.Status)
}

func TestSePayVerifyNotificationRejectsForeignMerchant(t *testing.T) {
	t.Parallel()

	srv := newSePayNotificationServer(t, "COMPLETED", "250000")
	defer srv.Close()

	p := newTestSePay(t, nil)
	pointSePayAtServer(t, p, srv)

	_, err := p.VerifyNotification(context.Background(),
		`{"merchant":"SOMEONE_ELSE","order_invoice_number":"sub2_1"}`, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "merchant mismatch")
}

func TestSePayVerifyNotificationRequiresOrderReference(t *testing.T) {
	t.Parallel()

	p := newTestSePay(t, nil)
	_, err := p.VerifyNotification(context.Background(), `{"merchant":"MERCHANT_TEST"}`, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "order_invoice_number")
}

func TestSePayVerifyNotificationFailsWhenUpstreamUnreachable(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	p := newTestSePay(t, nil)
	pointSePayAtServer(t, p, srv)

	// Without a usable upstream answer the callback must not be treated as
	// paid — an error here makes the gateway retry instead.
	_, err := p.VerifyNotification(context.Background(),
		`{"merchant":"MERCHANT_TEST","order_invoice_number":"sub2_1"}`, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "upstream confirmation failed")
}

func TestSePayCancelPaymentFallsBackToVoid(t *testing.T) {
	t.Parallel()

	var seen []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = append(seen, r.URL.Path)
		if strings.HasSuffix(r.URL.Path, "/order/cancel") {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"VOIDED"}`))
	}))
	defer srv.Close()

	p := newTestSePay(t, nil)
	pointSePayAtServer(t, p, srv)

	require.NoError(t, p.CancelPayment(context.Background(), "sub2_1"))
	assert.Equal(t, []string{"/v1/order/cancel", "/v1/order/voidTransaction"}, seen)
}

func TestSePayImplementsCancelableProvider(t *testing.T) {
	t.Parallel()

	var _ payment.CancelableProvider = newTestSePay(t, nil)
	var _ payment.MerchantIdentityProvider = newTestSePay(t, nil)
}

func TestCreateProviderRoutesSePayKeyOnly(t *testing.T) {
	t.Parallel()

	prov, err := CreateProvider(payment.TypeSePay, "1", testSePayConfig())
	require.NoError(t, err)
	assert.Equal(t, payment.TypeSePay, prov.ProviderKey())

	_, err = CreateProvider("stripe", "1", testSePayConfig())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown provider key")
}
