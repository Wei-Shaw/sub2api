package timezone

import (
	"testing"
	"time"
)

func TestParseRangeBoundary(t *testing.T) {
	start, err := ParseRangeBoundary("2026-09-09T15:30:00+09:00", "Asia/Tokyo", false)
	if err != nil {
		t.Fatal(err)
	}
	end, err := ParseRangeBoundary("2026-09-10T15:30:00+09:00", "Asia/Tokyo", true)
	if err != nil {
		t.Fatal(err)
	}
	if end.Sub(start) != 24*time.Hour || start.Hour() != 15 || end.Hour() != 15 {
		t.Fatalf("unexpected range %v to %v", start, end)
	}
	start, err = ParseRangeBoundary("2026-03-08", "America/New_York", false)
	if err != nil {
		t.Fatal(err)
	}
	end, err = ParseRangeBoundary("2026-03-08", "America/New_York", true)
	if err != nil {
		t.Fatal(err)
	}
	if end.Sub(start) != 23*time.Hour || end.Hour() != 0 {
		t.Fatalf("DST calendar day: %v to %v", start, end)
	}
	if _, err := ParseRangeBoundary("2026-09-10T15:30:00", "Asia/Tokyo", true); err == nil {
		t.Fatal("timezone-less timestamp accepted")
	}
}
