//go:build unit

package service

import "testing"

func TestParseFirstTokenThresholds(t *testing.T) {
	custom := FirstTokenThresholds{Warn: 1500, Slow: 5000, Critical: 12000}
	for _, tc := range []struct {
		name string
		raw  string
		want FirstTokenThresholds
	}{
		{"custom", `{"warn":1500,"slow":5000,"critical":12000}`, custom},
		{"missing", "", DefaultFirstTokenThresholds()},
		{"malformed", "invalid", DefaultFirstTokenThresholds()},
		{"null", "null", DefaultFirstTokenThresholds()},
		{"incomplete", `{"warn":1500}`, DefaultFirstTokenThresholds()},
		{"zero", `{"warn":0,"slow":5000,"critical":12000}`, DefaultFirstTokenThresholds()},
		{"negative", `{"warn":-1,"slow":5000,"critical":12000}`, DefaultFirstTokenThresholds()},
		{"unordered", `{"warn":5000,"slow":1500,"critical":12000}`, DefaultFirstTokenThresholds()},
		{"equal", `{"warn":1500,"slow":5000,"critical":5000}`, DefaultFirstTokenThresholds()},
		{"fractional_milliseconds", `{"warn":1.5,"slow":5000,"critical":12000}`, DefaultFirstTokenThresholds()},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := parseFirstTokenThresholds(tc.raw); got != tc.want {
				t.Fatalf("parseFirstTokenThresholds(%q) = %+v, want %+v", tc.raw, got, tc.want)
			}
		})
	}
}
