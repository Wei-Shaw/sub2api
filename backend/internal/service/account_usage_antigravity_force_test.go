//go:build unit

package service

import (
	"testing"
	"time"
)

// A manual refresh (force=true) must not be answered from the antigravity usage cache: an
// account upgraded or reset upstream otherwise keeps showing old figures for the cache TTL.
func TestCachedAntigravityUsage_ForceSkipsFreshCache(t *testing.T) {
	s := &AccountUsageService{cache: NewUsageCache()}
	resetsAt := time.Now().Add(2 * time.Hour)
	stored := &UsageInfo{FiveHour: &UsageProgress{ResetsAt: &resetsAt}}
	s.cache.antigravityCache.Store(int64(6), &antigravityUsageCache{usageInfo: stored, timestamp: time.Now()})

	usage, ok := s.cachedAntigravityUsage(6, false)
	if !ok || usage != stored {
		t.Fatalf("fresh cache should answer a normal query, got ok=%v", ok)
	}
	if usage.FiveHour.RemainingSeconds <= 0 {
		t.Fatalf("remaining seconds should be recalculated, got %d", usage.FiveHour.RemainingSeconds)
	}
	if usage, ok := s.cachedAntigravityUsage(6, true); ok || usage != nil {
		t.Fatalf("a forced query must skip the cache")
	}
}

func TestCachedAntigravityUsage_ExpiredOrMissing(t *testing.T) {
	s := &AccountUsageService{cache: NewUsageCache()}
	if _, ok := s.cachedAntigravityUsage(7, false); ok {
		t.Fatalf("missing entry must miss")
	}
	s.cache.antigravityCache.Store(int64(7), &antigravityUsageCache{usageInfo: &UsageInfo{}, timestamp: time.Now().Add(-time.Hour)})
	if _, ok := s.cachedAntigravityUsage(7, false); ok {
		t.Fatalf("expired entry must miss")
	}
}
