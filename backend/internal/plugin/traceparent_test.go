//go:build unit

package plugin

import (
	"strings"
	"testing"
)

func TestIsValidTraceparent(t *testing.T) {
	cases := []struct {
		name string
		tp   string
		want bool
	}{
		{
			name: "valid",
			tp:   "00-aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa-bbbbbbbbbbbbbbbb-01",
			want: true,
		},
		{
			name: "valid with mixed flags",
			tp:   "00-0123456789abcdef0123456789abcdef-fedcba9876543210-00",
			want: true,
		},
		{name: "empty", tp: "", want: false},
		{name: "too short", tp: "00-aaa-bbb-01", want: false},
		{
			name: "wrong version",
			tp:   "ff-aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa-bbbbbbbbbbbbbbbb-01",
			want: false,
		},
		{
			name: "uppercase hex (W3C requires lowercase)",
			tp:   "00-AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA-bbbbbbbbbbbbbbbb-01",
			want: false,
		},
		{
			name: "all-zero trace id",
			tp:   "00-00000000000000000000000000000000-bbbbbbbbbbbbbbbb-01",
			want: false,
		},
		{
			name: "all-zero span id",
			tp:   "00-aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa-0000000000000000-01",
			want: false,
		},
		{
			name: "non-hex char",
			tp:   "00-zzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzz-bbbbbbbbbbbbbbbb-01",
			want: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isValidTraceparent(tc.tp); got != tc.want {
				t.Fatalf("isValidTraceparent(%q) = %v, want %v", tc.tp, got, tc.want)
			}
		})
	}
}

func TestNewTraceparentShape(t *testing.T) {
	tp := newTraceparent()
	if !isValidTraceparent(tp) {
		t.Fatalf("newTraceparent produced invalid value %q", tp)
	}
	parts := strings.Split(tp, "-")
	if parts[0] != traceparentSupportedVersion {
		t.Fatalf("version = %q, want %q", parts[0], traceparentSupportedVersion)
	}
	if parts[3] != "01" {
		t.Fatalf("flags = %q, want sampled=01", parts[3])
	}
}

func TestNewTraceparentUniqueness(t *testing.T) {
	// 100 random traceparents should all be unique; if rand.Read is broken
	// this catches it.
	seen := make(map[string]struct{}, 100)
	for i := 0; i < 100; i++ {
		tp := newTraceparent()
		if _, dup := seen[tp]; dup {
			t.Fatalf("duplicate traceparent: %s", tp)
		}
		seen[tp] = struct{}{}
	}
}
