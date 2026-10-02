package payment

import (
	"regexp"
	"strings"
)

// Bank-transfer gateways (GPM Pay) carry no order of their own: the order is
// identified only by a code the payer types into the transfer memo. Banks
// routinely uppercase that memo and strip punctuation, so the code is the
// out_trade_no ("sub2_20260930aB3kX9mQ") without the underscore, uppercased,
// and recovering it yields an out_trade_no whose case may differ from the
// stored one. Order lookups from a memo must therefore be case-insensitive.

// transferCodePattern matches the code anywhere in a memo; banks prepend and
// append their own text, sometimes without a separator.
var transferCodePattern = regexp.MustCompile(`(?i)SUB2_?(\d{8}[A-Z0-9]{8})`)

// TransferCodeForOutTradeNo returns the memo code a payer must enter for an order.
func TransferCodeForOutTradeNo(outTradeNo string) string {
	return strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(outTradeNo), "_", ""))
}

// OutTradeNoFromTransferContent recovers an out_trade_no from a transfer memo,
// or returns "" when the memo carries no order code.
func OutTradeNoFromTransferContent(content string) string {
	m := transferCodePattern.FindStringSubmatch(content)
	if m == nil {
		return ""
	}
	return "sub2_" + m[1]
}
