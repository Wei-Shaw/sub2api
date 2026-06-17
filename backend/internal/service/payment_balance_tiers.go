package service

import (
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/shopspring/decimal"
)

type BalancePricingTier struct {
	Min        float64 `json:"min"`
	Max        float64 `json:"max"`
	Multiplier float64 `json:"multiplier"`
	Label      string  `json:"label"`
	Enabled    bool    `json:"enabled"`
	SortOrder  int     `json:"sortOrder"`
}

type ResolvedBalanceTier struct {
	Tier           *BalancePricingTier
	Multiplier     float64
	CreditedAmount float64
	Label          string
}

func parseBalancePricingTiers(raw string) ([]BalancePricingTier, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	var parsed []struct {
		Min        float64 `json:"min"`
		Max        float64 `json:"max"`
		Multiplier float64 `json:"multiplier"`
		Label      string  `json:"label"`
		Enabled    *bool   `json:"enabled"`
		SortOrder  *int    `json:"sortOrder"`
	}
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		return nil, fmt.Errorf("parse balance pricing tiers: %w", err)
	}
	tiers := make([]BalancePricingTier, 0, len(parsed))
	for i, item := range parsed {
		label := strings.TrimSpace(item.Label)
		if !isFinitePositiveOrZero(item.Min) || !isFinitePositiveOrZero(item.Max) || item.Min > item.Max {
			return nil, fmt.Errorf("invalid min/max at tier %d", i)
		}
		if math.IsNaN(item.Multiplier) || math.IsInf(item.Multiplier, 0) || item.Multiplier <= 0 {
			return nil, fmt.Errorf("invalid multiplier at tier %d", i)
		}
		if label == "" {
			return nil, fmt.Errorf("invalid label at tier %d", i)
		}
		enabled := true
		if item.Enabled != nil {
			enabled = *item.Enabled
		}
		if !enabled {
			continue
		}
		sortOrder := i
		if item.SortOrder != nil {
			sortOrder = *item.SortOrder
		}
		tiers = append(tiers, BalancePricingTier{
			Min:        roundMoney(item.Min),
			Max:        roundMoney(item.Max),
			Multiplier: item.Multiplier,
			Label:      label,
			Enabled:    true,
			SortOrder:  sortOrder,
		})
	}
	sort.SliceStable(tiers, func(i, j int) bool {
		if tiers[i].SortOrder != tiers[j].SortOrder {
			return tiers[i].SortOrder < tiers[j].SortOrder
		}
		return tiers[i].Min < tiers[j].Min
	})
	for i := 1; i < len(tiers); i++ {
		if tiers[i].Min <= tiers[i-1].Max {
			return nil, fmt.Errorf("balance pricing tiers overlap at index %d", i)
		}
	}
	return tiers, nil
}

func formatBalancePricingTiers(tiers []BalancePricingTier) (string, error) {
	if len(tiers) == 0 {
		return "", nil
	}
	raw, err := json.Marshal(tiers)
	if err != nil {
		return "", fmt.Errorf("serialize balance pricing tiers: %w", err)
	}
	normalized, err := parseBalancePricingTiers(string(raw))
	if err != nil {
		return "", err
	}
	out, err := json.Marshal(normalized)
	if err != nil {
		return "", fmt.Errorf("serialize normalized balance pricing tiers: %w", err)
	}
	return string(out), nil
}

func resolveBalancePricingTier(amount float64, tiers []BalancePricingTier, fallbackMultiplier float64) ResolvedBalanceTier {
	multiplier := normalizeBalanceRechargeMultiplier(fallbackMultiplier)
	var matched *BalancePricingTier
	for i := range tiers {
		if amount >= tiers[i].Min && amount <= tiers[i].Max {
			matched = &tiers[i]
			multiplier = tiers[i].Multiplier
			break
		}
	}
	resolved := ResolvedBalanceTier{
		Tier:           matched,
		Multiplier:     multiplier,
		CreditedAmount: calculateCreditedBalanceFromPayAmount(amount, multiplier),
	}
	if matched != nil {
		resolved.Label = matched.Label
	}
	return resolved
}

func calculateCreditedBalanceFromPayAmount(paymentAmount, multiplier float64) float64 {
	if paymentAmount <= 0 || math.IsNaN(paymentAmount) || math.IsInf(paymentAmount, 0) {
		return 0
	}
	multiplier = normalizeBalanceRechargeMultiplier(multiplier)
	return decimal.NewFromFloat(paymentAmount).
		Div(decimal.NewFromFloat(multiplier)).
		Round(2).
		InexactFloat64()
}

func roundMoney(value float64) float64 {
	return decimal.NewFromFloat(value).Round(2).InexactFloat64()
}

func isFinitePositiveOrZero(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0) && value >= 0
}
