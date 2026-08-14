package provider

import (
	"context"
	"crypto/hmac"
	"crypto/sha512"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/payment"
)

const testIPNSecret = "ipn-secret"

// NOWPayments refuses an invoice whose redirect URL runs past 255 characters
// with a bare HTTP 500 that names no field, so the limit is caught locally and
// the offending field named before the request goes out.
func TestCreatePaymentRejectsAnOverLongRedirectURL(t *testing.T) {
	p := newTestNOWPayments(t)
	p.httpClient = &http.Client{Transport: roundTripperFunc(func(*http.Request) (*http.Response, error) {
		t.Error("CreatePayment called upstream with a redirect URL it should have rejected")
		return nil, io.EOF
	})}

	_, err := p.CreatePayment(context.Background(), payment.CreatePaymentRequest{
		OrderID:   "sub2_20260814ABCD1234",
		Amount:    "10.00",
		Subject:   "Sub2API 10.00 USD",
		ReturnURL: "https://panel.example.com/payment/result?resume_token=" + strings.Repeat("a", 256),
	})
	if err == nil {
		t.Fatal("CreatePayment accepted an over-long success_url")
	}
	if !strings.Contains(err.Error(), "success_url") {
		t.Fatalf("error = %v, want it to name success_url", err)
	}
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func newTestNOWPayments(t *testing.T) *NOWPayments {
	t.Helper()
	p, err := NewNOWPayments("1", map[string]string{
		"apiKey":    "api-key",
		"ipnSecret": testIPNSecret,
	})
	if err != nil {
		t.Fatalf("NewNOWPayments() error: %v", err)
	}
	return p
}

func signIPN(t *testing.T, body string) string {
	t.Helper()
	canonical, err := nowPaymentsCanonicalJSON(body)
	if err != nil {
		t.Fatalf("nowPaymentsCanonicalJSON() error: %v", err)
	}
	mac := hmac.New(sha512.New, []byte(testIPNSecret))
	mac.Write(canonical)
	return hex.EncodeToString(mac.Sum(nil))
}

func TestNewNOWPaymentsDefaultsToUSD(t *testing.T) {
	t.Parallel()

	if got := newTestNOWPayments(t).currency(); got != payment.CurrencyNowPayments {
		t.Fatalf("currency() = %q, want %q", got, payment.CurrencyNowPayments)
	}
}

// NOWPayments signs the body with its object keys sorted, so a signature only
// verifies if we reproduce that ordering rather than the wire ordering.
func TestNOWPaymentsCanonicalJSONSortsKeysAndKeepsNumberLiterals(t *testing.T) {
	t.Parallel()

	canonical, err := nowPaymentsCanonicalJSON(`{"b":1.0,"a":{"d":2,"c":[3,{"f":4,"e":5}]}}`)
	if err != nil {
		t.Fatalf("nowPaymentsCanonicalJSON() error: %v", err)
	}
	want := `{"a":{"c":[3,{"e":5,"f":4}],"d":2},"b":1.0}`
	if string(canonical) != want {
		t.Fatalf("canonical = %s, want %s", canonical, want)
	}
}

func TestNOWPaymentsVerifyNotification(t *testing.T) {
	t.Parallel()

	finished := `{"payment_status":"finished","payment_id":5745459419,"invoice_id":4942419698,` +
		`"order_id":"SUB220260101ABCD1234","price_amount":10.5,"price_currency":"usd"}`

	t.Run("accepts a finished payment", func(t *testing.T) {
		t.Parallel()
		notification, err := newTestNOWPayments(t).VerifyNotification(context.Background(), finished,
			map[string]string{nowPaymentsSignatureHeader: signIPN(t, finished)})
		if err != nil {
			t.Fatalf("VerifyNotification() error: %v", err)
		}
		if notification == nil {
			t.Fatal("VerifyNotification() returned no notification")
		}
		if notification.OrderID != "SUB220260101ABCD1234" {
			t.Fatalf("OrderID = %q", notification.OrderID)
		}
		if notification.TradeNo != "5745459419" {
			t.Fatalf("TradeNo = %q, want the payment id", notification.TradeNo)
		}
		if notification.Amount != 10.5 {
			t.Fatalf("Amount = %v, want 10.5", notification.Amount)
		}
		if notification.Status != payment.ProviderStatusSuccess {
			t.Fatalf("Status = %q, want success", notification.Status)
		}
	})

	t.Run("rejects a forged signature", func(t *testing.T) {
		t.Parallel()
		_, err := newTestNOWPayments(t).VerifyNotification(context.Background(), finished,
			map[string]string{nowPaymentsSignatureHeader: "deadbeef"})
		if err == nil {
			t.Fatal("VerifyNotification() should reject a bad signature")
		}
	})

	t.Run("rejects a missing signature", func(t *testing.T) {
		t.Parallel()
		if _, err := newTestNOWPayments(t).VerifyNotification(context.Background(), finished, nil); err == nil {
			t.Fatal("VerifyNotification() should reject an unsigned callback")
		}
	})

	// Crediting on "confirming" would hand over goods for a payment the chain
	// can still drop.
	t.Run("ignores statuses still in flight", func(t *testing.T) {
		t.Parallel()
		for _, status := range []string{"waiting", "confirming", "confirmed", "sending", "partially_paid"} {
			body := `{"payment_status":"` + status + `","payment_id":1,"order_id":"SUB220260101ABCD1234","price_amount":1}`
			notification, err := newTestNOWPayments(t).VerifyNotification(context.Background(), body,
				map[string]string{nowPaymentsSignatureHeader: signIPN(t, body)})
			if err != nil {
				t.Fatalf("VerifyNotification(%s) error: %v", status, err)
			}
			if notification != nil {
				t.Fatalf("status %s must not settle the order, got %+v", status, notification)
			}
		}
	})

	t.Run("fails the order on terminal failures", func(t *testing.T) {
		t.Parallel()
		for _, status := range []string{"failed", "expired", "refunded"} {
			body := `{"payment_status":"` + status + `","payment_id":1,"order_id":"SUB220260101ABCD1234","price_amount":1}`
			notification, err := newTestNOWPayments(t).VerifyNotification(context.Background(), body,
				map[string]string{nowPaymentsSignatureHeader: signIPN(t, body)})
			if err != nil {
				t.Fatalf("VerifyNotification(%s) error: %v", status, err)
			}
			if notification == nil || notification.Status != payment.ProviderStatusFailed {
				t.Fatalf("status %s should fail the order, got %+v", status, notification)
			}
		}
	})
}

// A re-deposit is settled at the rate of the day and can fall short of the
// order total, so it must never be the thing that marks an order paid.
func TestNOWPaymentsVerifyNotificationIgnoresRepeatedDeposits(t *testing.T) {
	t.Parallel()

	body := `{"payment_status":"finished","payment_id":2,"parent_payment_id":1,` +
		`"order_id":"SUB220260101ABCD1234","price_amount":10.5,"price_currency":"usd"}`
	notification, err := newTestNOWPayments(t).VerifyNotification(context.Background(), body,
		map[string]string{nowPaymentsSignatureHeader: signIPN(t, body)})
	if err != nil {
		t.Fatalf("VerifyNotification() error: %v", err)
	}
	if notification != nil {
		t.Fatalf("a child payment must not settle the order, got %+v", notification)
	}
}

// NOWPayments signs with JSON.stringify, which does not escape HTML-significant
// characters the way Go's default encoder does.
func TestNOWPaymentsCanonicalJSONDoesNotEscapeHTML(t *testing.T) {
	t.Parallel()

	canonical, err := nowPaymentsCanonicalJSON(`{"order_description":"Tom & Jerry <b>"}`)
	if err != nil {
		t.Fatalf("nowPaymentsCanonicalJSON() error: %v", err)
	}
	want := `{"order_description":"Tom & Jerry <b>"}`
	if string(canonical) != want {
		t.Fatalf("canonical = %s, want %s", canonical, want)
	}
}

// newNOWPaymentsServer points a provider at a stub upstream.
func newNOWPaymentsServer(t *testing.T, handler http.HandlerFunc) *NOWPayments {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	p, err := NewNOWPayments("1", map[string]string{
		"apiKey":    "api-key",
		"ipnSecret": testIPNSecret,
		"apiBase":   server.URL,
	})
	if err != nil {
		t.Fatalf("NewNOWPayments() error: %v", err)
	}
	return p
}

// price_amount is typed as a number upstream, so a quoted string is rejected.
func TestNOWPaymentsCreatePaymentSendsNumericPrice(t *testing.T) {
	t.Parallel()

	var body map[string]any
	p := newNOWPaymentsServer(t, func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(raw, &body); err != nil {
			t.Errorf("request body is not JSON: %v", err)
		}
		_, _ = w.Write([]byte(`{"id":"4942419698","invoice_url":"https://nowpayments.io/payment/?iid=4942419698"}`))
	})

	if _, err := p.CreatePayment(context.Background(), payment.CreatePaymentRequest{
		OrderID: "SUB220260101ABCD1234",
		Amount:  "10.50",
	}); err != nil {
		t.Fatalf("CreatePayment() error: %v", err)
	}
	if _, ok := body["price_amount"].(float64); !ok {
		t.Fatalf("price_amount = %#v, want a JSON number", body["price_amount"])
	}
}

// The identifier held after checkout is an invoice ID, which the per-payment
// endpoint rejects; without the listing fallback an order never reconciles.
func TestNOWPaymentsQueryOrderFallsBackToInvoiceLookup(t *testing.T) {
	t.Parallel()

	// invoiceId is not a documented filter on the listing endpoint, so the
	// fixture answers with a foreign payment first: an implementation that
	// trusts the filter would settle this order against someone else's money.
	listing := `{"data":[` +
		`{"payment_id":9,"invoice_id":7777777777,"payment_status":"finished","price_amount":999,"price_currency":"usd"},` +
		`{"payment_id":2,"invoice_id":4942419698,"parent_payment_id":1,"payment_status":"finished","price_amount":3,"price_currency":"usd"},` +
		`{"payment_id":1,"invoice_id":4942419698,"payment_status":"finished","price_amount":10.5,"price_currency":"usd",` +
		`"updated_at":"2026-01-01T00:00:00Z"}],"total":3}`

	t.Run("resolves the original payment", func(t *testing.T) {
		t.Parallel()
		p := newNOWPaymentsServer(t, func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Query().Get("invoiceId") != "4942419698" {
				http.Error(w, `{"message":"payment not found"}`, http.StatusNotFound)
				return
			}
			_, _ = w.Write([]byte(listing))
		})
		resp, err := p.QueryOrder(context.Background(), "4942419698")
		if err != nil {
			t.Fatalf("QueryOrder() error: %v", err)
		}
		if resp.Status != payment.ProviderStatusPaid {
			t.Fatalf("Status = %q, want paid", resp.Status)
		}
		// A foreign payment and the re-deposit are both listed first; settling
		// on them would credit 999 USD or 3 USD.
		if resp.Amount != 10.5 {
			t.Fatalf("Amount = %v, want the original payment's 10.5", resp.Amount)
		}
		if resp.TradeNo != "1" {
			t.Fatalf("TradeNo = %q, want the original payment id", resp.TradeNo)
		}
	})

	t.Run("stays pending when the listing holds no payment for this invoice", func(t *testing.T) {
		t.Parallel()
		foreign := `{"data":[{"payment_id":9,"invoice_id":7777777777,"payment_status":"finished",` +
			`"price_amount":999,"price_currency":"usd"}],"total":1}`
		p := newNOWPaymentsServer(t, func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Query().Get("invoiceId") != "" {
				_, _ = w.Write([]byte(foreign))
				return
			}
			http.Error(w, `{"message":"payment not found"}`, http.StatusNotFound)
		})
		resp, err := p.QueryOrder(context.Background(), "4942419698")
		if err != nil {
			t.Fatalf("QueryOrder() error: %v", err)
		}
		if resp.Status != payment.ProviderStatusPending {
			t.Fatalf("Status = %q, want pending — the listed payment belongs to another invoice", resp.Status)
		}
	})

	// The listing needs a bearer token minted from dashboard credentials, which
	// an API key alone does not buy. A 401 must not read as "not paid".
	t.Run("reports the listing's own auth requirement", func(t *testing.T) {
		t.Parallel()
		p := newNOWPaymentsServer(t, func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Query().Get("invoiceId") != "" {
				http.Error(w, `{"message":"Unauthorized"}`, http.StatusUnauthorized)
				return
			}
			http.Error(w, `{"message":"payment not found"}`, http.StatusNotFound)
		})
		_, err := p.QueryOrder(context.Background(), "4942419698")
		if err == nil {
			t.Fatal("QueryOrder() reported no error for a 401 listing")
		}
		if !strings.Contains(err.Error(), "bearer token") {
			t.Fatalf("error = %v, want it to name the missing bearer token", err)
		}
	})

	t.Run("stays pending while the invoice has no payment", func(t *testing.T) {
		t.Parallel()
		p := newNOWPaymentsServer(t, func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Query().Get("invoiceId") != "" {
				_, _ = w.Write([]byte(`{"data":[],"total":0}`))
				return
			}
			http.Error(w, `{"message":"payment not found"}`, http.StatusNotFound)
		})
		resp, err := p.QueryOrder(context.Background(), "4942419698")
		if err != nil {
			t.Fatalf("QueryOrder() error: %v", err)
		}
		if resp.Status != payment.ProviderStatusPending {
			t.Fatalf("Status = %q, want pending", resp.Status)
		}
	})
}

func TestNOWPaymentsQueryStatusMapping(t *testing.T) {
	t.Parallel()

	cases := map[string]string{
		"finished":       payment.ProviderStatusPaid,
		"failed":         payment.ProviderStatusFailed,
		"expired":        payment.ProviderStatusFailed,
		"refunded":       payment.ProviderStatusRefunded,
		"waiting":        payment.ProviderStatusPending,
		"confirming":     payment.ProviderStatusPending,
		"partially_paid": payment.ProviderStatusPending,
		"something_new":  payment.ProviderStatusPending,
	}
	for raw, want := range cases {
		if got := nowPaymentsQueryStatus(raw); got != want {
			t.Fatalf("nowPaymentsQueryStatus(%q) = %q, want %q", raw, got, want)
		}
	}
}
