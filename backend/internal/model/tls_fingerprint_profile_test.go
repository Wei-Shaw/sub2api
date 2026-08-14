package model

import (
	"errors"
	"strings"
	"testing"
)

// TestTLSFingerprintProfileValidateALPN pins the guard that keeps a profile from
// advertising a protocol the fingerprint transport cannot speak. Without it an
// operator can save alpn_protocols ["h2"] and every request on that account
// fails at the upstream with no hint that the template is at fault.
func TestTLSFingerprintProfileValidateALPN(t *testing.T) {
	tests := []struct {
		name    string
		alpn    []string
		wantErr bool
	}{
		{name: "empty is allowed and falls back to the dialer default"},
		{name: "http/1.1 is allowed", alpn: []string{"http/1.1"}},
		{name: "h2 is rejected", alpn: []string{"h2"}, wantErr: true},
		{name: "h2 alongside http/1.1 is rejected", alpn: []string{"h2", "http/1.1"}, wantErr: true},
		{name: "h3 is rejected", alpn: []string{"h3"}, wantErr: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			profile := &TLSFingerprintProfile{Name: "test", ALPNProtocols: tc.alpn}

			err := profile.Validate()

			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected validation error for %v, got nil", tc.alpn)
				}
				var validationErr *ValidationError
				if !errors.As(err, &validationErr) {
					t.Fatalf("expected *ValidationError, got %T", err)
				}
				if validationErr.Field != "alpn_protocols" {
					t.Errorf("field: got %q, want %q", validationErr.Field, "alpn_protocols")
				}
				if !strings.Contains(err.Error(), "http/1.1") {
					t.Errorf("message should name the supported protocol, got %q", err.Error())
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error for %v: %v", tc.alpn, err)
			}
		})
	}
}

// TestTLSFingerprintProfileValidateName keeps the pre-existing name rule covered
// alongside the new ALPN rule.
func TestTLSFingerprintProfileValidateName(t *testing.T) {
	profile := &TLSFingerprintProfile{ALPNProtocols: []string{"http/1.1"}}

	err := profile.Validate()

	if err == nil {
		t.Fatal("expected validation error for an empty name, got nil")
	}
}
