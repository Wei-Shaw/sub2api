package service

import (
	"encoding/json"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	managedUpstreamOwnerKey         = "managed_by"
	managedUpstreamOwner            = "upstream_station_pool"
	managedUpstreamEffectiveRateKey = "upstream_effective_rate"
	managedCostTierWaitTimeout      = 2 * time.Second
)

type managedCostTier struct {
	rate     float64
	accounts []*Account
}

// buildManagedCostTiers only activates when every candidate is owned by the
// upstream station pool. A mixed pool keeps the existing scheduler semantics.
func buildManagedCostTiers(accounts []*Account) ([]managedCostTier, bool) {
	if len(accounts) == 0 {
		return nil, false
	}
	byRate := make(map[float64][]*Account)
	rates := make([]float64, 0, len(accounts))
	for _, account := range accounts {
		if account == nil || account.Extra == nil {
			return nil, false
		}
		owner, _ := account.Extra[managedUpstreamOwnerKey].(string)
		if owner != managedUpstreamOwner {
			return nil, false
		}
		rate, ok := managedEffectiveRate(account.Extra[managedUpstreamEffectiveRateKey])
		if !ok || rate < 0 {
			return nil, false
		}
		rate = math.Round(rate*1e8) / 1e8
		if _, exists := byRate[rate]; !exists {
			rates = append(rates, rate)
		}
		byRate[rate] = append(byRate[rate], account)
	}
	sort.Float64s(rates)
	tiers := make([]managedCostTier, 0, len(rates))
	for _, rate := range rates {
		tiers = append(tiers, managedCostTier{rate: rate, accounts: byRate[rate]})
	}
	return tiers, true
}

func managedEffectiveRate(value any) (float64, bool) {
	var rate float64
	switch typed := value.(type) {
	case float64:
		rate = typed
	case float32:
		rate = float64(typed)
	case int:
		rate = float64(typed)
	case int64:
		rate = float64(typed)
	case json.Number:
		parsed, err := typed.Float64()
		if err != nil {
			return 0, false
		}
		rate = parsed
	case string:
		parsed, err := strconv.ParseFloat(strings.TrimSpace(typed), 64)
		if err != nil {
			return 0, false
		}
		rate = parsed
	default:
		return 0, false
	}
	if math.IsNaN(rate) || math.IsInf(rate, 0) {
		return 0, false
	}
	return rate, true
}
