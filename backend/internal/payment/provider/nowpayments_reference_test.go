package provider

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/payment"
)

// The payment id arrives on the checkout redirect, which is to say from the
// browser's address bar. It is worth something only once upstream agrees it is
// this order's payment.
func TestNOWPaymentsVerifyPaymentReference(t *testing.T) {
	t.Parallel()

	const outTradeNo = "SUB220260814YC8X49F4"
	payments := map[string]string{
		"5106102174": `{"payment_id":5106102174,"invoice_id":4942419698,"payment_status":"finished",` +
			`"order_id":"` + outTradeNo + `","price_amount":10,"price_currency":"usd",` +
			`"updated_at":"2026-08-14T15:04:05Z"}`,
		"5106102175": `{"payment_id":5106102175,"invoice_id":1111111111,"payment_status":"finished",` +
			`"order_id":"SUB220260814OTHER99","price_amount":500,"price_currency":"usd"}`,
		"5106102176": `{"payment_id":5106102176,"parent_payment_id":5106102174,"invoice_id":4942419698,` +
			`"payment_status":"finished","order_id":"` + outTradeNo + `","price_amount":1,"price_currency":"usd"}`,
	}
	newServer := func(t *testing.T) *NOWPayments {
		t.Helper()
		return newNOWPaymentsServer(t, func(w http.ResponseWriter, r *http.Request) {
			if r.URL.RawQuery == "" {
				if body, ok := payments[strings.TrimPrefix(r.URL.Path, "/payment/")]; ok {
					_, _ = w.Write([]byte(body))
					return
				}
			}
			http.Error(w, `{"message":"payment not found"}`, http.StatusNotFound)
		})
	}

	t.Run("accepts the order's own payment", func(t *testing.T) {
		t.Parallel()
		resp, err := newServer(t).VerifyPaymentReference(context.Background(), "5106102174", outTradeNo)
		if err != nil {
			t.Fatalf("VerifyPaymentReference() error: %v", err)
		}
		if resp.TradeNo != "5106102174" {
			t.Fatalf("TradeNo = %q, want the payment id", resp.TradeNo)
		}
		if resp.Status != payment.ProviderStatusPaid {
			t.Fatalf("Status = %q, want paid", resp.Status)
		}
		if resp.Amount != 10 {
			t.Fatalf("Amount = %v, want 10", resp.Amount)
		}
	})

	// Anyone holding their own resume token could name a stranger's payment id.
	t.Run("rejects a payment belonging to another order", func(t *testing.T) {
		t.Parallel()
		_, err := newServer(t).VerifyPaymentReference(context.Background(), "5106102175", outTradeNo)
		if err == nil {
			t.Fatal("VerifyPaymentReference() accepted another order's payment")
		}
		if !strings.Contains(err.Error(), "belongs to order") {
			t.Fatalf("error = %v, want it to name the mismatched order", err)
		}
	})

	// A repeated or wrong-asset deposit is settled at the rate of the day, so
	// its amount need not cover the order.
	t.Run("rejects a child payment", func(t *testing.T) {
		t.Parallel()
		_, err := newServer(t).VerifyPaymentReference(context.Background(), "5106102176", outTradeNo)
		if err == nil {
			t.Fatal("VerifyPaymentReference() accepted a child payment")
		}
		if !strings.Contains(err.Error(), "child") {
			t.Fatalf("error = %v, want it to name the parent payment", err)
		}
	})

	// QueryOrder answers an unknown id by widening to the invoice listing.
	// Verification must not: an unknown reference is simply not this order's.
	t.Run("does not widen to the invoice listing", func(t *testing.T) {
		t.Parallel()
		p := newNOWPaymentsServer(t, func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Query().Get("invoiceId") != "" {
				t.Error("VerifyPaymentReference widened an unverified reference to the invoice listing")
			}
			http.Error(w, `{"message":"payment not found"}`, http.StatusNotFound)
		})
		if _, err := p.VerifyPaymentReference(context.Background(), "404404404", outTradeNo); err == nil {
			t.Fatal("VerifyPaymentReference() accepted an unknown reference")
		}
	})
}
