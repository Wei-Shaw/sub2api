package provider

import (
	"fmt"
	"strconv"
	"strings"
)

// buildVietQRPayload builds an EMVCo merchant-presented QR string following
// the VietQR/NAPAS standard: banking apps scan it and prefill the beneficiary
// account, amount and transfer content.
func buildVietQRPayload(bin, accountNumber string, amountVND int64, content string) string {
	merchantAccount := tlv("00", "A000000727") + tlv("01", bin) + tlv("02", accountNumber)
	payload := tlv("00", "01") + // Payload Format Indicator
		tlv("01", "12") + // Point of Initiation: dynamic (amount included)
		tlv("38", merchantAccount) + // Merchant Account Information (NAPAS)
		tlv("53", "704") + // Transaction Currency: VND
		tlv("54", strconv.FormatInt(amountVND, 10)) + // Transaction Amount
		tlv("58", "VN") + // Country Code
		tlv("62", tlv("08", content)) // Additional Data: purpose (transfer content)
	return payload + "6304" + strings.ToUpper(fmt.Sprintf("%04X", crc16CCITTFalse(payload+"6304")))
}

// tlv encodes one EMVCo TLV field with a two-digit length prefix.
func tlv(tag, value string) string {
	return tag + fmt.Sprintf("%02d", len(value)) + value
}

// crc16CCITTFalse computes CRC-16/CCITT-FALSE (poly 0x1021, init 0xFFFF, no
// reflection, no final XOR) — the checksum mandated by EMVCo QR (tag 63).
func crc16CCITTFalse(data string) uint16 {
	crc := uint16(0xFFFF)
	for i := 0; i < len(data); i++ {
		crc ^= uint16(data[i]) << 8
		for bit := 0; bit < 8; bit++ {
			if crc&0x8000 != 0 {
				crc = (crc << 1) ^ 0x1021
			} else {
				crc <<= 1
			}
		}
	}
	return crc
}
