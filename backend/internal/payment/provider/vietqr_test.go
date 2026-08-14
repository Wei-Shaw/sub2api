package provider

import (
	"fmt"
	"strings"
	"testing"
)

func TestCRC16CCITTFalse(t *testing.T) {
	// Standard check value for CRC-16/CCITT-FALSE.
	if got := crc16CCITTFalse("123456789"); got != 0x29B1 {
		t.Fatalf("crc16CCITTFalse(123456789) = %#04x, want 0x29b1", got)
	}
}

func parseTLV(t *testing.T, payload, tag string) string {
	t.Helper()
	for i := 0; i+4 <= len(payload); {
		id := payload[i : i+2]
		ln, ok := parseTwoDigitInt(payload[i+2 : i+4])
		if !ok || i+4+ln > len(payload) {
			t.Fatalf("malformed TLV at offset %d", i)
		}
		value := payload[i+4 : i+4+ln]
		if id == tag {
			return value
		}
		i += 4 + ln
	}
	return ""
}

func parseTwoDigitInt(s string) (int, bool) {
	n := 0
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return 0, false
		}
		n = n*10 + int(s[i]-'0')
	}
	return n, true
}

func TestBuildVietQRPayload(t *testing.T) {
	got := buildVietQRPayload("970422", "0123456789", 10000, "sub2_20260814aB3kX9mQ")

	if want := "000201010212"; !strings.HasPrefix(got, want) {
		t.Fatalf("prefix = %q, want %q", got[:12], want)
	}
	if v := parseTLV(t, got, "53"); v != "704" {
		t.Fatalf("currency tag 53 = %q, want 704", v)
	}
	if v := parseTLV(t, got, "54"); v != "10000" {
		t.Fatalf("amount tag 54 = %q, want 10000", v)
	}
	if v := parseTLV(t, got, "58"); v != "VN" {
		t.Fatalf("country tag 58 = %q, want VN", v)
	}
	merchant := parseTLV(t, got, "38")
	if v := parseTLV(t, merchant, "00"); v != "A000000727" {
		t.Fatalf("napas GUID = %q, want A000000727", v)
	}
	if v := parseTLV(t, merchant, "01"); v != "970422" {
		t.Fatalf("bin = %q, want 970422", v)
	}
	if v := parseTLV(t, merchant, "02"); v != "0123456789" {
		t.Fatalf("account = %q, want 0123456789", v)
	}
	if v := parseTLV(t, parseTLV(t, got, "62"), "08"); v != "sub2_20260814aB3kX9mQ" {
		t.Fatalf("content = %q", v)
	}

	// CRC tag must cover payload + "6304" and match the trailing 4 hex chars.
	idx := strings.LastIndex(got, "6304")
	if idx < 0 {
		t.Fatal("missing CRC tag")
	}
	if crc := crc16CCITTFalse(got[:idx+4]); fmt.Sprintf("%04X", crc) != got[idx+4:] {
		t.Fatalf("CRC = %s, want %04X", got[idx+4:], crc)
	}
}
