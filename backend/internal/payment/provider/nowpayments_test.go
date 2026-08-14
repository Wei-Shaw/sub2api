package provider

import (
	"context"
	"crypto/hmac"
	"crypto/sha512"
	"encoding/hex"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/payment"
)

const testIPNSecret = "ipn-secret"

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
