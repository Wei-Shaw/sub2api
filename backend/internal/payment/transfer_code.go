package payment

import "strings"

// NormalizeTransferCode canonicalizes a bank-transfer payment code for
// matching: uppercase, keep only letters and digits. Banks and SePay's code
// extraction may drop separators (the sub2_ underscore) or change case.
func NormalizeTransferCode(code string) string {
	var b strings.Builder
	for _, r := range strings.ToUpper(strings.TrimSpace(code)) {
		if (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// StripTransferSeparators removes separator characters from a transfer code,
// preserving case: sub2_20260815ZPbOX0Kl -> sub220260815ZPbOX0Kl. SePay's
// payment-code extraction treats codes as contiguous alphanumeric strings and
// prefix matching is case-insensitive, so the lowercase prefix and the mixed-
// case suffix pass extraction exactly as typed.
func StripTransferSeparators(code string) string {
	var b strings.Builder
	for _, r := range strings.TrimSpace(code) {
		if (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		}
	}
	return b.String()
}
