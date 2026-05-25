package service

import (
	"strings"
	"testing"
)

func TestEncodeEmailTextHeader_EncodesUnicodeAsRFC2047(t *testing.T) {
	const siteName = "\u6d4b\u8bd5\u7ad9"
	const codeLabel = "\u9a8c\u8bc1\u7801"
	header := encodeEmailTextHeader("[" + siteName + "] " + codeLabel)

	if strings.Contains(header, siteName) || strings.Contains(header, codeLabel) {
		t.Fatalf("expected unicode text to be RFC2047 encoded, got %q", header)
	}
	if !strings.HasPrefix(header, "=?UTF-8?") {
		t.Fatalf("expected RFC2047 UTF-8 encoded-word, got %q", header)
	}
}

func TestEncodeEmailTextHeader_RemovesCRLFBeforeEncoding(t *testing.T) {
	header := encodeEmailTextHeader("Subject\r\n injected")

	if strings.ContainsAny(header, "\r\n") {
		t.Fatalf("expected CR/LF to be removed, got %q", header)
	}
	if header != "Subject injected" {
		t.Fatalf("expected sanitized ASCII header to stay readable, got %q", header)
	}
}

func TestFormatEmailAddressHeader_EncodesUnicodeDisplayNameAsRFC2047(t *testing.T) {
	const displayName = "\u6d4b\u8bd5\u7ad9"
	header := formatEmailAddressHeader("noreply@example.com", displayName)

	if strings.Contains(header, displayName) {
		t.Fatalf("expected unicode display name to be RFC2047 encoded, got %q", header)
	}
	if !strings.Contains(header, "=?utf-8?") && !strings.Contains(header, "=?UTF-8?") {
		t.Fatalf("expected encoded display name, got %q", header)
	}
	if !strings.Contains(header, "<noreply@example.com>") {
		t.Fatalf("expected address to be preserved, got %q", header)
	}
}

func TestFormatEmailAddressHeader_RemovesCRLFFromDisplayName(t *testing.T) {
	header := formatEmailAddressHeader("noreply@example.com", "Sub2API\r\nInjected")

	if strings.ContainsAny(header, "\r\n") {
		t.Fatalf("expected CR/LF to be removed, got %q", header)
	}
	if !strings.Contains(header, "Sub2APIInjected") {
		t.Fatalf("expected sanitized display name, got %q", header)
	}
}
