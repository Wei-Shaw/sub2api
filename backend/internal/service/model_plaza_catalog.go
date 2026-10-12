package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// PlazaCatalogGroup is display-only. Prices always come from the billing engine.
// Nil Models retains the channel catalog; an explicit list replaces its model names.
type PlazaCatalogGroup struct {
	GroupID     int64    `json:"group_id"`
	Models      []string `json:"models,omitempty"`
	ImageModels []string `json:"image_models,omitempty"`
}

func parsePlazaCatalog(raw string) ([]PlazaCatalogGroup, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	var catalog []PlazaCatalogGroup
	if err := json.Unmarshal([]byte(raw), &catalog); err != nil {
		return nil, err
	}
	if catalog == nil {
		return nil, fmt.Errorf("catalog must be an array")
	}
	seen := make(map[int64]bool, len(catalog))
	for _, entry := range catalog {
		if entry.GroupID <= 0 || seen[entry.GroupID] {
			return nil, fmt.Errorf("invalid or duplicate group ID %d", entry.GroupID)
		}
		seen[entry.GroupID] = true
		models := make(map[string]bool)
		for _, names := range [][]string{entry.Models, entry.ImageModels} {
			for _, name := range names {
				if strings.TrimSpace(name) != name || name == "" || strings.Contains(name, "*") || models[name] {
					return nil, fmt.Errorf("invalid or duplicate catalog model %q", name)
				}
				models[name] = true
			}
		}
	}
	return catalog, nil
}

func plazaCatalogEntry(catalog []PlazaCatalogGroup, groupID int64) (PlazaCatalogGroup, bool) {
	for _, entry := range catalog {
		if entry.GroupID == groupID {
			return entry, true
		}
	}
	return PlazaCatalogGroup{}, false
}

func (s *ModelPlazaService) applyPlazaCatalog(ctx context.Context, pg *PlazaGroup, g *Group, entry PlazaCatalogGroup) {
	if entry.Models != nil {
		byName := make(map[string]PlazaModel, len(pg.Models))
		for _, model := range pg.Models {
			byName[model.Name] = model
		}
		pg.Models = make([]PlazaModel, 0, len(entry.Models)+len(entry.ImageModels))
		for _, name := range entry.Models {
			model, ok := byName[name]
			if !ok {
				model = PlazaModel{Name: name, Platform: g.Platform}
			}
			pg.Models = append(pg.Models, model)
		}
	}
	if !g.AllowImageGeneration || g.Platform != PlatformOpenAI || s.billingService == nil {
		return
	}
	// Use the same read-only cost calculation as OpenAI image requests, including
	// group overrides and channel resolution. No usage record or deduction is made.
	gateway := &OpenAIGatewayService{billingService: s.billingService, resolver: s.resolver}
	key := &APIKey{GroupID: &g.ID, Group: g}
	for _, name := range entry.ImageModels {
		if !g.ModelAllowlist.Allows(name) {
			continue
		}
		pricing := &ChannelModelPricing{BillingMode: BillingModeImage}
		for i, size := range []string{ImageBillingSize1K, ImageBillingSize2K, ImageBillingSize4K} {
			cost := gateway.calculateOpenAIImageCost(ctx, name, key, &OpenAIForwardResult{ImageCount: 1, ImageSize: size}, 1)
			price := cost.ActualCost
			pricing.Intervals = append(pricing.Intervals, PricingInterval{TierLabel: size, PerRequestPrice: &price, SortOrder: i})
		}
		pg.Models = append(pg.Models, PlazaModel{Name: name, Platform: g.Platform, Pricing: pricing})
	}
}
