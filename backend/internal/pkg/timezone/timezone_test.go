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
