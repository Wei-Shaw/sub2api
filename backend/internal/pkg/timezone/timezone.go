// Package timezone provides global timezone management for the application.
// Similar to PHP's date_default_timezone_set, this package allows setting
// a global timezone that affects all time.Now() calls.
package timezone

import (
	"fmt"
	"log"
	"time"
)

const (
	civilDateLayout              = "2006-01-02"
	maxSkippedCivilDateLookahead = 7
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

// UTCOffset returns the current UTC offset of the configured timezone, e.g. "+08:00".
func UTCOffset() string {
	return getUTCOffset(Location())
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

// ParseCivilDateStart returns the first real instant belonging to the requested
// civil date. It rejects dates that do not exist in the selected timezone.
func ParseCivilDateStart(value, userTZ string) (time.Time, error) {
	civilDate, err := time.Parse(civilDateLayout, value)
	if err != nil {
		return time.Time{}, err
	}
	loc := userLocation(userTZ)
	if start, ok := firstValidCivilDateInstant(civilDate.Year(), civilDate.Month(), civilDate.Day(), loc); ok {
		return start, nil
	}
	return time.Time{}, fmt.Errorf("civil date %s does not exist in timezone %s", value, loc)
}

// NextCivilDateStart returns the first real instant after the requested civil
// date. Entirely skipped dates are crossed without normalizing into a prior day.
func NextCivilDateStart(value, userTZ string) (time.Time, error) {
	civilDate, err := time.Parse(civilDateLayout, value)
	if err != nil {
		return time.Time{}, err
	}
	loc := userLocation(userTZ)
	for offset := 1; offset <= maxSkippedCivilDateLookahead; offset++ {
		next := civilDate.AddDate(0, 0, offset)
		if start, ok := firstValidCivilDateInstant(next.Year(), next.Month(), next.Day(), loc); ok {
			return start, nil
		}
	}
	return time.Time{}, fmt.Errorf("no valid civil date found after %s in timezone %s", value, loc)
}

// StartOfCivilDate returns the first real instant of t's civil date in userTZ.
func StartOfCivilDate(t time.Time, userTZ string) (time.Time, error) {
	loc := userLocation(userTZ)
	local := t.In(loc)
	if start, ok := firstValidCivilDateInstant(local.Year(), local.Month(), local.Day(), loc); ok {
		return start, nil
	}
	return time.Time{}, fmt.Errorf("civil date %s does not exist in timezone %s", local.Format(civilDateLayout), loc)
}

func userLocation(userTZ string) *time.Location {
	if userTZ != "" {
		if loc, err := time.LoadLocation(userTZ); err == nil {
			return loc
		}
	}
	return Location()
}

func firstValidCivilDateInstant(year int, month time.Month, day int, loc *time.Location) (time.Time, bool) {
	target := time.Date(year, month, day, 0, 0, 0, 0, time.UTC)
	searchStart := target.Add(-36 * time.Hour)
	searchEnd := target.Add(60 * time.Hour)
	previous := searchStart

	for probe := searchStart; !probe.After(searchEnd); probe = probe.Add(time.Minute) {
		local := probe.In(loc)
		if local.Year() != year || local.Month() != month || local.Day() != day {
			previous = probe
			continue
		}

		low, high := previous.Unix(), probe.Unix()
		for low+1 < high {
			mid := low + (high-low)/2
			candidate := time.Unix(mid, 0).In(loc)
			if civilDateBefore(candidate, year, month, day) {
				low = mid
			} else {
				high = mid
			}
		}
		start := time.Unix(high, 0).In(loc)
		if start.Year() == year && start.Month() == month && start.Day() == day {
			return start, true
		}
		return time.Time{}, false
	}
	return time.Time{}, false
}

func civilDateBefore(t time.Time, year int, month time.Month, day int) bool {
	if t.Year() != year {
		return t.Year() < year
	}
	if t.Month() != month {
		return t.Month() < month
	}
	return t.Day() < day
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
	if start, err := StartOfCivilDate(t, userTZ); err == nil {
		return start
	}
	loc := userLocation(userTZ)
	t = t.In(loc)
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, loc)
}
