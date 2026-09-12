package timezone

import (
	"testing"
	"time"
)

func TestRangeEndDateMetadata(t *testing.T) {
	for _, tc := range []struct{ raw, tz, date string }{
		{"2026-03-08", "America/New_York", "2026-03-08"},
		{"2026-11-01", "America/New_York", "2026-11-01"},
		{"2026-09-10T06:30:00.123456789Z", "", "2026-09-10"},
		{"2026-09-10T00:00:00Z", "", "2026-09-09"},
	} {
		end, err := ParseRangeBoundary(tc.raw, tc.tz, true)
		if err != nil {
			t.Fatal(err)
		}
		if got := RangeEndDate(end); got != tc.date {
			t.Fatalf("%s: got %s, want %s", tc.raw, got, tc.date)
		}
		if len(tc.raw) > 10 && end.Format(time.RFC3339Nano) != tc.raw {
			t.Fatal("lost precision")
		}
	}
}
