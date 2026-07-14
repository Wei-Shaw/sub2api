package service

import (
	"testing"
	"time"
)

func TestOpenAIAccountRuntimeStatsBoundsAndDeletesEntries(t *testing.T) {
	stats := newOpenAIAccountRuntimeStats()
	for accountID := int64(1); accountID <= openAIAccountRuntimeStatsMaxEntries+1; accountID++ {
		stats.loadOrCreate(accountID)
	}
	beforeDelete := stats.size()
	if beforeDelete > openAIAccountRuntimeStatsMaxEntries {
		t.Fatalf("expected at most %d retained stats, got %d", openAIAccountRuntimeStatsMaxEntries, beforeDelete)
	}

	var retainedID int64
	stats.accounts.Range(func(key, _ any) bool {
		retainedID, _ = key.(int64)
		return false
	})
	stats.delete(retainedID)
	if got := stats.size(); got != beforeDelete-1 {
		t.Fatalf("expected explicit deletion to reclaim one stat, got %d", got)
	}
}

func TestOpenAIAccountRuntimeStatsSweepsStaleEntries(t *testing.T) {
	stats := newOpenAIAccountRuntimeStats()
	stat := stats.loadOrCreate(42)
	now := time.Now()
	stat.lastUsedUnixNano.Store(now.Add(-openAIAccountRuntimeStatsStaleTTL - time.Second).UnixNano())

	stats.sweep(now, true)
	if got := stats.size(); got != 0 {
		t.Fatalf("expected stale stat to be reclaimed, got %d", got)
	}
}
