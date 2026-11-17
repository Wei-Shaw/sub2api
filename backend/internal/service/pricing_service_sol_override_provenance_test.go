package service

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func TestSolDefaultMigrationOverrideProvenance(t *testing.T) {
	body := obsoleteSolCatalog(t, nil)
	for _, source := range []struct {
		name, url                string
		fallback                 bool
		input, output, cacheRead float64
	}{
		{"default", solDefaultURL, false, 4e-6, 20e-6, 0.4e-6},
		{"deploy", solDeployURL, false, 4e-6, 20e-6, 0.4e-6},
		{"custom", "https://example.com/prices.json", false, 5e-6, 30e-6, 0.5e-6},
		{"fallback", solDefaultURL, true, 5e-6, 30e-6, 0.5e-6},
	} {
		t.Run(source.name, func(t *testing.T) {
			for _, override := range []struct {
				name, body               string
				input, output, cacheRead float64
				provenance               tokenPriceOverrides
			}{
				{"none", `{}`, source.input, source.output, source.cacheRead, tokenPriceOverrides{}},
				{"partial", `{"gpt-5.6-sol":{"input_cost_per_token":9e-6}}`, 9e-6, source.output, source.cacheRead, tokenPriceOverrides{input: true}},
				{"explicit_zero", `{"gpt-5.6-sol":{"output_cost_per_token":0,"cache_read_input_token_cost":0}}`, source.input, 0, 0, tokenPriceOverrides{output: true, cacheRead: true}},
				{"obsolete_override_card", body, 5e-6, 30e-6, 0.5e-6, tokenPriceOverrides{input: true, output: true, cacheRead: true}},
				{"null_is_not_explicit", `{"gpt-5.6-sol":{"input_cost_per_token":null,"cache_read_input_token_cost":0}}`, 0, source.output, 0, tokenPriceOverrides{cacheRead: true}},
				{"invalid_entry", `{"gpt-5.6-sol":[]}`, source.input, source.output, source.cacheRead, tokenPriceOverrides{}},
			} {
				t.Run(override.name, func(t *testing.T) {
					cfg := &config.Config{Pricing: config.PricingConfig{
						RemoteURL: source.url, DataDir: t.TempDir(),
					}}
					cfg.Pricing.OverrideFile = filepath.Join(cfg.Pricing.DataDir, "override.json")
					require.NoError(t, os.WriteFile(cfg.Pricing.OverrideFile, []byte(override.body), 0644))
					catalog := body
					if source.fallback {
						cfg.Pricing.FallbackFile = filepath.Join(cfg.Pricing.DataDir, "fallback.json")
						require.NoError(t, os.WriteFile(cfg.Pricing.FallbackFile, []byte(body), 0644))
						catalog = `{"other":{"input_cost_per_token":1e-6}}`
					}
					svc := &PricingService{cfg: cfg}
					data, _, err := svc.buildPricingData([]byte(catalog))
					require.NoError(t, err)
					pricing := data["gpt-5.6-sol"]
					require.NotNil(t, pricing)
					require.InDelta(t, override.input, pricing.InputCostPerToken, 1e-12)
					require.InDelta(t, override.output, pricing.OutputCostPerToken, 1e-12)
					require.InDelta(t, override.cacheRead, pricing.CacheReadInputTokenCost, 1e-12)
					require.Equal(t, override.provenance, pricing.priceOverrides)
					svc.pricingData = data
					billing := NewBillingService(cfg, svc)
					card, err := billing.GetModelPricing("gpt-5.6-sol")
					require.NoError(t, err)
					require.Equal(t, override.provenance, card.priceOverrides)
				})
			}
		})
	}
}

func TestSolDefaultMigrationOverrideReloadClearsProvenance(t *testing.T) {
	for _, fallback := range []bool{false, true} {
		name := "catalog"
		input, output, cacheRead := 4e-6, 20e-6, 0.4e-6
		if fallback {
			name = "fallback"
			input, output, cacheRead = 5e-6, 30e-6, 0.5e-6
		}
		t.Run(name, func(t *testing.T) {
			cfg := &config.Config{Pricing: config.PricingConfig{
				RemoteURL: solDefaultURL, DataDir: t.TempDir(),
			}}
			cfg.Pricing.OverrideFile = filepath.Join(cfg.Pricing.DataDir, "override.json")
			require.NoError(t, os.WriteFile(cfg.Pricing.OverrideFile, []byte(`{"gpt-5.6-sol":{"input_cost_per_token":9e-6,"output_cost_per_token":0,"cache_read_input_token_cost":0}}`), 0644))
			catalog := obsoleteSolCatalog(t, nil)
			if fallback {
				cfg.Pricing.FallbackFile = filepath.Join(cfg.Pricing.DataDir, "fallback.json")
				require.NoError(t, os.WriteFile(cfg.Pricing.FallbackFile, []byte(catalog), 0644))
				catalog = `{"other":{"input_cost_per_token":1e-6}}`
			}
			svc := &PricingService{cfg: cfg}
			require.NoError(t, os.WriteFile(svc.getPricingFilePath(), []byte(catalog), 0644))
			require.NoError(t, svc.loadPricingData(svc.getPricingFilePath()))
			original := svc.GetModelPricing("gpt-5.6-sol")
			require.NotNil(t, original)
			require.InDelta(t, 9e-6, original.InputCostPerToken, 1e-12)
			require.Zero(t, original.OutputCostPerToken)
			require.Zero(t, original.CacheReadInputTokenCost)
			require.Equal(t, tokenPriceOverrides{input: true, output: true, cacheRead: true}, original.priceOverrides)

			require.NoError(t, os.WriteFile(cfg.Pricing.OverrideFile, []byte(`{"gpt-5.6-sol":{"cache_read_input_token_cost":0}}`), 0644))
			svc.reloadIfCustomFilesChanged()
			pricing := svc.GetModelPricing("gpt-5.6-sol")
			require.NotNil(t, pricing)
			require.InDelta(t, input, pricing.InputCostPerToken, 1e-12)
			require.InDelta(t, output, pricing.OutputCostPerToken, 1e-12)
			require.Zero(t, pricing.CacheReadInputTokenCost)
			require.Equal(t, tokenPriceOverrides{cacheRead: true}, pricing.priceOverrides)

			require.NoError(t, os.Remove(cfg.Pricing.OverrideFile))
			svc.reloadIfCustomFilesChanged()
			pricing = svc.GetModelPricing("gpt-5.6-sol")
			require.NotNil(t, pricing)
			require.InDelta(t, input, pricing.InputCostPerToken, 1e-12)
			require.InDelta(t, output, pricing.OutputCostPerToken, 1e-12)
			require.InDelta(t, cacheRead, pricing.CacheReadInputTokenCost, 1e-12)
			require.Equal(t, tokenPriceOverrides{}, pricing.priceOverrides)
			require.Equal(t, tokenPriceOverrides{input: true, output: true, cacheRead: true}, original.priceOverrides)
			persisted, err := os.ReadFile(svc.getPricingFilePath())
			require.NoError(t, err)
			require.JSONEq(t, catalog, string(persisted))
		})
	}
}
