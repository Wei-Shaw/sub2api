package payment

import (
	"regexp"
	"strings"
)

// OrderCodePrefix is the fixed prefix of every generated out_trade_no.
//
// The code has to survive a round trip through a Vietnamese bank transfer
// description, which is why it is uppercase alphanumeric only: banks routinely
// uppercase the content and drop punctuation, so anything else would come back
// from the SePay webhook in a form we could no longer match to an exact order.
const OrderCodePrefix = "SUB2"

// OrderCodeRandomLength is the number of random characters appended after the
// prefix and the YYYYMMDD date segment.
const OrderCodeRandomLength = 8

// OrderCodeAlphabet is the character set the random segment draws from.
const OrderCodeAlphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

// orderCodePattern matches a full order code inside normalized transfer text.
var orderCodePattern = regexp.MustCompile(OrderCodePrefix + `[0-9]{8}[0-9A-Z]{8}`)

// NormalizeTransferContent strips everything a bank may add to or remove from a
// transfer description, leaving uppercase alphanumerics only.
func NormalizeTransferContent(raw string) string {
	var b strings.Builder
	b.Grow(len(raw))
	for _, ch := range strings.ToUpper(raw) {
		if (ch >= '0' && ch <= '9') || (ch >= 'A' && ch <= 'Z') {
			b.WriteRune(ch)
		}
	}
	return b.String()
}

// ExtractOrderCode returns the order code embedded in a bank transfer
// description, or an empty string when the text carries none.
func ExtractOrderCode(raw string) string {
	return orderCodePattern.FindString(NormalizeTransferContent(raw))
}
