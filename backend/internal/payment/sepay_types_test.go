//go:build unit

package payment

import "testing"

func TestSepayPaymentTypeUsesItsOwnBaseType(t *testing.T) {
	if TypeSepay != "sepay" {
		t.Fatalf("TypeSepay = %q, want sepay", TypeSepay)
	}
	if got := GetBasePaymentType(TypeSepay); got != TypeSepay {
		t.Fatalf("GetBasePaymentType(TypeSepay) = %q, want %q", got, TypeSepay)
	}
}
