package payment

import "testing"

func TestNormalizeTransferCode(t *testing.T) {
	cases := []struct{ in, want string }{
		{"sub2_20260815YujbZRZd", "SUB220260815YUJBZRZD"},
		{"sub220260815YujbZRZd", "SUB220260815YUJBZRZD"},
		{"SUB2_20260815YUJBZRZD", "SUB220260815YUJBZRZD"},
		{"  sub2-2026.x y  ", "SUB22026XY"},
		{"", ""},
		{"___", ""},
	}
	for _, tc := range cases {
		if got := NormalizeTransferCode(tc.in); got != tc.want {
			t.Errorf("NormalizeTransferCode(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestStripTransferSeparators(t *testing.T) {
	cases := []struct{ in, want string }{
		{"sub2_20260815ZPbOX0Kl", "sub220260815ZPbOX0Kl"},
		{"sub2_20260815YujbZRZd", "sub220260815YujbZRZd"},
		{"SUB2_20260815", "SUB220260815"},
		{"", ""},
		{"___", ""},
	}
	for _, tc := range cases {
		if got := StripTransferSeparators(tc.in); got != tc.want {
			t.Errorf("StripTransferSeparators(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
