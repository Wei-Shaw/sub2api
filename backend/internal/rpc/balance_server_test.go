package rpc

import "testing"

func TestParseAmount(t *testing.T) {
	ok := []struct {
		in   string
		want float64
	}{
		{"1", 1},
		{"0.5", 0.5},
		{"12.34567890", 12.3456789},
	}
	for _, c := range ok {
		got, err := parseAmount(c.in)
		if err != nil {
			t.Fatalf("parseAmount(%q) unexpected err: %v", c.in, err)
		}
		if got != c.want {
			t.Fatalf("parseAmount(%q)=%v want %v", c.in, got, c.want)
		}
	}

	bad := []string{"", "abc", "0", "-1", "NaN", "Inf", "1e400"}
	for _, c := range bad {
		if _, err := parseAmount(c); err == nil {
			t.Fatalf("parseAmount(%q) expected error, got nil", c)
		}
	}
}

func TestFormatAmount(t *testing.T) {
	cases := map[float64]string{
		1:        "1",
		0.5:      "0.5",
		12.34:    "12.34",
		100.0001: "100.0001",
	}
	for in, want := range cases {
		if got := formatAmount(in); got != want {
			t.Fatalf("formatAmount(%v)=%q want %q", in, got, want)
		}
	}
}
