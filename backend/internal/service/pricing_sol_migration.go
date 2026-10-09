package service

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"strings"
)

func (s *PricingService) isFallbackPricingBody(body []byte) bool {
	marker, err := os.ReadFile(s.fallbackPricingMarkerPath())
	hash := sha256.Sum256(body)
	return err == nil && strings.TrimSpace(string(marker)) == hex.EncodeToString(hash[:])
}

func (s *PricingService) fallbackPricingMarkerPath() string {
	return s.getPricingFilePath() + ".fallback.sha256"
}

// migrateObsoleteDefaultSolPricing repairs issue #7465's stale default-source Sol
// cards, including offline caches from installations predating the pricing fix.
// Corrected rates follow the official source: https://developers.openai.com/api/docs/pricing.
// The prior bundled card used a 3.125e-6 batch cache-creation rate, whereas the
// obsolete remote card used 2.5e-6; both normalize to 2.5e-6. Matching is fail-safe:
// the required base tuple and every present optional rate must match known stale
// values, with only that batch field accepting the second historical value.
// Unknown or changed rates leave the entire card untouched so future source prices
// are not pinned. Absent optional rates and explicit long-context metadata stay intact.
func migrateObsoleteDefaultSolPricing(rawData map[string]json.RawMessage) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(rawData["gpt-5.6-sol"], &fields); err != nil || fields == nil {
		return
	}
	rates := map[string][2]float64{
		"input_cost_per_token":                                       {5e-6, 4e-6},
		"input_cost_per_token_batches":                               {2.5e-6, 2e-6},
		"input_cost_per_token_flex":                                  {2.5e-6, 2e-6},
		"input_cost_per_token_priority":                              {10e-6, 8e-6},
		"input_cost_per_token_above_272k_tokens":                     {10e-6, 8e-6},
		"input_cost_per_token_above_272k_tokens_batches":             {4e-6, 4e-6},
		"input_cost_per_token_above_272k_tokens_flex":                {5e-6, 4e-6},
		"input_cost_per_token_above_272k_tokens_priority":            {20e-6, 16e-6},
		"output_cost_per_token":                                      {30e-6, 20e-6},
		"output_cost_per_token_batches":                              {15e-6, 10e-6},
		"output_cost_per_token_flex":                                 {15e-6, 10e-6},
		"output_cost_per_token_priority":                             {60e-6, 40e-6},
		"output_cost_per_token_above_272k_tokens":                    {45e-6, 30e-6},
		"output_cost_per_token_above_272k_tokens_batches":            {15e-6, 15e-6},
		"output_cost_per_token_above_272k_tokens_flex":               {22.5e-6, 15e-6},
		"output_cost_per_token_above_272k_tokens_priority":           {90e-6, 60e-6},
		"cache_creation_input_token_cost":                            {6.25e-6, 5e-6},
		"cache_creation_input_token_cost_batches":                    {2.5e-6, 2.5e-6},
		"cache_creation_input_token_cost_flex":                       {3.125e-6, 2.5e-6},
		"cache_creation_input_token_cost_priority":                   {12.5e-6, 10e-6},
		"cache_creation_input_token_cost_above_272k_tokens":          {12.5e-6, 10e-6},
		"cache_creation_input_token_cost_above_272k_tokens_batches":  {5e-6, 5e-6},
		"cache_creation_input_token_cost_above_272k_tokens_flex":     {6.25e-6, 5e-6},
		"cache_creation_input_token_cost_above_272k_tokens_priority": {25e-6, 20e-6},
		"cache_read_input_token_cost":                                {0.5e-6, 0.4e-6},
		"cache_read_input_token_cost_batches":                        {0.2e-6, 0.2e-6},
		"cache_read_input_token_cost_flex":                           {0.25e-6, 0.2e-6},
		"cache_read_input_token_cost_priority":                       {1e-6, 0.8e-6},
		"cache_read_input_token_cost_above_272k_tokens":              {1e-6, 0.8e-6},
		"cache_read_input_token_cost_above_272k_tokens_batches":      {0.4e-6, 0.4e-6},
		"cache_read_input_token_cost_above_272k_tokens_flex":         {0.5e-6, 0.4e-6},
		"cache_read_input_token_cost_above_272k_tokens_priority":     {2e-6, 1.6e-6},
	}
	for _, field := range []string{"input_cost_per_token", "output_cost_per_token", "cache_read_input_token_cost"} {
		if _, exists := fields[field]; !exists {
			return
		}
	}
	for field := range fields {
		if strings.HasPrefix(field, "input_cost_per_token") || strings.HasPrefix(field, "output_cost_per_token") ||
			strings.HasPrefix(field, "cache_creation_input_token_cost") || strings.HasPrefix(field, "cache_read_input_token_cost") {
			if _, known := rates[field]; !known {
				return
			}
		}
	}
	for field, pair := range rates {
		if raw, exists := fields[field]; exists {
			var value float64
			if string(raw) == "null" || json.Unmarshal(raw, &value) != nil {
				return
			}
			if value != pair[0] && !(field == "cache_creation_input_token_cost_batches" && value == 3.125e-6) {
				return
			}
		}
	}
	for field, pair := range rates {
		if _, exists := fields[field]; exists {
			fields[field], _ = json.Marshal(pair[1])
		}
	}
	rawData["gpt-5.6-sol"], _ = json.Marshal(fields)
}
