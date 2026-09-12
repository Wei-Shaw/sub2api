package repository

import (
	"testing"
	"time"
)

func TestTrendPartialHoursBypassAggregates(t *testing.T) {
	start := time.Date(2026, 9, 9, 5, 40, 0, 0, time.UTC)
	if trendRangeHasWholeBuckets(start, start.Add(24*time.Hour), "hour") {
		t.Fatal("partial buckets accepted")
	}
	start = start.Truncate(time.Hour)
	if !trendRangeHasWholeBuckets(start, start.Add(24*time.Hour), "hour") {
		t.Fatal("whole buckets rejected")
	}
	if trendRangeHasWholeBuckets(start, start.Add(24*time.Hour+time.Nanosecond), "hour") {
		t.Fatal("partial end accepted")
	}
}
