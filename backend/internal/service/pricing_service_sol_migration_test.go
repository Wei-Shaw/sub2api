package service

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

const solDefaultURL = "https://raw.githubusercontent.com/Wei-Shaw/model-price-repo/main/model_prices_and_context_window.json"
const solDeployURL = "https://raw.githubusercontent.com/Wei-Shaw/model-price-repo/refs/heads/main//model_prices_and_context_window.json"

type solMigrationRemoteClient struct {
	body      string
	err       error
	downloads int
	hashes    int
}

func (client *solMigrationRemoteClient) FetchPricingJSON(context.Context, string) ([]byte, error) {
	client.downloads++
	return []byte(client.body), client.err
}

func (client *solMigrationRemoteClient) FetchHashText(context.Context, string) (string, error) {
	client.hashes++
	return fmt.Sprintf("%x", sha256.Sum256([]byte(client.body))), client.err
}

func TestSolDefaultMigrationPriorBundledOfflineCache(t *testing.T) {
	body, err := os.ReadFile("testdata/obsolete_bundled_sol.json")
	require.NoError(t, err)
	for _, url := range []string{solDefaultURL, solDeployURL} {
		t.Run(url, func(t *testing.T) {
			cfg := &config.Config{Pricing: config.PricingConfig{
				RemoteURL: url, HashURL: "https://example.com/hash", DataDir: t.TempDir(), UpdateIntervalHours: 24,
			}}
			client := &solMigrationRemoteClient{err: errors.New("offline")}
			svc := NewPricingService(cfg, client)
			require.NoError(t, os.WriteFile(svc.getPricingFilePath(), body, 0644))
			require.NoError(t, svc.Initialize())
			t.Cleanup(svc.Stop)
			require.Equal(t, 1, client.hashes)
			require.Zero(t, client.downloads)
			pricing := svc.GetModelPricing("gpt-5.6-sol")
			require.InDelta(t, 4e-6, pricing.InputCostPerToken, 1e-12)
			require.InDelta(t, 20e-6, pricing.OutputCostPerToken, 1e-12)
			require.InDelta(t, 5e-6, pricing.CacheCreationInputTokenCost, 1e-12)
			require.InDelta(t, 0.4e-6, pricing.CacheReadInputTokenCost, 1e-12)
			require.Equal(t, 272000, pricing.LongContextInputTokenThreshold)
			require.Equal(t, 2.0, pricing.LongContextInputCostMultiplier)
			require.Equal(t, 1.5, pricing.LongContextOutputCostMultiplier)
			billing := NewBillingService(&config.Config{}, svc)
			for _, input := range []int{272000, 272001} {
				cost, err := billing.CalculateCost("gpt-5.6-sol", UsageTokens{InputTokens: input, OutputTokens: 100}, 1)
				require.NoError(t, err)
				want := float64(input)*4e-6 + 100*20e-6
				if input > 272000 {
					want = float64(input)*8e-6 + 100*30e-6
				}
				require.InDelta(t, want, cost.ActualCost, 1e-12)
				require.Equal(t, input > 272000, cost.LongContextBillingApplied)
			}
			var catalog map[string]json.RawMessage
			require.NoError(t, json.Unmarshal(body, &catalog))
			migrateObsoleteDefaultSolPricing(catalog)
			var fields map[string]any
			require.NoError(t, json.Unmarshal(catalog["gpt-5.6-sol"], &fields))
			require.Equal(t, 2.5e-6, fields["cache_creation_input_token_cost_batches"])
			require.NotContains(t, fields, "cache_read_input_token_cost_batches")
			persisted, err := os.ReadFile(svc.getPricingFilePath())
			require.NoError(t, err)
			require.JSONEq(t, string(body), string(persisted))
		})
	}
}

func TestSolDefaultMigrationRefreshPreservesFuturePrices(t *testing.T) {
	for _, refresh := range []string{"force", "hash_change"} {
		t.Run(refresh, func(t *testing.T) {
			svc := initializedSolPricing(t, solDefaultURL, obsoleteSolCatalog(t, nil), "", true)
			client := &solMigrationRemoteClient{}
			svc.remoteClient = client
			if refresh == "hash_change" {
				svc.cfg.Pricing.HashURL = "https://example.com/hash"
			}
			prior, err := os.ReadFile("testdata/obsolete_bundled_sol.json")
			require.NoError(t, err)
			for _, body := range []string{string(prior), obsoleteSolCatalog(t, map[string]any{
				"input_cost_per_token": 7e-6, "output_cost_per_token": 23e-6,
				"cache_creation_input_token_cost_batches": 3e-6,
			})} {
				client.body = body
				if refresh == "force" {
					require.NoError(t, svc.ForceUpdate())
				} else {
					require.NoError(t, svc.syncWithRemote())
				}
				pricing := svc.GetModelPricing("gpt-5.6-sol")
				if body == string(prior) {
					require.InDelta(t, 4e-6, pricing.InputCostPerToken, 1e-12)
					require.InDelta(t, 20e-6, pricing.OutputCostPerToken, 1e-12)
				} else {
					require.InDelta(t, 7e-6, pricing.InputCostPerToken, 1e-12)
					require.InDelta(t, 23e-6, pricing.OutputCostPerToken, 1e-12)
					require.InDelta(t, 6.25e-6, pricing.CacheCreationInputTokenCost, 1e-12)
				}
				require.Equal(t, fmt.Sprintf("%x", sha256.Sum256([]byte(body))), svc.localHash)
				persisted, err := os.ReadFile(svc.getPricingFilePath())
				require.NoError(t, err)
				require.JSONEq(t, body, string(persisted))
			}
			require.Equal(t, 2, client.downloads)
			if refresh == "hash_change" {
				require.NoError(t, svc.syncWithRemote())
				require.Equal(t, 2, client.downloads)
			}
		})
	}
}

func TestSolDefaultMigrationRejectsUnknownBatchCacheCreation(t *testing.T) {
	for _, fixture := range []string{"obsolete_default_sol.json", "obsolete_bundled_sol.json"} {
		for _, value := range []any{0, 3e-6, 3.1251e-6, nil, "3.125e-6"} {
			t.Run(fixture+"/"+fmt.Sprint(value), func(t *testing.T) {
				body, err := os.ReadFile(filepath.Join("testdata", fixture))
				require.NoError(t, err)
				var catalog map[string]map[string]any
				require.NoError(t, json.Unmarshal(body, &catalog))
				catalog["gpt-5.6-sol"]["cache_creation_input_token_cost_batches"] = value
				body, err = json.Marshal(catalog)
				require.NoError(t, err)
				var raw map[string]json.RawMessage
				require.NoError(t, json.Unmarshal(body, &raw))
				before := string(raw["gpt-5.6-sol"])
				migrateObsoleteDefaultSolPricing(raw)
				require.JSONEq(t, before, string(raw["gpt-5.6-sol"]))
			})
		}
	}
}

func obsoleteSolCatalog(t *testing.T, patch map[string]any) string {
	t.Helper()
	body, err := os.ReadFile("testdata/obsolete_default_sol.json")
	require.NoError(t, err)
	var catalog map[string]map[string]any
	require.NoError(t, json.Unmarshal(body, &catalog))
	for field, value := range patch {
		if value == nil {
			delete(catalog["gpt-5.6-sol"], field)
		} else {
			catalog["gpt-5.6-sol"][field] = value
		}
	}
	body, err = json.Marshal(catalog)
	require.NoError(t, err)
	return string(body)
}

func initializedSolPricing(t *testing.T, url, body, override string, cached bool) *PricingService {
	t.Helper()
	cfg := &config.Config{Pricing: config.PricingConfig{
		RemoteURL: url, DataDir: t.TempDir(), UpdateIntervalHours: 24,
	}}
	if override != "" {
		cfg.Pricing.OverrideFile = filepath.Join(cfg.Pricing.DataDir, "override.json")
		require.NoError(t, os.WriteFile(cfg.Pricing.OverrideFile, []byte(override), 0644))
	}
	svc := NewPricingService(cfg, stubPricingRemoteClient{body: body})
	if cached {
		require.NoError(t, os.WriteFile(svc.getPricingFilePath(), []byte(body), 0644))
	}
	require.NoError(t, svc.Initialize())
	t.Cleanup(svc.Stop)
	return svc
}

func TestSolDefaultMigrationBillingBoundaries(t *testing.T) {
	for _, url := range []string{solDefaultURL, solDeployURL} {
		for _, cached := range []bool{false, true} {
			t.Run(url+"/cached="+strconv.FormatBool(cached), func(t *testing.T) {
				svc := initializedSolPricing(t, url, obsoleteSolCatalog(t, nil), "", cached)
				billing := NewBillingService(&config.Config{}, svc)
				for _, model := range []string{"gpt-5.6-sol", "gpt-5.6-sol-20260901", "openai/gpt-5.6-sol", "gpt-5.6-sol-fast", "openai/gpt-5.6-sol-20260901", "gpt-5.6"} {
					for _, input := range []int{272000, 272001} {
						for _, tier := range []string{"", "priority", "fast", "flex"} {
							tokens := UsageTokens{InputTokens: input - 2000, CacheCreationTokens: 1000, CacheReadTokens: 1000, OutputTokens: 100}
							cost, err := billing.CalculateCostWithServiceTier(model, tokens, 1.25, tier)
							require.NoError(t, err)
							inputScale, outputScale, tierScale := 1.0, 1.0, 1.0
							if input > 272000 {
								inputScale, outputScale = 2, 1.5
							}
							if tier == "priority" || tier == "fast" {
								tierScale = 2
							}
							if tier == "flex" {
								tierScale = 0.5
							}
							want := (float64(tokens.InputTokens)*4e-6+1000*5e-6+1000*0.4e-6)*inputScale*tierScale + 100*20e-6*outputScale*tierScale
							require.InDelta(t, want, cost.TotalCost, 1e-12, "%s input=%d tier=%s", model, input, tier)
							require.InDelta(t, want*1.25, cost.ActualCost, 1e-12)
							require.Equal(t, input > 272000, cost.LongContextBillingApplied)
						}
					}
				}
				require.NoError(t, svc.ForceUpdate())
				require.InDelta(t, 4e-6, svc.GetModelPricing("gpt-5.6-sol").InputCostPerToken, 1e-12)
				persisted, err := os.ReadFile(svc.getPricingFilePath())
				require.NoError(t, err)
				require.JSONEq(t, obsoleteSolCatalog(t, nil), string(persisted))
			})
		}
	}
}

func TestSolDefaultMigrationSourceAndFutureControls(t *testing.T) {
	for _, test := range []struct {
		name, url string
		patch     map[string]any
		want      float64
	}{
		{"custom", "https://example.com/prices.json", nil, 5e-6},
		{"local_only", "", nil, 5e-6},
		{"similar_url", solDefaultURL + "?custom=1", nil, 5e-6},
		{"future_base", solDefaultURL, map[string]any{"input_cost_per_token": 6e-6}, 6e-6},
		{"future_output", solDefaultURL, map[string]any{"output_cost_per_token": 21e-6}, 5e-6},
		{"future_cache", solDefaultURL, map[string]any{"cache_read_input_token_cost": 0.6e-6}, 5e-6},
		{"future_priority", solDefaultURL, map[string]any{"output_cost_per_token_priority": 42e-6}, 5e-6},
		{"future_above", solDefaultURL, map[string]any{"input_cost_per_token_above_272k_tokens": 11e-6}, 5e-6},
		{"new_ladder", solDefaultURL, map[string]any{"input_cost_per_token_above_200k_tokens": 12e-6}, 5e-6},
		{"explicit_zero_rate", solDefaultURL, map[string]any{"cache_creation_input_token_cost": 0}, 5e-6},
	} {
		t.Run(test.name, func(t *testing.T) {
			svc := initializedSolPricing(t, test.url, obsoleteSolCatalog(t, test.patch), "", true)
			require.InDelta(t, test.want, svc.GetModelPricing("gpt-5.6-sol").InputCostPerToken, 1e-12)
		})
	}
}

func TestSolDefaultMigrationOverrideReloadAndDisable(t *testing.T) {
	svc := initializedSolPricing(t, solDefaultURL, obsoleteSolCatalog(t, nil), `{"gpt-5.6-sol":{"output_cost_per_token":0,"long_context_input_token_threshold":0}}`, true)
	pricing := svc.GetModelPricing("gpt-5.6-sol")
	require.InDelta(t, 4e-6, pricing.InputCostPerToken, 1e-12)
	require.Zero(t, pricing.OutputCostPerToken)
	require.Zero(t, pricing.LongContextInputTokenThreshold)
	require.NoError(t, os.WriteFile(svc.cfg.Pricing.OverrideFile, []byte(`{"gpt-5.6-sol":{"input_cost_per_token":9e-6}}`), 0644))
	svc.reloadIfCustomFilesChanged()
	pricing = svc.GetModelPricing("gpt-5.6-sol")
	require.InDelta(t, 9e-6, pricing.InputCostPerToken, 1e-12)
	require.InDelta(t, 20e-6, pricing.OutputCostPerToken, 1e-12)
	svc.remoteClient = stubPricingRemoteClient{body: obsoleteSolCatalog(t, map[string]any{"input_cost_per_token": 7e-6})}
	require.NoError(t, svc.ForceUpdate())
	require.NoError(t, os.Remove(svc.cfg.Pricing.OverrideFile))
	svc.reloadIfCustomFilesChanged()
	require.InDelta(t, 7e-6, svc.GetModelPricing("gpt-5.6-sol").InputCostPerToken, 1e-12)
}

func TestSolDefaultMigrationRawRatesAndMetadata(t *testing.T) {
	var catalog map[string]json.RawMessage
	require.NoError(t, json.Unmarshal([]byte(obsoleteSolCatalog(t, map[string]any{
		"custom_field": map[string]any{"keep": true}, "supports_service_tier": false,
		"long_context_input_token_threshold": 0,
	})), &catalog))
	var before map[string]any
	require.NoError(t, json.Unmarshal(catalog["gpt-5.6-sol"], &before))
	migrateObsoleteDefaultSolPricing(catalog)
	var after map[string]any
	require.NoError(t, json.Unmarshal(catalog["gpt-5.6-sol"], &after))
	bundled, err := os.ReadFile("../../resources/model-pricing/model_prices_and_context_window.json")
	require.NoError(t, err)
	var corrected map[string]map[string]any
	require.NoError(t, json.Unmarshal(bundled, &corrected))
	for field, value := range before {
		baseField, multiplier := field, 1.0
		if strings.Contains(field, "_above_272k_tokens") {
			baseField = strings.Replace(field, "_above_272k_tokens", "", 1)
			multiplier = 2
			if strings.HasPrefix(field, "output_") {
				multiplier = 1.5
			}
		}
		if strings.HasPrefix(field, "input_cost_per_token") || strings.HasPrefix(field, "output_cost_per_token") || strings.HasPrefix(field, "cache_creation_input_token_cost") || strings.HasPrefix(field, "cache_read_input_token_cost") {
			baseRate, ok := corrected["gpt-5.6-sol"][baseField].(float64)
			require.True(t, ok, baseField)
			require.InDelta(t, baseRate*multiplier, after[field], 1e-12, field)
		} else {
			require.Equal(t, value, after[field], field)
		}
	}
	require.Len(t, after, len(before))
	first := catalog["gpt-5.6-sol"]
	migrateObsoleteDefaultSolPricing(catalog)
	require.Equal(t, first, catalog["gpt-5.6-sol"])
	svc := initializedSolPricing(t, solDefaultURL, obsoleteSolCatalog(t, map[string]any{
		"long_context_input_token_threshold": 0, "supports_service_tier": false,
	}), "", true)
	pricing := svc.GetModelPricing("gpt-5.6-sol")
	require.Zero(t, pricing.LongContextInputTokenThreshold)
	require.False(t, pricing.SupportsServiceTier)
	require.InDelta(t, 4e-6, pricing.InputCostPerToken, 1e-12)
}

func TestSolDefaultMigrationCustomFallback(t *testing.T) {
	for _, remoteBody := range []string{`{"other":{"input_cost_per_token":1e-6}}`, `{`} {
		t.Run(remoteBody, func(t *testing.T) {
			cfg := &config.Config{Pricing: config.PricingConfig{RemoteURL: solDefaultURL, DataDir: t.TempDir(), UpdateIntervalHours: 24}}
			cfg.Pricing.FallbackFile = filepath.Join(cfg.Pricing.DataDir, "fallback.json")
			cfg.Pricing.OverrideFile = filepath.Join(cfg.Pricing.DataDir, "override.json")
			require.NoError(t, os.WriteFile(cfg.Pricing.FallbackFile, []byte(obsoleteSolCatalog(t, nil)), 0644))
			require.NoError(t, os.WriteFile(cfg.Pricing.OverrideFile, []byte(`{"gpt-5.6-sol":{"cache_read_input_token_cost":0}}`), 0644))
			svc := NewPricingService(cfg, stubPricingRemoteClient{body: remoteBody})
			require.NoError(t, svc.Initialize())
			t.Cleanup(svc.Stop)
			pricing := svc.GetModelPricing("gpt-5.6-sol")
			require.InDelta(t, 5e-6, pricing.InputCostPerToken, 1e-12)
			require.Zero(t, pricing.CacheReadInputTokenCost)
			require.NoError(t, os.WriteFile(cfg.Pricing.OverrideFile, []byte(`{"gpt-5.6-sol":{"output_cost_per_token":0}}`), 0644))
			svc.reloadIfCustomFilesChanged()
			pricing = svc.GetModelPricing("gpt-5.6-sol")
			require.InDelta(t, 5e-6, pricing.InputCostPerToken, 1e-12)
			require.Zero(t, pricing.OutputCostPerToken)
			if remoteBody == "{" {
				restarted := NewPricingService(cfg, stubPricingRemoteClient{body: remoteBody})
				require.NoError(t, restarted.Initialize())
				t.Cleanup(restarted.Stop)
				require.InDelta(t, 5e-6, restarted.GetModelPricing("gpt-5.6-sol").InputCostPerToken, 1e-12)
				svc.remoteClient = stubPricingRemoteClient{body: obsoleteSolCatalog(t, nil)}
				require.NoError(t, svc.ForceUpdate())
				require.InDelta(t, 4e-6, svc.GetModelPricing("gpt-5.6-sol").InputCostPerToken, 1e-12)
				svc.reloadIfCustomFilesChanged()
				require.NoError(t, svc.reloadCustomPricingLayers())
				require.InDelta(t, 4e-6, svc.GetModelPricing("gpt-5.6-sol").InputCostPerToken, 1e-12)
			}
		})
	}
}

func TestSolDefaultMigrationDoesNotInventLongContext(t *testing.T) {
	var catalog map[string]map[string]any
	require.NoError(t, json.Unmarshal([]byte(obsoleteSolCatalog(t, nil)), &catalog))
	patch := map[string]any{}
	for field := range catalog["gpt-5.6-sol"] {
		if strings.Contains(field, "_above_") {
			patch[field] = nil
		}
	}
	svc := initializedSolPricing(t, solDefaultURL, obsoleteSolCatalog(t, patch), "", true)
	pricing := svc.GetModelPricing("gpt-5.6-sol")
	require.InDelta(t, 4e-6, pricing.InputCostPerToken, 1e-12)
	require.Zero(t, pricing.LongContextInputTokenThreshold)
	billing := NewBillingService(&config.Config{}, svc)
	cost, err := billing.CalculateCost("gpt-5.6-sol", UsageTokens{InputTokens: 300000, OutputTokens: 100}, 1)
	require.NoError(t, err)
	require.InDelta(t, 1.202, cost.ActualCost, 1e-12)
	require.False(t, cost.LongContextBillingApplied)
}

func TestSolDefaultMigrationExplicitFastCardWins(t *testing.T) {
	var catalog map[string]json.RawMessage
	require.NoError(t, json.Unmarshal([]byte(obsoleteSolCatalog(t, nil)), &catalog))
	catalog["gpt-5.6-sol-fast"] = json.RawMessage(`{"input_cost_per_token":9e-6,"output_cost_per_token":11e-6}`)
	body, err := json.Marshal(catalog)
	require.NoError(t, err)
	svc := initializedSolPricing(t, solDefaultURL, string(body), "", true)
	require.InDelta(t, 9e-6, svc.GetModelPricing("gpt-5.6-sol-fast").InputCostPerToken, 1e-12)
	require.InDelta(t, 4e-6, svc.GetModelPricing("gpt-5.6-sol").InputCostPerToken, 1e-12)
}

func TestSolDefaultMigrationGroupAndChannelControls(t *testing.T) {
	svc := initializedSolPricing(t, solDefaultURL, obsoleteSolCatalog(t, nil), "", true)
	billing := NewBillingService(&config.Config{}, svc)
	zero, output := 0.0, 3e-6
	channel := ChannelModelPricing{Models: []string{"gpt-5.6-sol"}, BillingMode: BillingModeToken,
		InputPrice: &zero, OutputPrice: &output, CacheWritePrice: &zero, CacheReadPrice: &zero}
	pricing, err := billing.GetModelPricingWithChannel("gpt-5.6-sol", &channel)
	require.NoError(t, err)
	require.Zero(t, pricing.InputPricePerToken)
	require.InDelta(t, 3e-6, pricing.OutputPricePerToken, 1e-12)
	resolver := NewModelPricingResolver(nil, billing)
	group := &Group{ModelPricing: []ChannelModelPricing{channel}, LongContextPricingEnabled: false}
	cost, err := billing.CalculateCostUnified(CostInput{Ctx: context.Background(), Model: "gpt-5.6-sol", Group: group,
		Tokens: UsageTokens{InputTokens: 300000, OutputTokens: 100}, RateMultiplier: 1, Resolver: resolver})
	require.NoError(t, err)
	require.InDelta(t, 0.0003, cost.ActualCost, 1e-12)
	require.False(t, cost.LongContextBillingApplied)
}
