package service

import "testing"

func TestNormalizeProxyHost(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		raw  string
		want string
	}{
		{name: "ipv4 untouched", raw: "45.145.57.212", want: "45.145.57.212"},
		{name: "hostname untouched", raw: "proxy.example.com", want: "proxy.example.com"},
		{name: "trim whitespace", raw: "  proxy.example.com  ", want: "proxy.example.com"},
		{name: "ipv6 brackets removed", raw: "[2001:db8::1]", want: "2001:db8::1"},
		{name: "ipv6 brackets removed with spaces", raw: " [::1] ", want: "::1"},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := NormalizeProxyHost(tc.raw); got != tc.want {
				t.Fatalf("NormalizeProxyHost(%q) = %q, want %q", tc.raw, got, tc.want)
			}
		})
	}
}

func TestNormalizeProxyIPVersion(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "defaults empty to ipv4", in: "", want: ProxyIPVersionIPv4},
		{name: "keeps ipv4", in: "ipv4", want: ProxyIPVersionIPv4},
		{name: "keeps ipv6 case insensitive", in: " IPV6 ", want: ProxyIPVersionIPv6},
		{name: "unknown falls back to ipv4", in: "dual", want: ProxyIPVersionIPv4},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := NormalizeProxyIPVersion(tc.in); got != tc.want {
				t.Fatalf("NormalizeProxyIPVersion(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}
