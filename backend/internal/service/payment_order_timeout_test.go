//go:build unit

package service

import (
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/payment"
)

// A crypto order that expires mid-confirmation is only recoverable inside
// toPaid's few minutes of grace, so the bank-shaped global timeout does not get
// to close it that early.
func TestOrderTimeoutMinutes(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name        string
		configured  int
		paymentType string
		want        int
	}{
		{"crypto floors a short global window", 15, payment.TypeNowPayments, minCryptoOrderTimeoutMin},
		{"crypto keeps a longer configured window", 180, payment.TypeNowPayments, 180},
		{"bank transfer obeys the configured window", 15, payment.TypeSePay, 15},
		{"unset falls back to the default", 0, payment.TypeSePay, defaultOrderTimeoutMin},
		{"unset still floors crypto", 0, payment.TypeNowPayments, minCryptoOrderTimeoutMin},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := orderTimeoutMinutes(&PaymentConfig{OrderTimeoutMin: tc.configured}, tc.paymentType)
			if got != tc.want {
				t.Fatalf("orderTimeoutMinutes(%d, %q) = %d, want %d", tc.configured, tc.paymentType, got, tc.want)
			}
		})
	}

	if got := orderTimeoutMinutes(nil, payment.TypeSePay); got != defaultOrderTimeoutMin {
		t.Fatalf("orderTimeoutMinutes(nil) = %d, want %d", got, defaultOrderTimeoutMin)
	}
}

// The reference goes into a provider request path, so it is held to the shape
// providers actually issue.
func TestIsSafePaymentReference(t *testing.T) {
	t.Parallel()

	valid := []string{"5106102174", "abc-123_DEF"}
	for _, reference := range valid {
		if !isSafePaymentReference(reference) {
			t.Fatalf("isSafePaymentReference(%q) = false, want true", reference)
		}
	}

	invalid := []string{"", "../../v1/payout", "51061 02174", "5106102174?x=1", "a/b", strings.Repeat("9", maxPaymentReferenceLen+1)}
	for _, reference := range invalid {
		if isSafePaymentReference(reference) {
			t.Fatalf("isSafePaymentReference(%q) = true, want false", reference)
		}
	}
}
