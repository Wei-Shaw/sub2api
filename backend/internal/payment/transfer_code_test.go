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
