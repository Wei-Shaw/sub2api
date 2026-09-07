package service

import (
	"testing"
	"time"
)

func TestSelectByHealthPrefersRecentSuccessAndLowerLatency(t *testing.T) {
	previous := sharedAccountHealthStats
	defer func() { sharedAccountHealthStats = previous }()
	sharedAccountHealthStats = newOpenAIAccountRuntimeStats()
	now := time.Now()
	fast := &Account{ID: 201, Priority: 1, LastUsedAt: &now}
	slow := &Account{ID: 202, Priority: 1, LastUsedAt: &now}
	fastLatency := 80
	slowLatency := 700
	sharedAccountHealthStats.reportWithCategory(fast.ID, true, &fastLatency, "")
	sharedAccountHealthStats.reportWithCategory(slow.ID, true, &slowLatency, "")

	items := []accountWithLoad{
		{account: slow, loadInfo: &AccountLoadInfo{AccountID: slow.ID}},
		{account: fast, loadInfo: &AccountLoadInfo{AccountID: fast.ID}},
	}
	selected := selectByHealth(items, false)
	if selected == nil || selected.account.ID != fast.ID {
		t.Fatalf("selected account = %v, want fast account %d", selected, fast.ID)
	}
}

func TestSelectByHealthDoesNotLetColdAccountBeatKnownHealthyAccount(t *testing.T) {
	previous := sharedAccountHealthStats
	defer func() { sharedAccountHealthStats = previous }()
	sharedAccountHealthStats = newOpenAIAccountRuntimeStats()
	known := &Account{ID: 203, Priority: 1}
	cold := &Account{ID: 204, Priority: 1}
	sharedAccountHealthStats.reportWithCategory(known.ID, true, nil, "")
	stat := sharedAccountHealthStats.loadOrCreate(known.ID)
	if stat.samples.Load() != 1 {
		t.Fatalf("samples=%d, want 1", stat.samples.Load())
	}

	items := []accountWithLoad{
		{account: cold, loadInfo: &AccountLoadInfo{AccountID: cold.ID}},
		{account: known, loadInfo: &AccountLoadInfo{AccountID: known.ID}},
	}
	selected := selectByHealth(items, false)
	if selected == nil || selected.account.ID != known.ID {
		t.Fatalf("selected account = %v, want known account %d", selected, known.ID)
	}
}

func TestSharedAccountHealthSnapshotExpiresProbeSamples(t *testing.T) {
	previous := sharedAccountHealthStats
	defer func() { sharedAccountHealthStats = previous }()
	sharedAccountHealthStats = newOpenAIAccountRuntimeStats()
	sharedAccountHealthStats.reportProbeWithLatency(205, true, 120, time.Now().Add(-openAIScheduledProbeSampleTTL-time.Second))

	_, _, hasSample := sharedAccountHealthSnapshot(205)
	if hasSample {
		t.Fatal("expired probe sample must not affect health selection")
	}
}

func TestLegacyFallbackOrderingIgnoresSharedHealthSamples(t *testing.T) {
	previous := sharedAccountHealthStats
	defer func() { sharedAccountHealthStats = previous }()
	sharedAccountHealthStats = newOpenAIAccountRuntimeStats()
	now := time.Now()
	older := &Account{ID: 206, Priority: 1, LastUsedAt: ptrTimeForHealthTest(now.Add(-time.Minute))}
	newer := &Account{ID: 207, Priority: 1, LastUsedAt: ptrTimeForHealthTest(now)}
	sharedAccountHealthStats.reportWithCategory(newer.ID, true, nil, "")

	ordered := []*Account{newer, older}
	(&GatewayService{}).sortCandidatesForFallback(ordered, false, "last_used")
	if ordered[0].ID != older.ID {
		t.Fatalf("legacy fallback selected account %d, want %d", ordered[0].ID, older.ID)
	}
}

func ptrTimeForHealthTest(value time.Time) *time.Time { return &value }
