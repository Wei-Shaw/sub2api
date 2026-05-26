//go:build unit

package plugin

import "testing"

func TestValidateLoopbackAddr(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name       string
		addr       string
		allowEmpty bool
		wantErr    bool
	}{
		{"ipv4_loopback", "127.0.0.1:55321", false, false},
		{"ipv4_loopback_zero_port", "127.0.0.1:0", false, false},
		{"ipv6_loopback", "[::1]:55321", false, false},
		{"localhost", "localhost:55321", false, false},
		{"empty_disallowed", "", false, true},
		{"empty_allowed", "", true, false},
		{"zero_zero_zero_zero", "0.0.0.0:55321", false, true},
		{"lan_ipv4", "192.168.1.10:55321", false, true},
		{"public_ipv4", "8.8.8.8:55321", false, true},
		{"unspec_ipv6", "[::]:55321", false, true},
		{"malformed", "not-an-addr", false, true},
		{"hostname_non_local", "example.com:55321", false, true},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := validateLoopbackAddr(tc.addr, tc.allowEmpty)
			if tc.wantErr && err == nil {
				t.Fatalf("validateLoopbackAddr(%q, %v) returned nil; want error", tc.addr, tc.allowEmpty)
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("validateLoopbackAddr(%q, %v) returned %v; want nil", tc.addr, tc.allowEmpty, err)
			}
		})
	}
}
