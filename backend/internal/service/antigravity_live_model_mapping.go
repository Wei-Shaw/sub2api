package service

import (
	"maps"
	"strings"
)

// antigravityCatalogTierOrder lists the thinking tiers Antigravity encodes as an ID
// suffix, from shallowest to deepest.
var antigravityCatalogTierOrder = []string{"low", "medium", "high", "xhigh"}

// antigravityCatalogLevelFamilies groups catalog IDs of the form <base>-<tier>.
// A base is a family only when it has at least two tiers and one of them is high:
// a lone tiered ID such as gpt-oss-120b-medium is an ordinary model, and a family
// without high has no default for requests that state no effort.
// Levels are returned in antigravityCatalogTierOrder.
func antigravityCatalogLevelFamilies(catalog []string) map[string][]string {
	tiers := make(map[string]map[string]struct{})
	for _, id := range catalog {
		for _, tier := range antigravityCatalogTierOrder {
			base, ok := strings.CutSuffix(id, "-"+tier)
			if !ok || base == "" {
				continue
			}
			if tiers[base] == nil {
				tiers[base] = make(map[string]struct{}, len(antigravityCatalogTierOrder))
			}
			tiers[base][tier] = struct{}{}
			break
		}
	}
	families := make(map[string][]string)
	for base, present := range tiers {
		if _, hasHigh := present["high"]; !hasHigh || len(present) < 2 {
			continue
		}
		levels := make([]string, 0, len(present))
		for _, tier := range antigravityCatalogTierOrder {
			if _, ok := present[tier]; ok {
				levels = append(levels, tier)
			}
		}
		families[base] = levels
	}
	return families
}

// deriveAntigravityLiveModelMapping builds the model mapping of a live-mode account
// from its catalog. Family members are hidden and the bare family base maps to
// <base>-high (the effort-based tier choice happens in resolveAntigravityLevelFamily);
// every other catalog ID maps to itself. Explicit entries are applied last and win,
// so admin aliases and wildcards keep working.
func deriveAntigravityLiveModelMapping(catalog []string, explicit map[string]string) map[string]string {
	return deriveAntigravityLiveModelMappingWithFamilies(normalizeAntigravityCatalogIDs(catalog), explicit, nil)
}

// deriveAntigravityLiveModelMappingWithFamilies is deriveAntigravityLiveModelMapping
// for a normalized catalog whose families may already be computed.
func deriveAntigravityLiveModelMappingWithFamilies(catalog []string, explicit map[string]string, families map[string][]string) map[string]string {
	if families == nil {
		families = antigravityCatalogLevelFamilies(catalog)
	}
	mapping := make(map[string]string, len(catalog)+len(explicit))
	for _, id := range catalog {
		mapping[id] = id
	}
	for base, levels := range families {
		for _, level := range levels {
			delete(mapping, base+"-"+level)
		}
		mapping[base] = base + "-high"
	}
	maps.Copy(mapping, explicit)
	return mapping
}

// pickAntigravityFamilyLevel returns the tiered ID of base for level; an empty
// level or one the family lacks selects high.
func pickAntigravityFamilyLevel(base string, levels []string, level string) string {
	for _, candidate := range levels {
		if candidate == level {
			return base + "-" + level
		}
	}
	return base + "-high"
}
