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
