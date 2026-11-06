//go:build unit

package service

import (
	"encoding/json"
	"math"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/antigravity"
	"github.com/stretchr/testify/require"
)

func TestAntigravityQuota_PreservesFractionalUsageAndSharedUltraPool(t *testing.T) {
	var summary antigravity.QuotaSummary
	require.NoError(t, json.Unmarshal([]byte(`{"groups":[{"displayName":"Gemini Models","buckets":[
 {"bucketId":"gemini-weekly","window":"weekly","remainingFraction":0.24144104},
 {"bucketId":"gemini-5h","window":"5h","remainingFraction":0.99672854}
 ]}]}`), &summary))
	groups := antigravityQuotaGroups(&summary)
	require.Len(t, groups, 1)
	require.InDelta(t, 75.855896, *groups[0].Buckets[0].Utilization, 0.000001)
	require.InDelta(t, 0.327146, *groups[0].Buckets[1].Utilization, 0.000001)
	models := &antigravity.FetchAvailableModelsResponse{Models: map[string]antigravity.ModelInfo{}}
	for _, model := range []string{"gemini-2.5-pro", "gemini-3.8-flash-high", "gemini-4-argon-high"} {
		models.Models[model] = antigravity.ModelInfo{QuotaInfo: &antigravity.ModelQuotaInfo{RemainingFraction: .24144104}}
	}
	info := (&AntigravityQuotaFetcher{}).buildUsageInfo(models, "g1-ultra-tier", "ULTRA", nil)
	for _, quota := range info.AntigravityQuota {
		require.InDelta(t, *groups[0].Buckets[0].Utilization, quota.Utilization, 0.000001)
	}
	// Model availability does not add an Argon pool or turn a weekly quota into 5h.
	require.Empty(t, info.AntigravityQuotaGroups)
	require.Nil(t, info.FiveHour)
}

func TestAntigravityModelQuota_RejectsInvalidFractions(t *testing.T) {
	for _, fraction := range []float64{-.1, 1.1, math.NaN(), math.Inf(1)} {
		models := &antigravity.FetchAvailableModelsResponse{Models: map[string]antigravity.ModelInfo{
			"gemini-4-argon-high": {QuotaInfo: &antigravity.ModelQuotaInfo{RemainingFraction: fraction}},
		}}
		info := (&AntigravityQuotaFetcher{}).buildUsageInfo(models, "", "", nil)
		require.Empty(t, info.AntigravityQuota)
	}
}
