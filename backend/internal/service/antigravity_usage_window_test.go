//go:build unit

package service

import (
	"encoding/json"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/antigravity"
	"github.com/stretchr/testify/require"
)

func TestAntigravityQuotaSummary_IndependentWindows(t *testing.T) {
	reset := time.Now().Add(2 * time.Hour).UTC().Format(time.RFC3339)
	var summary antigravity.QuotaSummary
	require.NoError(t, json.Unmarshal([]byte(fmt.Sprintf(`{"groups":[
  {"displayName":"Gemini Models","buckets":[
   {"bucketId":"gemini-5h","window":"5h","remainingFraction":0.8},
   {"bucketId":"gemini-weekly","window":"weekly","remainingFraction":0.05,"resetTime":%q}]},
  {"displayName":"Gemini 4 Argon","buckets":[
   {"bucketId":"gemini-argon-wtu-weekly","window":"weekly","remainingFraction":0.7}]},
  {"displayName":"Claude and GPT models","buckets":[
   {"bucketId":"3p-weekly","window":"weekly","remainingFraction":0}]}
 ]}`, reset)), &summary))
	groups := antigravityQuotaGroups(&summary)
	require.Len(t, groups, 3)
	require.InDelta(t, 20, *groups[0].Buckets[0].Utilization, 0.001)
	require.Equal(t, "weekly", groups[0].Buckets[1].Window)
	require.InDelta(t, 95, *groups[0].Buckets[1].Utilization, 0.001)
	require.InDelta(t, 30, *groups[1].Buckets[0].Utilization, 0.001)
	require.InDelta(t, 100, *groups[2].Buckets[0].Utilization, 0.001)
}

func TestAntigravityQuotaSummary_UnknownDisabledAndFutureBuckets(t *testing.T) {
	var summary antigravity.QuotaSummary
	require.NoError(t, json.Unmarshal([]byte(`{"groups":[{"displayName":"New pool","buckets":[
  {"bucketId":"future-monthly","window":"monthly","remainingFraction":0.4},
  {"bucketId":"gemini-5h","disabled":true,"remainingFraction":1},
  {"bucketId":"gemini-weekly","resetTime":"2026-11-07T00:00:00Z"},
  {"bucketId":"invalid","remainingFraction":1.5}
 ]}]}`), &summary))
	buckets := antigravityQuotaGroups(&summary)[0].Buckets
	require.Equal(t, "monthly", buckets[0].Window)
	require.True(t, buckets[1].Disabled)
	require.Nil(t, buckets[1].Utilization)
	require.Nil(t, buckets[2].Utilization)
	require.Nil(t, buckets[3].Utilization)
}

func TestAntigravityModelQuota_DoesNotInventWindows(t *testing.T) {
	fetcher := &AntigravityQuotaFetcher{}
	models := &antigravity.FetchAvailableModelsResponse{Models: map[string]antigravity.ModelInfo{
		"gemini-3.8-flash-high": {QuotaInfo: &antigravity.ModelQuotaInfo{RemainingFraction: .9}},
		"gemini-4-argon-low": {QuotaInfo: &antigravity.ModelQuotaInfo{
			RemainingFraction: .1, ResetTime: time.Now().Add(2 * time.Hour).Format(time.RFC3339),
		}},
	}}
	info := fetcher.buildUsageInfo(models, "g1-pro-tier", "PRO", nil)
	require.Nil(t, info.FiveHour)
	require.Nil(t, info.SevenDay)
	require.Len(t, info.AntigravityQuota, 2)
	require.Equal(t, 90.0, info.AntigravityQuota["gemini-4-argon-low"].Utilization)
}

func TestAntigravityQuotaSnapshot_TransientErrorAndRecovery(t *testing.T) {
	now := time.Now()
	previous := &UsageInfo{UpdatedAt: &now, AntigravityQuotaGroups: []AntigravityQuotaGroup{
		{Name: "Gemini 4 Argon", Buckets: []AntigravityQuotaBucket{{ID: "argon", Window: "weekly"}}},
	}}
	current := &UsageInfo{ErrorCode: errorCodeRateLimited}
	retainAntigravityQuotaSnapshot(previous, current)
	require.True(t, current.AntigravityQuotaStale)
	require.Equal(t, previous.UpdatedAt, current.UpdatedAt)
	require.Equal(t, previous.AntigravityQuotaGroups, current.AntigravityQuotaGroups)
	next := &UsageInfo{ErrorCode: errorCodeNetworkError}
	retainAntigravityQuotaSnapshot(current, next)
	require.Equal(t, &now, next.UpdatedAt)
	unauthorized := &UsageInfo{NeedsReauth: true}
	retainAntigravityQuotaSnapshot(previous, unauthorized)
	require.Empty(t, unauthorized.AntigravityQuotaGroups)
}

func TestDigestSessionStore_AtomicCapacity(t *testing.T) {
	store := NewDigestSessionStore()
	t.Cleanup(store.Close)
	store.cache.maxEntries = 2
	store.Save(1, "a", "u:1", "first", 1, "")
	store.Save(1, "b", "u:2", "second", 2, "")
	store.Save(1, "c", "u:3", "third", 3, "")
	_, _, _, found := store.Find(1, "a", "u:1")
	require.False(t, found)
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			for j := 0; j < 20; j++ {
				chain := fmt.Sprintf("u:%d:%d", n, j)
				store.Save(1, "prefix", chain, "session", 10, "")
				store.Find(1, "prefix", chain)
			}
		}(i)
	}
	wg.Wait()
	require.LessOrEqual(t, store.cache.ItemCount(), 2)
	require.LessOrEqual(t, len(store.cache.entries), 2)
	require.Equal(t, 5*time.Minute, digestSessionTTL)
}
