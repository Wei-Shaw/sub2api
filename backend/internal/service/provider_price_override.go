package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
)

type ProviderPriceOverride struct {
	ID                 string   `json:"id"`
	GroupName          string   `json:"group_name"`
	ModelName          string   `json:"model_name"`
	InputPrice         *float64 `json:"input_price,omitempty"`
	OutputPrice        *float64 `json:"output_price,omitempty"`
	CacheInputPrice    *float64 `json:"cache_input_price,omitempty"`
	CacheCreatePrice   *float64 `json:"cache_create_price,omitempty"`
	CacheCreatePrice1h *float64 `json:"cache_create_price_1h,omitempty"`
	CacheWritePrice    *float64 `json:"cache_write_price,omitempty"`   // compatibility alias
	CacheReadPrice     *float64 `json:"cache_read_price,omitempty"`    // compatibility alias
	ImageOutputPrice   *float64 `json:"image_output_price,omitempty"`  // compatibility alias
	Enabled            bool     `json:"enabled"`
	Note               string   `json:"note,omitempty"`
	SortOrder          int      `json:"sort_order"`
}

func normalizeProviderPriceOverrides(items []ProviderPriceOverride) []ProviderPriceOverride {
	if len(items) == 0 {
		return nil
	}

	normalized := make([]ProviderPriceOverride, 0, len(items))
	seen := make(map[string]int, len(items))
	for i, item := range items {
		item.ID = strings.TrimSpace(item.ID)
		if item.ID == "" {
			item.ID = fmt.Sprintf("provider-price-%d", i+1)
		}
		item.GroupName = strings.TrimSpace(item.GroupName)
		item.ModelName = strings.TrimSpace(item.ModelName)
		item.Note = strings.TrimSpace(item.Note)
		if item.CacheInputPrice == nil {
			item.CacheInputPrice = item.CacheReadPrice
		}
		if item.CacheCreatePrice == nil {
			item.CacheCreatePrice = item.CacheWritePrice
		}
		if item.GroupName == "" || item.ModelName == "" {
			continue
		}
		baseID := item.ID
		for suffix := 2; seen[strings.ToLower(item.ID)] > 0; suffix++ {
			item.ID = fmt.Sprintf("%s-%d", baseID, suffix)
		}
		seen[strings.ToLower(item.ID)]++
		normalized = append(normalized, item)
	}

	sort.SliceStable(normalized, func(i, j int) bool {
		if normalized[i].SortOrder == normalized[j].SortOrder {
			if strings.EqualFold(normalized[i].GroupName, normalized[j].GroupName) {
				return strings.ToLower(normalized[i].ModelName) < strings.ToLower(normalized[j].ModelName)
			}
			return strings.ToLower(normalized[i].GroupName) < strings.ToLower(normalized[j].GroupName)
		}
		return normalized[i].SortOrder < normalized[j].SortOrder
	})

	return normalized
}

func parseProviderPriceOverrides(raw string) []ProviderPriceOverride {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "[]" {
		return nil
	}
	var items []ProviderPriceOverride
	if err := json.Unmarshal([]byte(raw), &items); err != nil {
		return nil
	}
	return normalizeProviderPriceOverrides(items)
}

func marshalProviderPriceOverrides(items []ProviderPriceOverride) (string, error) {
	normalized := normalizeProviderPriceOverrides(items)
	if len(normalized) == 0 {
		return "[]", nil
	}
	b, err := json.Marshal(normalized)
	if err != nil {
		return "", fmt.Errorf("marshal provider price overrides: %w", err)
	}
	return string(b), nil
}

func (s *SettingService) GetProviderPriceOverrides(ctx context.Context) ([]ProviderPriceOverride, error) {
	if s == nil || s.settingRepo == nil {
		return nil, nil
	}
	raw, err := s.settingRepo.GetValue(ctx, SettingKeyProviderPriceOverrides)
	if err != nil {
		if errors.Is(err, ErrSettingNotFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("get provider price overrides: %w", err)
	}
	return parseProviderPriceOverrides(raw), nil
}

func (s *SettingService) UpdateProviderPriceOverrides(ctx context.Context, items []ProviderPriceOverride) ([]ProviderPriceOverride, error) {
	if s == nil || s.settingRepo == nil {
		return nil, fmt.Errorf("setting repo is nil")
	}
	payload, err := marshalProviderPriceOverrides(items)
	if err != nil {
		return nil, err
	}
	if err := s.settingRepo.Set(ctx, SettingKeyProviderPriceOverrides, payload); err != nil {
		return nil, fmt.Errorf("set provider price overrides: %w", err)
	}
	return parseProviderPriceOverrides(payload), nil
}
