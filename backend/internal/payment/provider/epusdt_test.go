package provider

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/payment"
)

func newTestEpusdt(t *testing.T, apiBase string, overrides map[string]string) *Epusdt {
	t.Helper()
	cfg := map[string]string{
		"pid":       "1000",
		"secretKey": "epusdt_secret_key",
		"apiBase":   apiBase,
		"notifyUrl": "https://merchant.example/api/v1/payment/webhook/epusdt",
		"returnUrl": "https://merchant.example/payment/result",
	}
	for k, v := range overrides {
		cfg[k] = v
	}
	e, err := NewEpusdt("inst-1", cfg)
	if err != nil {
		t.Fatalf("NewEpusdt: %v", err)
	}
	return e
}

func TestEpusdtSignDeterministicAndExcludesSignature(t *testing.T) {
	t.Parallel()

	params := map[string]string{
		"pid":       "1000",
		"order_id":  "ORD1",
		"amount":    "100",
		"currency":  "cny",
		"signature": "should_be_ignored",
		"empty":     "",
	}
	key := "secret"
	s1 := epusdtSign(params, key)
	s2 := epusdtSign(params, key)
	if s1 != s2 {
		t.Fatalf("epusdtSign not deterministic: %q != %q", s1, s2)
	}
	if len(s1) != 32 {
		t.Fatalf("MD5 hex should be 32 chars, got %d", len(s1))
	}
	// Removing signature and empty must not change the signature.
	stripped := map[string]string{"pid": "1000", "order_id": "ORD1", "amount": "100", "currency": "cny"}
	if epusdtSign(stripped, key) != s1 {
		t.Fatal("signature and empty values must be excluded from signing")
	}
}

func TestEpusdtSignMatchesKnownVector(t *testing.T) {
	t.Parallel()
	// Mirrors the API.md GMPay example: same params + secret must MD5 to the
	// documented signature, proving byte-for-byte parity with epusdt.
	params := map[string]string{
		"pid":          "1000",
		"order_id":     "ORD202605230001",
		"currency":     "cny",
		"token":        "usdt",
		"network":      "tron",
		"amount":       "100",
		"notify_url":   "https://merchant.example/notify",
		"redirect_url": "https://merchant.example/return",
		"name":         "VIP",
	}
	got := epusdtSign(params, "epusdt_secret_key")
	const want = "476412c422f4dd75c3d533f5c47a9cac"
	if got != want {
		t.Fatalf("epusdtSign = %q, want %q", got, want)
	}
}

// TestEpusdtVerifyNotificationJSONNumbers is the critical test: epusdt signs the
// numeric fields as literals, then serializes them as JSON numbers. Verification
// must recompute the same signature by reading those literals via UseNumber.
func TestEpusdtVerifyNotificationJSONNumbers(t *testing.T) {
	t.Parallel()

	key := "epusdt_secret_key"
	signable := map[string]string{
		"pid":                  "1000",
		"trade_id":             "20260523171652123456001",
		"order_id":             "ORD1",
		"amount":               "100",
		"actual_amount":        "14.29",
		"token":                "USDT",
		"block_transaction_id": "0xabc",
		"status":               "2",
	}
	sign := epusdtSign(signable, key)
	// amount/actual_amount/status are JSON numbers, strings stay quoted.
	rawBody := fmt.Sprintf(`{"pid":"1000","trade_id":"20260523171652123456001","order_id":"ORD1","amount":100,"actual_amount":14.29,"token":"USDT","block_transaction_id":"0xabc","status":2,"signature":%q}`, sign)

	e := &Epusdt{config: map[string]string{"pid": "1000", "secretKey": key}}
	notif, err := e.VerifyNotification(context.Background(), rawBody, nil)
	if err != nil {
		t.Fatalf("VerifyNotification: %v", err)
	}
	if notif.Status != payment.NotificationStatusSuccess {
		t.Fatalf("status = %q, want success", notif.Status)
	}
	if notif.OrderID != "ORD1" || notif.TradeNo != "20260523171652123456001" {
		t.Fatalf("order/trade = %q/%q", notif.OrderID, notif.TradeNo)
	}
	if notif.Amount != 100 {
		t.Fatalf("amount = %v, want 100", notif.Amount)
	}
	if notif.Metadata["pid"] != "1000" {
		t.Fatalf("pid metadata = %q", notif.Metadata["pid"])
	}
}

func TestEpusdtVerifyNotificationInvalidSignature(t *testing.T) {
	t.Parallel()
	e := &Epusdt{config: map[string]string{"pid": "1000", "secretKey": "k"}}
	rawBody := `{"pid":"1000","order_id":"ORD1","status":2,"signature":"deadbeef"}`
	if _, err := e.VerifyNotification(context.Background(), rawBody, nil); err == nil {
		t.Fatal("expected invalid signature error")
	}
}

func TestEpusdtVerifyNotificationNonPaidStatus(t *testing.T) {
	t.Parallel()
	key := "k"
	signable := map[string]string{"pid": "1000", "order_id": "ORD1", "status": "3"}
	sign := epusdtSign(signable, key)
	rawBody := fmt.Sprintf(`{"pid":"1000","order_id":"ORD1","status":3,"signature":%q}`, sign)
	e := &Epusdt{config: map[string]string{"pid": "1000", "secretKey": key}}
	notif, err := e.VerifyNotification(context.Background(), rawBody, nil)
	if err != nil {
		t.Fatalf("VerifyNotification: %v", err)
	}
	if notif.Status != payment.ProviderStatusFailed {
		t.Fatalf("status = %q, want failed", notif.Status)
	}
}

func TestNewEpusdtTokenNetworkXor(t *testing.T) {
	t.Parallel()
	_, err := NewEpusdt("i", map[string]string{
		"pid": "1000", "secretKey": "k", "apiBase": "https://pay.example.com",
		"notifyUrl": "https://m/notify", "token": "usdt",
	})
	if err == nil {
		t.Fatal("expected error when token set but network empty")
	}
}

func TestNewEpusdtAPIBaseNormalized(t *testing.T) {
	t.Parallel()
	e := newTestEpusdt(t, "https://pay.example.com/payments/gmpay/v1/order/create-transaction?x=1", nil)
	if got := e.apiBase(); got != "https://pay.example.com" {
		t.Fatalf("apiBase = %q, want https://pay.example.com", got)
	}
}

func TestEpusdtCreatePayment(t *testing.T) {
	t.Parallel()
	var gotSignature, gotToken string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != epusdtCreatePath {
			t.Errorf("path = %q, want %q", r.URL.Path, epusdtCreatePath)
		}
		_ = r.ParseForm()
		gotSignature = r.PostFormValue("signature")
		gotToken = r.PostFormValue("token")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status_code":200,"message":"success","data":{"trade_id":"TID123","payment_url":"https://pay.example.com/cashier/TID123","status":1}}`))
	}))
	defer srv.Close()

	e := newTestEpusdt(t, srv.URL, map[string]string{"token": "usdt", "network": "tron"})
	resp, err := e.CreatePayment(context.Background(), payment.CreatePaymentRequest{
		OrderID: "ORD1", Amount: "100", Subject: "VIP",
	})
	if err != nil {
		t.Fatalf("CreatePayment: %v", err)
	}
	if resp.PayURL != "https://pay.example.com/cashier/TID123" || resp.TradeNo != "TID123" {
		t.Fatalf("resp = %+v", resp)
	}
	if gotToken != "usdt" {
		t.Fatalf("token sent = %q, want usdt", gotToken)
	}
	if len(gotSignature) != 32 {
		t.Fatalf("signature sent = %q", gotSignature)
	}
}

func TestEpusdtQueryOrder(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status_code":200,"data":{"trade_id":"TID123","status":2}}`))
	}))
	defer srv.Close()

	e := newTestEpusdt(t, srv.URL, nil)
	resp, err := e.QueryOrder(context.Background(), "TID123")
	if err != nil {
		t.Fatalf("QueryOrder: %v", err)
	}
	if resp.Status != payment.ProviderStatusPaid {
		t.Fatalf("status = %q, want paid", resp.Status)
	}
}

func TestEpusdtRefundUnsupported(t *testing.T) {
	t.Parallel()
	e := newTestEpusdt(t, "https://pay.example.com", nil)
	if _, err := e.Refund(context.Background(), payment.RefundRequest{OrderID: "ORD1", Amount: "1"}); err == nil {
		t.Fatal("expected refund unsupported error")
	}
}
