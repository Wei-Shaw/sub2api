package provider

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/payment"
)

const testIPayeesKey = "secret-key-123"

// newIPayeesTestServer spins up a fake iPayees API. verifyStatus controls what
// verify-payments reports so tests can decouple the webhook body from the
// authoritative status.
func newIPayeesTestServer(t *testing.T, verifyStatus string, capturedCreate *map[string]any) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/api/create-charge", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("ip-ipayees-api-key") != testIPayeesKey {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"status":false,"message":"Unauthorized"}`))
			return
		}
		if capturedCreate != nil {
			body, _ := io.ReadAll(r.Body)
			var m map[string]any
			_ = json.Unmarshal(body, &m)
			*capturedCreate = m
		}
		_, _ = w.Write([]byte(`{"status":true,"id":"180599","url":"https://gw.example/payment/180599"}`))
	})
	mux.HandleFunc("/api/verify-payments", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("ip-ipayees-api-key") != testIPayeesKey {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_, _ = w.Write([]byte(`{"status":"` + verifyStatus + `","transaction_id":"TXN-1","amount":"100.00","currency":"BDT","method":"bKash"}`))
	})
	return httptest.NewServer(mux)
}

func newTestIPayees(t *testing.T, base string) *IPayees {
	t.Helper()
	p, err := NewIPayees("1", map[string]string{
		"apiBase":  base + "/api",
		"apiKey":   testIPayeesKey,
		"currency": "BDT",
		"contact":  "billing@example.com",
	})
	if err != nil {
		t.Fatalf("NewIPayees: %v", err)
	}
	return p
}

func TestIPayeesCreatePayment(t *testing.T) {
	var captured map[string]any
	srv := newIPayeesTestServer(t, ipayeesStatusCompleted, &captured)
	defer srv.Close()
	p := newTestIPayees(t, srv.URL)

	resp, err := p.CreatePayment(context.Background(), payment.CreatePaymentRequest{
		OrderID:   "ORD-1",
		Amount:    "100.00",
		Subject:   "Top up",
		NotifyURL: "https://me.example/api/v1/payment/webhook/ipayees",
		ReturnURL: "https://me.example/payment/result",
	})
	if err != nil {
		t.Fatalf("CreatePayment: %v", err)
	}
	if resp.TradeNo != "180599" || resp.PayURL != "https://gw.example/payment/180599" {
		t.Fatalf("unexpected resp: %+v", resp)
	}
	// The invoiceid we send MUST be our internal order id (used to reconcile later).
	meta, _ := captured["metadata"].(map[string]any)
	if meta == nil || meta["invoiceid"] != "ORD-1" {
		t.Fatalf("metadata.invoiceid not sent correctly: %v", captured["metadata"])
	}
	for _, k := range []string{"amount", "currency", "webhook_url", "redirect_url", "email_mobile"} {
		if _, ok := captured[k]; !ok {
			t.Fatalf("create-charge missing field %q; body=%v", k, captured)
		}
	}
}

func TestIPayeesQueryOrderPaid(t *testing.T) {
	srv := newIPayeesTestServer(t, ipayeesStatusCompleted, nil)
	defer srv.Close()
	p := newTestIPayees(t, srv.URL)

	res, err := p.QueryOrder(context.Background(), "180599")
	if err != nil {
		t.Fatalf("QueryOrder: %v", err)
	}
	if res.Status != payment.ProviderStatusPaid {
		t.Fatalf("expected paid, got %q", res.Status)
	}
	if res.Amount != 100.00 {
		t.Fatalf("expected amount 100, got %v", res.Amount)
	}
	if res.Metadata["currency"] != "BDT" {
		t.Fatalf("expected currency BDT, got %q", res.Metadata["currency"])
	}
}

func ipayeesWebhookBody(status string) string {
	return `{"id":"180599","status":"` + status + `","amount":"100.00","currency":"BDT","metadata":{"invoiceid":"ORD-1"}}`
}

func TestIPayeesVerifyNotification_RejectsBadKey(t *testing.T) {
	srv := newIPayeesTestServer(t, ipayeesStatusCompleted, nil)
	defer srv.Close()
	p := newTestIPayees(t, srv.URL)

	_, err := p.VerifyNotification(context.Background(), ipayeesWebhookBody("completed"), map[string]string{
		"ip-ipayees-api-key": "wrong-key",
	})
	if err == nil {
		t.Fatal("expected rejection for wrong api key, got nil")
	}
}

func TestIPayeesVerifyNotification_Paid(t *testing.T) {
	srv := newIPayeesTestServer(t, ipayeesStatusCompleted, nil)
	defer srv.Close()
	p := newTestIPayees(t, srv.URL)

	n, err := p.VerifyNotification(context.Background(), ipayeesWebhookBody("completed"), map[string]string{
		"ip-ipayees-api-key": testIPayeesKey,
	})
	if err != nil {
		t.Fatalf("VerifyNotification: %v", err)
	}
	if n.Status != payment.ProviderStatusSuccess {
		t.Fatalf("expected success, got %q", n.Status)
	}
	if n.OrderID != "ORD-1" {
		t.Fatalf("expected OrderID ORD-1, got %q", n.OrderID)
	}
	if n.Amount != 100.00 {
		t.Fatalf("expected amount 100, got %v", n.Amount)
	}
}

// Critical: even when the webhook body claims "completed", if the authoritative
// verify-payments call reports the payment is NOT complete, the notification must
// come back non-success. This proves the webhook body is never trusted.
func TestIPayeesVerifyNotification_UntrustedWebhookBody(t *testing.T) {
	srv := newIPayeesTestServer(t, "pending", nil)
	defer srv.Close()
	p := newTestIPayees(t, srv.URL)

	n, err := p.VerifyNotification(context.Background(), ipayeesWebhookBody("completed"), map[string]string{
		"ip-ipayees-api-key": testIPayeesKey,
	})
	if err != nil {
		t.Fatalf("VerifyNotification: %v", err)
	}
	if n.Status == payment.ProviderStatusSuccess {
		t.Fatal("webhook body was trusted: reported success while verify-payments said pending")
	}
}

func TestIPayeesNormalizeAPIBase(t *testing.T) {
	cases := map[string]string{
		"https://gw.example/api":                "https://gw.example/api",
		"https://gw.example/api/":               "https://gw.example/api",
		"https://gw.example/api/create-charge":  "https://gw.example/api",
		"https://gw.example/api/verify-payments": "https://gw.example/api",
	}
	for in, want := range cases {
		if got := normalizeIPayeesAPIBase(in); got != want {
			t.Fatalf("normalizeIPayeesAPIBase(%q)=%q want %q", in, got, want)
		}
	}
	if strings.TrimSpace(normalizeIPayeesAPIBase("not a url")) != "" {
		t.Fatal("expected empty for invalid url")
	}
}
