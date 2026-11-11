//go:build unit

package provider

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/payment"
	"github.com/stretchr/testify/require"
)

const creemTestProductJSON = `{"id":"prod_1","status":"active","billing_type":"onetime","currency":"USD","tax_mode":"inclusive"}`

func TestNewCreemValidatesConfig(t *testing.T) {
	t.Parallel()

	_, err := NewCreem("1", map[string]string{"apiKey": "creem_test_x", "webhookSecret": "s"})
	require.ErrorContains(t, err, "productId")

	_, err = NewCreem("1", map[string]string{"apiKey": "creem_test_x", "webhookSecret": "s", "productId": "prod_1", "apiBase": "https://evil.example.com"})
	require.ErrorContains(t, err, "apiBase host")

	_, err = NewCreem("1", map[string]string{"apiKey": "creem_test_x", "webhookSecret": "s", "productId": "prod_1", "apiBase": creemProdAPIBase})
	require.ErrorContains(t, err, "does not match the API key environment")

	_, err = NewCreem("1", map[string]string{"apiKey": "creem_test_x", "webhookSecret": "s", "productId": "prod_1", "currency": "CNY"})
	require.ErrorContains(t, err, "USD or EUR")

	prov, err := NewCreem("1", map[string]string{"apiKey": "creem_test_x", "webhookSecret": "s", "productId": "prod_1"})
	require.NoError(t, err)
	require.Equal(t, payment.TypeCreem, prov.ProviderKey())
	require.Equal(t, []payment.PaymentType{payment.TypeCreem}, prov.SupportedTypes())
	require.Equal(t, creemTestAPIBase, prov.config["apiBase"])
	require.Equal(t, "USD", prov.config["currency"])

	live, err := NewCreem("1", map[string]string{"apiKey": "creem_live", "webhookSecret": "s", "productId": "prod_1", "apiBase": "https://api.creem.io/v1/"})
	require.NoError(t, err)
	require.Equal(t, creemProdAPIBase, live.config["apiBase"])
}

func TestCreemCreatePaymentSendsCustomPriceAndOrderID(t *testing.T) {
	t.Parallel()

	var checkoutBody map[string]any
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "creem_test_key", r.Header.Get("x-api-key"))
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v1/products/prod_1":
			_, _ = w.Write([]byte(creemTestProductJSON))
		case r.Method == http.MethodPost && r.URL.Path == "/v1/checkouts":
			body, err := io.ReadAll(r.Body)
			require.NoError(t, err)
			require.NoError(t, json.Unmarshal(body, &checkoutBody))
			_, _ = w.Write([]byte(`{"id":"ch_1","status":"pending","checkout_url":"https://creem.io/test/checkout/prod_1/ch_1"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	prov := mustTestCreemProvider(t, server, nil)
	resp, err := prov.CreatePayment(context.Background(), payment.CreatePaymentRequest{
		OrderID:   "sub2_order",
		Amount:    "12.34",
		ReturnURL: "https://merchant.example.com/payment/result?order_id=7",
	})
	require.NoError(t, err)
	require.Equal(t, "ch_1", resp.TradeNo)
	require.Equal(t, "https://creem.io/test/checkout/prod_1/ch_1", resp.PayURL)
	require.Equal(t, "USD", resp.Currency)
	require.Empty(t, resp.ClientSecret)

	require.Equal(t, "sub2_order", checkoutBody["request_id"])
	require.Equal(t, "prod_1", checkoutBody["product_id"])
	require.EqualValues(t, 1234, checkoutBody["custom_price"])
	require.EqualValues(t, 1, checkoutBody["units"])
	require.Equal(t, "https://merchant.example.com/payment/result?order_id=7", checkoutBody["success_url"])
	require.Equal(t, map[string]any{"order_id": "sub2_order"}, checkoutBody["metadata"])
}

func TestCreemSuccessURLDropsIPHosts(t *testing.T) {
	t.Parallel()

	require.Equal(t, "", creemSuccessURL("http://127.0.0.1:18080/payment/result?order_id=1"))
	require.Equal(t, "", creemSuccessURL("http://[::1]:8080/payment/result"))
	require.Equal(t, "", creemSuccessURL(""))
	require.Equal(t, "http://localhost:18080/payment/result", creemSuccessURL("http://localhost:18080/payment/result"))
	require.Equal(t, "https://sub2api.example.com/payment/result?order_id=1", creemSuccessURL(" https://sub2api.example.com/payment/result?order_id=1 "))
}

func TestCreemCreatePaymentRejectsAmountsCreemCannotCharge(t *testing.T) {
	t.Parallel()

	var calls atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		http.NotFound(w, r)
	}))
	defer server.Close()
	prov := mustTestCreemProvider(t, server, nil)

	for _, amount := range []string{"0.99", "1.005", "-1", "abc", "1000000.00"} {
		_, err := prov.CreatePayment(context.Background(), payment.CreatePaymentRequest{OrderID: "o", Amount: amount})
		require.Error(t, err, amount)
	}
	require.Zero(t, calls.Load(), "invalid amounts must be rejected before calling Creem")
}

func TestCreemCreatePaymentRejectsMisconfiguredProduct(t *testing.T) {
	t.Parallel()

	cases := map[string]struct {
		product string
		want    string
	}{
		"exclusive tax": {`{"id":"prod_1","status":"active","billing_type":"onetime","currency":"USD","tax_mode":"exclusive"}`, "tax_mode exclusive"},
		"recurring":     {`{"id":"prod_1","status":"active","billing_type":"recurring","currency":"USD","tax_mode":"inclusive"}`, "one-time product"},
		"currency":      {`{"id":"prod_1","status":"active","billing_type":"onetime","currency":"EUR","tax_mode":"inclusive"}`, "currency EUR does not match"},
		"archived":      {`{"id":"prod_1","status":"archived","billing_type":"onetime","currency":"USD","tax_mode":"inclusive"}`, "not active"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/v1/products/prod_1" {
					_, _ = w.Write([]byte(tc.product))
					return
				}
				t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
				http.NotFound(w, r)
			}))
			defer server.Close()
			prov := mustTestCreemProvider(t, server, map[string]string{"apiKey": "creem_test_" + name})
			_, err := prov.CreatePayment(context.Background(), payment.CreatePaymentRequest{OrderID: "o", Amount: "10.00"})
			require.ErrorContains(t, err, tc.want)
		})
	}
}

func TestCreemVerifyNotificationLegacySignature(t *testing.T) {
	t.Parallel()

	prov := mustTestCreemProvider(t, nil, nil)
	body := creemCheckoutCompletedBody("paid", 1234, 1234, "USD")
	headers := map[string]string{"creem-signature": creemLegacySignature(body, "whsec_legacy")}

	n, err := prov.VerifyNotification(context.Background(), body, headers)
	require.NoError(t, err)
	require.NotNil(t, n)
	require.Equal(t, "ch_1", n.TradeNo)
	require.Equal(t, "sub2_order", n.OrderID)
	require.Equal(t, payment.NotificationStatusSuccess, n.Status)
	require.InDelta(t, 12.34, n.Amount, 0.0001)
	require.Equal(t, "USD", n.Metadata["currency"])
	require.Equal(t, "paid", n.Metadata["status"])
	require.Equal(t, "tran_1", n.Metadata["transaction_id"])

	headers["creem-signature"] = creemLegacySignature(body, "other")
	_, err = prov.VerifyNotification(context.Background(), body, headers)
	require.ErrorContains(t, err, "invalid signature")

	_, err = prov.VerifyNotification(context.Background(), body, map[string]string{})
	require.ErrorContains(t, err, "missing creem-signature")
}

func TestCreemVerifyNotificationStandardWebhookSignature(t *testing.T) {
	t.Parallel()

	key := []byte("0123456789abcdef0123456789abcdef")
	secret := "whsec_" + base64.StdEncoding.EncodeToString(key)
	prov := mustTestCreemProvider(t, nil, map[string]string{"webhookSecret": secret})
	body := creemCheckoutCompletedBody("paid", 500, 500, "USD")

	now := time.Now()
	headers := creemStandardHeaders(body, "msg_1", now, key)
	n, err := prov.VerifyNotification(context.Background(), body, headers)
	require.NoError(t, err)
	require.Equal(t, "sub2_order", n.OrderID)

	stale := creemStandardHeaders(body, "msg_1", now.Add(-10*time.Minute), key)
	_, err = prov.VerifyNotification(context.Background(), body, stale)
	require.ErrorContains(t, err, "outside tolerance")

	tampered := creemStandardHeaders(body, "msg_1", now, key)
	_, err = prov.VerifyNotification(context.Background(), body+" ", tampered)
	require.ErrorContains(t, err, "invalid signature")
}

func TestCreemVerifyNotificationReportsDiscountedAmountAndIgnoresOtherEvents(t *testing.T) {
	t.Parallel()

	prov := mustTestCreemProvider(t, nil, nil)

	// 用了折扣码：实付低于订单金额，必须上报实付，交给履约金额核对拦截。
	body := creemCheckoutCompletedBody("paid", 1000, 800, "USD")
	n, err := prov.VerifyNotification(context.Background(), body, map[string]string{"creem-signature": creemLegacySignature(body, "whsec_legacy")})
	require.NoError(t, err)
	require.InDelta(t, 8.00, n.Amount, 0.0001)

	other := `{"id":"evt_2","eventType":"refund.created","created_at":1,"object":{"id":"ref_1"}}`
	n, err = prov.VerifyNotification(context.Background(), other, map[string]string{"creem-signature": creemLegacySignature(other, "whsec_legacy")})
	require.NoError(t, err)
	require.Nil(t, n)

	// 新版事件信封 {type, data} 同样能解析。
	envelope := `{"id":"evt_3","type":"checkout.completed","data":{"id":"ch_9","request_id":"sub2_x","order":{"id":"ord_9","transaction":"tran_9","amount":200,"amount_paid":200,"currency":"USD","status":"paid"}}}`
	n, err = prov.VerifyNotification(context.Background(), envelope, map[string]string{"creem-signature": creemLegacySignature(envelope, "whsec_legacy")})
	require.NoError(t, err)
	require.Equal(t, "ch_9", n.TradeNo)
	require.Equal(t, "sub2_x", n.OrderID)
}

func TestCreemQueryOrder(t *testing.T) {
	t.Parallel()

	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/v1/checkouts", r.URL.Path)
		switch r.URL.Query().Get("checkout_id") {
		case "ch_paid":
			_, _ = w.Write([]byte(`{"id":"ch_paid","status":"completed","request_id":"sub2_order","order":{"id":"ord_1","transaction":"tran_1","amount":1234,"amount_paid":1234,"currency":"USD","status":"paid","updated_at":"2026-10-28T10:00:00.000Z"}}`))
		case "ch_pending":
			_, _ = w.Write([]byte(`{"id":"ch_pending","status":"pending","request_id":"sub2_order"}`))
		default:
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"status":404,"message":["Checkout not found"]}`))
		}
	}))
	defer server.Close()
	prov := mustTestCreemProvider(t, server, nil)

	paid, err := prov.QueryOrder(context.Background(), "ch_paid")
	require.NoError(t, err)
	require.Equal(t, payment.ProviderStatusPaid, paid.Status)
	require.InDelta(t, 12.34, paid.Amount, 0.0001)
	require.Equal(t, "2026-10-28T10:00:00Z", paid.PaidAt)

	pending, err := prov.QueryOrder(context.Background(), "ch_pending")
	require.NoError(t, err)
	require.Equal(t, payment.ProviderStatusPending, pending.Status)

	_, err = prov.QueryOrder(context.Background(), "ch_missing")
	require.ErrorContains(t, err, "Checkout not found")
}

func TestCreemRefundIsFullAmountOnly(t *testing.T) {
	t.Parallel()

	var refundBodies []string
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v1/checkouts":
			_, _ = w.Write([]byte(`{"id":"ch_1","order":{"id":"ord_1","transaction":"tran_1","amount":1234,"amount_paid":1234,"currency":"USD","status":"paid"}}`))
		case r.Method == http.MethodGet && r.URL.Path == "/v1/transactions":
			require.Equal(t, "tran_1", r.URL.Query().Get("transaction_id"))
			_, _ = w.Write([]byte(`{"id":"tran_1","status":"paid","amount":1234}`))
		case r.Method == http.MethodPost && r.URL.Path == "/v1/refunds":
			body, _ := io.ReadAll(r.Body)
			refundBodies = append(refundBodies, string(body))
			// 真实响应只有 status，没有退款 id（2026-10 在 test-api.creem.io 实测）。
			_, _ = w.Write([]byte(`{"status":"succeeded"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	prov := mustTestCreemProvider(t, server, nil)

	_, err := prov.Refund(context.Background(), payment.RefundRequest{TradeNo: "ch_1", Amount: "5.00"})
	require.ErrorContains(t, err, "only full refunds")
	require.Empty(t, refundBodies)

	resp, err := prov.Refund(context.Background(), payment.RefundRequest{TradeNo: "ch_1", Amount: "12.34"})
	require.NoError(t, err)
	require.Equal(t, "tran_1", resp.RefundID)
	require.Equal(t, payment.ProviderStatusSuccess, resp.Status)
	require.Equal(t, []string{`{"transaction_id":"tran_1"}`}, refundBodies)

	require.Equal(t, payment.ProviderStatusPending, creemRefundProviderStatus("requiresAction"))
	require.Equal(t, payment.ProviderStatusFailed, creemRefundProviderStatus("canceled"))
}

func TestCreemRefundTreatsAlreadyRefundedTransactionAsSuccess(t *testing.T) {
	t.Parallel()

	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v1/checkouts":
			_, _ = w.Write([]byte(`{"id":"ch_1","order":{"id":"ord_1","transaction":"tran_1","amount":1234,"amount_paid":1234,"currency":"USD","status":"paid"}}`))
		case r.Method == http.MethodGet && r.URL.Path == "/v1/transactions":
			_, _ = w.Write([]byte(`{"id":"tran_1","status":"refunded","amount":1234,"refunded_amount":1234}`))
		case r.Method == http.MethodPost && r.URL.Path == "/v1/refunds":
			t.Errorf("must not re-refund an already refunded transaction")
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"status":400,"message":"Transaction has no remaining amount to refund."}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	prov := mustTestCreemProvider(t, server, nil)

	resp, err := prov.Refund(context.Background(), payment.RefundRequest{TradeNo: "ch_1", Amount: "12.34"})
	require.NoError(t, err)
	require.Equal(t, "tran_1", resp.RefundID)
	require.Equal(t, payment.ProviderStatusSuccess, resp.Status)
}

func TestCreemQueryRefundReportsTransactionState(t *testing.T) {
	t.Parallel()

	txStatus := "paid"
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/checkouts":
			require.Equal(t, "ch_1", r.URL.Query().Get("checkout_id"))
			_, _ = w.Write([]byte(`{"id":"ch_1","order":{"id":"ord_1","transaction":"tran_1","amount":1000,"amount_paid":1000,"currency":"USD","status":"paid"}}`))
		case "/v1/transactions":
			require.Equal(t, "tran_1", r.URL.Query().Get("transaction_id"))
			_, _ = w.Write([]byte(`{"id":"tran_1","status":"` + txStatus + `"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	prov := mustTestCreemProvider(t, server, nil)

	resp, err := prov.QueryRefund(context.Background(), payment.RefundQueryRequest{TradeNo: "ch_1"})
	require.NoError(t, err)
	require.Equal(t, payment.ProviderStatusPending, resp.Status, "a refund not yet reflected on the transaction stays pending")

	txStatus = "refunded"
	resp, err = prov.QueryRefund(context.Background(), payment.RefundQueryRequest{TradeNo: "ch_1"})
	require.NoError(t, err)
	require.Equal(t, payment.ProviderStatusSuccess, resp.Status)
	require.Equal(t, "tran_1", resp.RefundID)
}

func TestCreemRefundSettlesWhenTransactionTurnsRefundedShortlyAfter(t *testing.T) {
	creemRefundRecheckInterval = time.Millisecond
	t.Cleanup(func() { creemRefundRecheckInterval = time.Second })

	txReads := 0
	refundPosted := false
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/v1/checkouts":
			_, _ = w.Write([]byte(`{"id":"ch_1","order":{"id":"ord_1","transaction":"tran_1","amount":1000,"amount_paid":1000,"currency":"USD","status":"paid"}}`))
		case r.URL.Path == "/v1/transactions":
			txReads++
			status := "paid"
			// 发起退款后第二次复查才看到 refunded，模拟 Creem 异步处理。
			if refundPosted && txReads >= 3 {
				status = "refunded"
			}
			_, _ = w.Write([]byte(`{"id":"tran_1","status":"` + status + `"}`))
		case r.Method == http.MethodPost && r.URL.Path == "/v1/refunds":
			refundPosted = true
			_, _ = w.Write([]byte(`{"status":"pending"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	prov := mustTestCreemProvider(t, server, nil)

	resp, err := prov.Refund(context.Background(), payment.RefundRequest{TradeNo: "ch_1", Amount: "10.00"})
	require.NoError(t, err)
	require.Equal(t, payment.ProviderStatusSuccess, resp.Status, "pending from /v1/refunds is re-checked against the transaction")
	require.Equal(t, 3, txReads, "one idempotency check before, two re-checks after")
}

func mustTestCreemProvider(t *testing.T, server *httptest.Server, overrides map[string]string) *Creem {
	t.Helper()
	cfg := map[string]string{
		"apiKey":        "creem_test_key",
		"webhookSecret": "whsec_legacy",
		"productId":     "prod_1",
	}
	for k, v := range overrides {
		cfg[k] = v
	}
	prov, err := NewCreem("1", cfg)
	require.NoError(t, err)
	if server != nil {
		prov.config["apiBase"] = server.URL
		prov.httpClient = server.Client()
	}
	return prov
}

func creemCheckoutCompletedBody(orderStatus string, amount, amountPaid int64, currency string) string {
	return `{"id":"evt_1","eventType":"checkout.completed","created_at":1793201489000,"object":{"id":"ch_1","object":"checkout","request_id":"sub2_order","status":"completed","product":{"id":"prod_1"},"metadata":{"order_id":"sub2_order"},"order":{"id":"ord_1","transaction":"tran_1","amount":` +
		strconv.FormatInt(amount, 10) + `,"amount_paid":` + strconv.FormatInt(amountPaid, 10) + `,"currency":"` + currency + `","status":"` + orderStatus + `"}}}`
}

func creemLegacySignature(body, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(body))
	return hex.EncodeToString(mac.Sum(nil))
}

func creemStandardHeaders(body, id string, at time.Time, key []byte) map[string]string {
	ts := strconv.FormatInt(at.Unix(), 10)
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write([]byte(id + "." + ts + "." + body))
	return map[string]string{
		"webhook-id":        id,
		"webhook-timestamp": ts,
		"webhook-signature": "v1,bogus v1," + base64.StdEncoding.EncodeToString(mac.Sum(nil)),
	}
}
