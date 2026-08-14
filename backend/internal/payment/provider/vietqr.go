package provider

import (
	"fmt"
	"strings"
)

// VietQR / EMVCo QR field identifiers used by the Napas transfer scheme.
const (
	vietQRPayloadFormatID       = "00"
	vietQRInitiationMethodID    = "01"
	vietQRMerchantAccountID     = "38"
	vietQRTransactionCurrencyID = "53"
	vietQRTransactionAmountID   = "54"
	vietQRCountryCodeID         = "58"
	vietQRAdditionalDataID      = "62"

	vietQRGUIDSubID          = "00"
	vietQRBeneficiarySubID   = "01"
	vietQRServiceCodeSubID   = "02"
	vietQRAcquirerBINSubID   = "00"
	vietQRAccountNumberSubID = "01"
	vietQRPurposeSubID       = "08"

	vietQRPayloadFormat   = "01"
	vietQRDynamicMethod   = "12"
	vietQRGUID            = "A000000727"
	vietQRServiceTransfer = "QRIBFTTA"
	vietQRCurrencyVND     = "704"
	vietQRCountryVN       = "VN"
	vietQRCRCPlaceholder  = "6304"
)

// vietQRBankBINs maps the short bank codes SePay accepts to their Napas BIN.
// Admins may bypass this table entirely by configuring `bankBin` directly.
var vietQRBankBINs = map[string]string{
	"ABB":          "970425",
	"ABBANK":       "970425",
	"ACB":          "970416",
	"AGRIBANK":     "970405",
	"BAB":          "970409",
	"BACABANK":     "970409",
	"BAOVIETBANK":  "970438",
	"BIDV":         "970418",
	"BVB":          "970438",
	"BVBANK":       "970454",
	"CAKE":         "546034",
	"CBB":          "970444",
	"CBBANK":       "970444",
	"CIMB":         "422589",
	"COOPBANK":     "970446",
	"DBS":          "796500",
	"DOB":          "970406",
	"DONGABANK":    "970406",
	"EIB":          "970431",
	"EXIMBANK":     "970431",
	"GPB":          "970408",
	"GPBANK":       "970408",
	"HDB":          "970437",
	"HDBANK":       "970437",
	"HLBVN":        "970442",
	"HSBC":         "458761",
	"ICB":          "970415",
	"IVB":          "970434",
	"KIENLONGBANK": "970452",
	"KLB":          "970452",
	"LPB":          "970449",
	"LPBANK":       "970449",
	"MB":           "970422",
	"MBBANK":       "970422",
	"MSB":          "970426",
	"NAB":          "970428",
	"NAMABANK":     "970428",
	"NCB":          "970419",
	"NHB":          "801011",
	"OCB":          "970448",
	"OCEANBANK":    "970414",
	"PBVN":         "970439",
	"PGB":          "970430",
	"PGBANK":       "970430",
	"PVCOMBANK":    "970412",
	"SACOMBANK":    "970403",
	"SCB":          "970429",
	"SCVN":         "970410",
	"SEABANK":      "970440",
	"SGICB":        "970400",
	"SHB":          "970443",
	"SHBVN":        "970424",
	"STB":          "970403",
	"TCB":          "970407",
	"TECHCOMBANK":  "970407",
	"TIMO":         "963388",
	"TPB":          "970423",
	"TPBANK":       "970423",
	"UBANK":        "546035",
	"VAB":          "970427",
	"VBA":          "970405",
	"VBB":          "970433",
	"VCB":          "970436",
	"VIB":          "970441",
	"VIETABANK":    "970427",
	"VIETBANK":     "970433",
	"VIETCOMBANK":  "970436",
	"VIETINBANK":   "970415",
	"VNPTMONEY":    "971011",
	"VPB":          "970432",
	"VPBANK":       "970432",
	"VRB":          "970421",
	"VTLMONEY":     "971005",
	"WVN":          "970457",
}

// resolveVietQRBankBIN returns the six-digit Napas BIN for a bank.
// An explicitly configured BIN always wins over the short-code lookup so a bank
// missing from the table above never blocks a deployment.
func resolveVietQRBankBIN(bankBin, bankCode string) (string, error) {
	if bin := strings.TrimSpace(bankBin); bin != "" {
		if len(bin) != 6 || !isVietQRDigits(bin) {
			return "", fmt.Errorf("bankBin must be a 6-digit Napas BIN")
		}
		return bin, nil
	}
	code := strings.ToUpper(strings.TrimSpace(bankCode))
	if code == "" {
		return "", fmt.Errorf("bankCode is required when bankBin is not configured")
	}
	if len(code) == 6 && isVietQRDigits(code) {
		return code, nil
	}
	bin, ok := vietQRBankBINs[code]
	if !ok {
		return "", fmt.Errorf("unknown bankCode %q — configure bankBin with the 6-digit Napas BIN instead", bankCode)
	}
	return bin, nil
}

func isVietQRDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, ch := range s {
		if ch < '0' || ch > '9' {
			return false
		}
	}
	return true
}

// buildVietQRPayload assembles the EMVCo payload a Vietnamese banking app scans
// to prefill a transfer: fixed beneficiary, fixed amount, and the order code as
// the transfer description that SePay later echoes back over the webhook.
func buildVietQRPayload(bin, accountNumber, amount, purpose string) string {
	beneficiary := vietQRField(vietQRAcquirerBINSubID, bin) +
		vietQRField(vietQRAccountNumberSubID, accountNumber)

	merchantAccount := vietQRField(vietQRGUIDSubID, vietQRGUID) +
		vietQRField(vietQRBeneficiarySubID, beneficiary) +
		vietQRField(vietQRServiceCodeSubID, vietQRServiceTransfer)

	var b strings.Builder
	b.WriteString(vietQRField(vietQRPayloadFormatID, vietQRPayloadFormat))
	b.WriteString(vietQRField(vietQRInitiationMethodID, vietQRDynamicMethod))
	b.WriteString(vietQRField(vietQRMerchantAccountID, merchantAccount))
	b.WriteString(vietQRField(vietQRTransactionCurrencyID, vietQRCurrencyVND))
	b.WriteString(vietQRField(vietQRTransactionAmountID, amount))
	b.WriteString(vietQRField(vietQRCountryCodeID, vietQRCountryVN))
	if purpose != "" {
		b.WriteString(vietQRField(vietQRAdditionalDataID, vietQRField(vietQRPurposeSubID, purpose)))
	}

	payload := b.String() + vietQRCRCPlaceholder
	return payload + vietQRCRC16(payload)
}

func vietQRField(id, value string) string {
	return fmt.Sprintf("%s%02d%s", id, len(value), value)
}

// vietQRCRC16 computes CRC-16/CCITT-FALSE over the payload including the
// trailing "6304" tag, as required by the EMVCo QR specification.
func vietQRCRC16(payload string) string {
	crc := uint16(0xFFFF)
	for i := 0; i < len(payload); i++ {
		crc ^= uint16(payload[i]) << 8
		for bit := 0; bit < 8; bit++ {
			if crc&0x8000 != 0 {
				crc = (crc << 1) ^ 0x1021
				continue
			}
			crc <<= 1
		}
	}
	return fmt.Sprintf("%04X", crc)
}
