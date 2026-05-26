// Package timezone provides global timezone management for the application.
// Similar to PHP's date_default_timezone_set, this package allows setting
// a global timezone that affects all time.Now() calls.
package timezone

import (
	"fmt"
	"log"
	"time"
)

var (
	// location is the global timezone location
	location *time.Location
	// tzName stores the timezone name for logging/debugging
	tzName string
)

// Init initializes the global timezone setting.
// This should be called once at application startup.
// Example timezone values: "Asia/Shanghai", "America/New_York", "UTC"
func Init(tz string) error {
	if tz == "" {
		tz = "Asia/Shanghai" // Default timezone
	}

	loc, err := time.LoadLocation(tz)
	if err != nil {
		return fmt.Errorf("invalid timezone %q: %w", tz, err)
	}

	// Set the global Go time.Local to our timezone
	// This affects time.Now() throughout the application
	time.Local = loc
	location = loc
	tzName = tz

	log.Printf("Timezone initialized: %s (UTC offset: %s)", tz, getUTCOffset(loc))
	return nil
}

// getUTCOffset returns the current UTC offset for a location
func getUTCOffset(loc *time.Location) string {
	_, offset := time.Now().In(loc).Zone()
	hours := offset / 3600
	minutes := (offset % 3600) / 60
	if minutes < 0 {
		minutes = -minutes
	}
	sign := "+"
	if hours < 0 {
		sign = "-"
		hours = -hours
	}
	return fmt.Sprintf("%s%02d:%02d", sign, hours, minutes)
}

// Now returns the current time in the configured timezone.
// This is equivalent to time.Now() after Init() is called,
// but provided for explicit timezone-aware code.
func Now() time.Time {
	if location == nil {
		return time.Now()
	}
	return time.Now().In(location)
}

// Location returns the configured timezone location.
func Location() *time.Location {
	if location == nil {
		return time.Local
	}
	return location
}

// Name returns the configured timezone name.
func Name() string {
	if tzName == "" {
		return "Local"
	}
	return tzName
}

// StartOfDay returns the start of the given day (00:00:00) in the configured timezone.
func StartOfDay(t time.Time) time.Time {
	loc := Location()
	t = t.In(loc)
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, loc)
}

// Today returns the start of today (00:00:00) in the configured timezone.
func Today() time.Time {
	return StartOfDay(Now())
}

// EndOfDay returns the end of the given day (23:59:59.999999999) in the configured timezone.
func EndOfDay(t time.Time) time.Time {
	loc := Location()
	t = t.In(loc)
	return time.Date(t.Year(), t.Month(), t.Day(), 23, 59, 59, 999999999, loc)
}

// StartOfWeek returns the start of the week (Monday 00:00:00) for the given time.
func StartOfWeek(t time.Time) time.Time {
	loc := Location()
	t = t.In(loc)
	weekday := int(t.Weekday())
	if weekday == 0 {
		weekday = 7 // Sunday is day 7
	}
	return time.Date(t.Year(), t.Month(), t.Day()-weekday+1, 0, 0, 0, 0, loc)
}

// StartOfMonth returns the start of the month (1st day 00:00:00) for the given time.
func StartOfMonth(t time.Time) time.Time {
	loc := Location()
	t = t.In(loc)
	return time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, loc)
}

// ParseInLocation parses a time string in the configured timezone.
func ParseInLocation(layout, value string) (time.Time, error) {
	return time.ParseInLocation(layout, value, Location())
}

// ParseInUserLocation parses a time string in the user's timezone.
// If userTZ is empty or invalid, falls back to the configured server timezone.
func ParseInUserLocation(layout, value, userTZ string) (time.Time, error) {
	loc := Location() // default to server timezone
	if userTZ != "" {
		if userLoc, err := time.LoadLocation(userTZ); err == nil {
			loc = userLoc
		}
	}
	return time.ParseInLocation(layout, value, loc)
}

// NowInUserLocation returns the current time in the user's timezone.
// If userTZ is empty or invalid, falls back to the configured server timezone.
func NowInUserLocation(userTZ string) time.Time {
	if userTZ == "" {
		return Now()
	}
	if userLoc, err := time.LoadLocation(userTZ); err == nil {
		return time.Now().In(userLoc)
	}
	return Now()
}

// StartOfDayInUserLocation returns the start of the given day in the user's timezone.
// If userTZ is empty or invalid, falls back to the configured server timezone.
func StartOfDayInUserLocation(t time.Time, userTZ string) time.Time {
	loc := Location()
	if userTZ != "" {
		if userLoc, err := time.LoadLocation(userTZ); err == nil {
			loc = userLoc
		}
	}
	t = t.In(loc)
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, loc)
}
