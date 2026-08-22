package payment

import "testing"

func TestGetBasePaymentTypeSePay(t *testing.T) {
	if got := GetBasePaymentType("sepay"); got != TypeSePay {
		t.Fatalf("GetBasePaymentType(sepay) = %q, want %q", got, TypeSePay)
	}
	if got := GetBasePaymentType(string(TypeSePay)); got != TypeSePay {
		t.Fatalf("GetBasePaymentType(TypeSePay) = %q, want %q", got, TypeSePay)
	}
}
