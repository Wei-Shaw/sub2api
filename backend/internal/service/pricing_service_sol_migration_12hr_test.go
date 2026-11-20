package service

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func obsoleteSol12hrCatalog(t *testing.T) map[string]json.RawMessage {
	t.Helper()
	body, err := os.ReadFile("testdata/obsolete_default_sol_12hr.json")
	require.NoError(t, err)
	var catalog map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(body, &catalog))
	return catalog
}

func TestSolDefaultMigration12hrRawRatesAndMetadata(t *testing.T) {
	catalog := obsoleteSol12hrCatalog(t)
	var before map[string]any
	require.NoError(t, json.Unmarshal(catalog["gpt-5.6-sol"], &before))
	before["long_context_input_token_threshold"] = float64(0)
	before["long_context_input_cost_multiplier"] = 3.0
	before["long_context_output_cost_multiplier"] = 2.0
	before["supports_service_tier"] = false
	var err error
	catalog["gpt-5.6-sol"], err = json.Marshal(before)
	require.NoError(t, err)
	catalog["openai/gpt-5.6-sol"] = append(json.RawMessage(nil), catalog["gpt-5.6-sol"]...)
	alias := string(catalog["openai/gpt-5.6-sol"])
	migrateObsoleteDefaultSolPricing(catalog)
	var after map[string]any
	require.NoError(t, json.Unmarshal(catalog["gpt-5.6-sol"], &after))
	require.Equal(t, 4e-6, after["input_cost_per_token"])
	require.Equal(t, 0.4e-6, after["cache_read_input_token_cost"])
	require.Equal(t, 20e-6, after["output_cost_per_token"])
	wantRates := map[string]float64{
		"cache_creation_input_token_cost_above_12hr":                            8e-6,
		"cache_creation_input_token_cost_above_12hr_batches":                    4e-6,
		"cache_creation_input_token_cost_above_12hr_flex":                       4e-6,
		"cache_creation_input_token_cost_above_12hr_priority":                   16e-6,
		"cache_creation_input_token_cost_above_12hr_above_272k_tokens":          16e-6,
		"cache_creation_input_token_cost_above_12hr_above_272k_tokens_batches":  8e-6,
		"cache_creation_input_token_cost_above_12hr_above_272k_tokens_flex":     8e-6,
		"cache_creation_input_token_cost_above_12hr_above_272k_tokens_priority": 32e-6,
	}
	for field, want := range wantRates {
		require.Equal(t, want, after[field], field)
		t.Run("absent/"+field, func(t *testing.T) {
			partial := obsoleteSol12hrCatalog(t)
			var entry map[string]any
			require.NoError(t, json.Unmarshal(partial["gpt-5.6-sol"], &entry))
			delete(entry, field)
			partial["gpt-5.6-sol"], err = json.Marshal(entry)
			require.NoError(t, err)
			migrateObsoleteDefaultSolPricing(partial)
			require.NoError(t, json.Unmarshal(partial["gpt-5.6-sol"], &entry))
			require.Equal(t, 4e-6, entry["input_cost_per_token"])
			require.NotContains(t, entry, field)
		})
	}
	for field, value := range before {
		if !strings.HasPrefix(field, "input_cost_per_token") && !strings.HasPrefix(field, "output_cost_per_token") &&
			!strings.HasPrefix(field, "cache_creation_input_token_cost") && !strings.HasPrefix(field, "cache_read_input_token_cost") {
			require.Equal(t, value, after[field], field)
		}
	}
	require.Len(t, after, len(before))
	require.Equal(t, alias, string(catalog["openai/gpt-5.6-sol"]))
	first := string(catalog["gpt-5.6-sol"])
	migrateObsoleteDefaultSolPricing(catalog)
	require.Equal(t, first, string(catalog["gpt-5.6-sol"]))
}

func TestSolDefaultMigration12hrRejectsChangedOrUnknownRates(t *testing.T) {
	catalog := obsoleteSol12hrCatalog(t)
	var fields map[string]any
	require.NoError(t, json.Unmarshal(catalog["gpt-5.6-sol"], &fields))
	for field, rate := range fields {
		if !strings.HasPrefix(field, "cache_creation_input_token_cost_above_12hr") {
			continue
		}
		numericRate, ok := rate.(float64)
		require.True(t, ok)
		for _, value := range []any{0, numericRate * 1.01, nil, "0.00001"} {
			t.Run(field+"/"+fmt.Sprint(value), func(t *testing.T) {
				changed := obsoleteSol12hrCatalog(t)
				var entry map[string]any
				require.NoError(t, json.Unmarshal(changed["gpt-5.6-sol"], &entry))
				entry[field] = value
				var err error
				changed["gpt-5.6-sol"], err = json.Marshal(entry)
				require.NoError(t, err)
				before := string(changed["gpt-5.6-sol"])
				migrateObsoleteDefaultSolPricing(changed)
				require.Equal(t, before, string(changed["gpt-5.6-sol"]))
			})
		}
	}
	for _, field := range []string{"cache_creation_input_token_cost_above_24hr", "cache_creation_input_token_cost_above_12hr_above_200k_tokens"} {
		t.Run(field, func(t *testing.T) {
			changed := obsoleteSol12hrCatalog(t)
			var entry map[string]any
			require.NoError(t, json.Unmarshal(changed["gpt-5.6-sol"], &entry))
			entry[field] = 10e-6
			var err error
			changed["gpt-5.6-sol"], err = json.Marshal(entry)
			require.NoError(t, err)
			before := string(changed["gpt-5.6-sol"])
			migrateObsoleteDefaultSolPricing(changed)
			require.Equal(t, before, string(changed["gpt-5.6-sol"]))
		})
	}
}

func TestSolDefaultMigration12hrSourcesAndOverrides(t *testing.T) {
	body, err := os.ReadFile("testdata/obsolete_default_sol_12hr.json")
	require.NoError(t, err)
	for _, source := range []struct {
		url  string
		want float64
	}{
		{solDefaultURL, 4e-6},
		{solDeployURL, 4e-6},
		{"https://example.com/prices.json", 5e-6},
		{"", 5e-6},
	} {
		for _, cached := range []bool{false, true} {
			if source.url == "" && !cached {
				continue
			}
			t.Run(source.url+"/cached="+strconv.FormatBool(cached), func(t *testing.T) {
				svc := initializedSolPricing(t, source.url, string(body), "", cached)
				for _, model := range []string{"gpt-5.6-sol", "openai/gpt-5.6-sol", "gpt-5.6-sol-20260901", "gpt-5.6-sol-fast", "gpt-5.6"} {
					pricing := svc.GetModelPricing(model)
					require.NotNil(t, pricing)
					require.InDelta(t, source.want, pricing.InputCostPerToken, 1e-12)
					require.Equal(t, 272000, pricing.LongContextInputTokenThreshold)
					require.Equal(t, 2.0, pricing.LongContextInputCostMultiplier)
					require.Equal(t, 1.5, pricing.LongContextOutputCostMultiplier)
				}
				if source.url != "" {
					require.NoError(t, svc.ForceUpdate())
					require.InDelta(t, source.want, svc.GetModelPricing("gpt-5.6-sol").InputCostPerToken, 1e-12)
				}
				persisted, err := os.ReadFile(svc.getPricingFilePath())
				require.NoError(t, err)
				require.JSONEq(t, string(body), string(persisted))
			})
		}
	}
	t.Run("canonical_override", func(t *testing.T) {
		svc := initializedSolPricing(t, solDefaultURL, string(body), `{"gpt-5.6-sol":{"output_cost_per_token":0,"cache_read_input_token_cost":0,"long_context_input_token_threshold":0}}`, true)
		for _, model := range []string{"gpt-5.6-sol", "openai/gpt-5.6-sol", "gpt-5.6"} {
			pricing := svc.GetModelPricing(model)
			require.InDelta(t, 4e-6, pricing.InputCostPerToken, 1e-12)
			require.Zero(t, pricing.OutputCostPerToken)
			require.Zero(t, pricing.CacheReadInputTokenCost)
			require.Zero(t, pricing.LongContextInputTokenThreshold)
		}
	})
	t.Run("override_card_is_not_migrated", func(t *testing.T) {
		svc := initializedSolPricing(t, solDefaultURL, string(body), string(body), true)
		pricing := svc.GetModelPricing("gpt-5.6-sol")
		require.InDelta(t, 5e-6, pricing.InputCostPerToken, 1e-12)
		require.InDelta(t, 30e-6, pricing.OutputCostPerToken, 1e-12)
		require.InDelta(t, 0.5e-6, pricing.CacheReadInputTokenCost, 1e-12)
	})
}
