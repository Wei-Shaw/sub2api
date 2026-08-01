package service

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
)

const LegacyPricingCatalogSourceSet = "legacy-pricing"
const CatalogNormalizerVersion = "catalog-normalizer-v1"

// BuildCatalogRevisionStage converts the legacy LiteLLM pricing map into an
// explicit, deterministic catalog candidate. It deliberately does not alter
// PricingService lookup or billing behavior.
func (s *PricingService) BuildCatalogRevisionStage(scope string, revision int64) (CatalogRevisionStage, error) {
	if s == nil {
		return CatalogRevisionStage{}, fmt.Errorf("pricing service is nil")
	}
	if strings.TrimSpace(scope) == "" {
		return CatalogRevisionStage{}, fmt.Errorf("catalog scope is required")
	}
	if revision <= 0 {
		return CatalogRevisionStage{}, fmt.Errorf("catalog revision must be positive")
	}

	entries := s.ListModelPricingCatalog()
	if len(entries) == 0 {
		return CatalogRevisionStage{}, fmt.Errorf("legacy pricing catalog is empty")
	}

	models := make([]CatalogSnapshotModelSpec, 0, len(entries))
	for index, entry := range entries {
		canonicalKey := normalizeCatalogLookupValue(entry.Model)
		if canonicalKey == "" {
			continue
		}

		pricingJSON, err := json.Marshal(entry.Pricing)
		if err != nil {
			return CatalogRevisionStage{}, fmt.Errorf("marshal pricing for %q: %w", entry.Model, err)
		}
		capabilities, err := json.Marshal(map[string]any{
			"supports_prompt_caching": entry.Pricing.SupportsPromptCaching,
			"supports_service_tier":   entry.Pricing.SupportsServiceTier,
			"mode":                    entry.Pricing.Mode,
			"max_input_tokens":        entry.Pricing.MaxInputTokens,
			"max_output_tokens":       entry.Pricing.MaxOutputTokens,
		})
		if err != nil {
			return CatalogRevisionStage{}, fmt.Errorf("marshal capabilities for %q: %w", entry.Model, err)
		}
		sourceHash := sha256.Sum256(append([]byte(canonicalKey+"\x00"), pricingJSON...))
		models = append(models, CatalogSnapshotModelSpec{
			ID:                   int64(index + 1),
			RevisionID:           int64(index + 1),
			CanonicalKey:         canonicalKey,
			OperatorState:        CatalogOperatorStateEnabled,
			SourceState:          CatalogSourceStatePresent,
			Provider:             nonEmptyCatalogValue(entry.Pricing.LiteLLMProvider, "legacy"),
			Platform:             nonEmptyCatalogValue(entry.Pricing.LiteLLMProvider, CatalogPlatformScopeAny),
			Mode:                 nonEmptyCatalogValue(entry.Pricing.Mode, "chat"),
			Capabilities:         capabilities,
			PricingSchemaVersion: 1,
			PricingValid:         !entry.Pricing.TokenPricingAbsent,
			PricingSource:        LegacyPricingCatalogSourceSet,
			Pricing:              &entry.Pricing,
			SourceMetadata:       json.RawMessage(`{"source_set":"legacy-pricing","legacy_key":"` + escapeJSONString(entry.Model) + `"}`),
			SourceHash:           hex.EncodeToString(sourceHash[:]),
			Aliases: []CatalogAliasSpec{{
				Alias:         canonicalKey,
				PlatformScope: CatalogPlatformScopeAny,
			}},
		})
	}
	if len(models) == 0 {
		return CatalogRevisionStage{}, fmt.Errorf("legacy pricing catalog contains no usable model keys")
	}

	canonicalPayload, err := json.Marshal(models)
	if err != nil {
		return CatalogRevisionStage{}, fmt.Errorf("marshal normalized catalog: %w", err)
	}
	normalizedHash := sha256.Sum256(canonicalPayload)
	return CatalogRevisionStage{
		Revision:       revision,
		NormalizedHash: hex.EncodeToString(normalizedHash[:]),
		Normalizer:     CatalogNormalizerVersion,
		SyncRun: CatalogSyncRunSpec{
			SourceSet:       LegacyPricingCatalogSourceSet,
			Trigger:         "pricing-service",
			Normalizer:      CatalogNormalizerVersion,
			SourceCount:     len(entries),
			NormalizedCount: len(models),
			AddedCount:      len(models),
		},
		Models: models,
	}, nil
}

func nonEmptyCatalogValue(value, fallback string) string {
	if value = strings.TrimSpace(value); value != "" {
		return value
	}
	return fallback
}

func escapeJSONString(value string) string {
	encoded, _ := json.Marshal(value)
	if len(encoded) >= 2 {
		return string(encoded[1 : len(encoded)-1])
	}
	return ""
}
