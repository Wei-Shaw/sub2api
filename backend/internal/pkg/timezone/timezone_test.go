package timezone

import (
	"testing"
	"time"
)

func TestInit(t *testing.T) {
	// Test with valid timezone
	err := Init("Asia/Shanghai")
	if err != nil {
		t.Fatalf("Init failed with valid timezone: %v", err)
	}

	// Verify time.Local was set
	if time.Local.String() != "Asia/Shanghai" {
		t.Errorf("time.Local not set correctly, got %s", time.Local.String())
	}

	// Verify our location variable
	if Location().String() != "Asia/Shanghai" {
		t.Errorf("Location() not set correctly, got %s", Location().String())
	}

	// Test Name()
	if Name() != "Asia/Shanghai" {
		t.Errorf("Name() not set correctly, got %s", Name())
	}
}

func TestInitInvalidTimezone(t *testing.T) {
	err := Init("Invalid/Timezone")
	if err == nil {
		t.Error("Init should fail with invalid timezone")
	}
}

func TestTimeNowAffected(t *testing.T) {
	// Reset to UTC first
	if err := Init("UTC"); err != nil {
		t.Fatalf("Init failed with UTC: %v", err)
	}
	utcNow := time.Now()

	// Switch to Shanghai (UTC+8)
	if err := Init("Asia/Shanghai"); err != nil {
		t.Fatalf("Init failed with Asia/Shanghai: %v", err)
	}
	shanghaiNow := time.Now()

	// The times should be the same instant, but different timezone representation
	// Shanghai should be 8 hours ahead in display
	_, utcOffset := utcNow.Zone()
	_, shanghaiOffset := shanghaiNow.Zone()

	expectedDiff := 8 * 3600 // 8 hours in seconds
	actualDiff := shanghaiOffset - utcOffset

	if actualDiff != expectedDiff {
		t.Errorf("Timezone offset difference incorrect: expected %d, got %d", expectedDiff, actualDiff)
	}
}

func TestToday(t *testing.T) {
	if err := Init("Asia/Shanghai"); err != nil {
		t.Fatalf("Init failed with Asia/Shanghai: %v", err)
	}

	today := Today()
	now := Now()

	// Today should be at 00:00:00
	if today.Hour() != 0 || today.Minute() != 0 || today.Second() != 0 {
		t.Errorf("Today() not at start of day: %v", today)
	}

	// Today should be same date as now
	if today.Year() != now.Year() || today.Month() != now.Month() || today.Day() != now.Day() {
		t.Errorf("Today() date mismatch: today=%v, now=%v", today, now)
	}
}

func TestStartOfDay(t *testing.T) {
	if err := Init("Asia/Shanghai"); err != nil {
		t.Fatalf("Init failed with Asia/Shanghai: %v", err)
	}

	// Create a time at 15:30:45
	testTime := time.Date(2024, 6, 15, 15, 30, 45, 123456789, Location())
	startOfDay := StartOfDay(testTime)

	expected := time.Date(2024, 6, 15, 0, 0, 0, 0, Location())
	if !startOfDay.Equal(expected) {
		t.Errorf("StartOfDay incorrect: expected %v, got %v", expected, startOfDay)
	}
}

func TestTruncateVsStartOfDay(t *testing.T) {
	// This test demonstrates why Truncate(24*time.Hour) can be problematic
	// and why StartOfDay is more reliable for timezone-aware code

	if err := Init("Asia/Shanghai"); err != nil {
		t.Fatalf("Init failed with Asia/Shanghai: %v", err)
	}

	now := Now()

	// Truncate operates on UTC, not local time
	truncated := now.Truncate(24 * time.Hour)

	// StartOfDay operates on local time
	startOfDay := StartOfDay(now)

	// These will likely be different for non-UTC timezones
	t.Logf("Now: %v", now)
	t.Logf("Truncate(24h): %v", truncated)
	t.Logf("StartOfDay: %v", startOfDay)

	// The truncated time may not be at local midnight
	// StartOfDay is always at local midnight
	if startOfDay.Hour() != 0 {
		t.Errorf("StartOfDay should be at hour 0, got %d", startOfDay.Hour())
	}
}

func TestDSTAwareness(t *testing.T) {
	// Test with a timezone that has DST (America/New_York)
	err := Init("America/New_York")
	if err != nil {
		t.Skipf("America/New_York timezone not available: %v", err)
	}

	// Just verify it doesn't crash
	_ = Today()
	_ = Now()
	_ = StartOfDay(Now())
}

func TestStartOfWeek_Boundaries(t *testing.T) {
	if err := Init("Asia/Shanghai"); err != nil {
		t.Fatalf("Init: %v", err)
	}
	t.Cleanup(func() { _ = Init("UTC") })

	loc := Location()
	wantMon := time.Date(2026, 5, 18, 0, 0, 0, 0, loc) // 2026-05-18 是周一

	cases := []struct {
		name string
		in   time.Time
	}{
		{"friday", time.Date(2026, 5, 22, 14, 30, 0, 0, loc)},
		{"sunday", time.Date(2026, 5, 24, 10, 0, 0, 0, loc)},
		{"monday-self", time.Date(2026, 5, 18, 9, 15, 30, 0, loc)},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := StartOfWeek(c.in); !got.Equal(wantMon) {
				t.Errorf("StartOfWeek(%v) = %v, want %v", c.in, got, wantMon)
			}
		})
	}
}

func TestIsLast24HoursPeriod(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{input: "last24hours", want: true},
		{input: "LAST24H", want: true},
		{input: "24h", want: true},
		{input: "today", want: false},
		{input: "", want: false},
	}

	for _, tt := range tests {
		if got := IsLast24HoursPeriod(tt.input); got != tt.want {
			t.Fatalf("IsLast24HoursPeriod(%q)=%v want %v", tt.input, got, tt.want)
		}
	}
}

func TestLast24HoursInUserLocation(t *testing.T) {
	if err := Init("UTC"); err != nil {
		t.Fatalf("Init failed with UTC: %v", err)
	}

	before := time.Now().UTC()
	start, end := Last24HoursInUserLocation("UTC")
	after := time.Now().UTC()

	if !start.Before(end) {
		t.Fatalf("expected start before end, got start=%v end=%v", start, end)
	}
	if diff := end.Sub(start); diff != 24*time.Hour {
		t.Fatalf("expected exact 24h window, got %v", diff)
	}
	if start.Before(before.Add(-24*time.Hour-time.Second)) || start.After(after.Add(-24*time.Hour+time.Second)) {
		t.Fatalf("unexpected start time: %v", start)
	}
	if end.Before(before.Add(-time.Second)) || end.After(after.Add(time.Second)) {
		t.Fatalf("unexpected end time: %v", end)
	}
}
