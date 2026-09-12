package timezone

import "time"

// RangeEndDate labels the last calendar date covered by a half-open range.
// Subtracting a day would mislabel precise ends and daylight-saving days.
func RangeEndDate(end time.Time) string {
	return end.Add(-time.Nanosecond).Format("2006-01-02")
}

// ParseRangeBoundary preserves timestamp precision and treats date-only ends
// as the next local midnight for half-open database ranges.
func ParseRangeBoundary(value, userTZ string, end bool) (time.Time, error) {
	if len(value) != len("2006-01-02") {
		return time.Parse(time.RFC3339Nano, value)
	}
	t, err := ParseInUserLocation("2006-01-02", value, userTZ)
	if err == nil && end {
		t = t.AddDate(0, 0, 1)
	}
	return t, err
}
