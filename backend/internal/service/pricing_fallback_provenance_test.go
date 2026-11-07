package service

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func TestFallbackPricingMarkerWriteFailureDoesNotPersistUnmarkedCache(t *testing.T) {
	for _, existingCache := range []bool{false, true} {
		t.Run(map[bool]string{false: "missing-cache", true: "invalid-cache"}[existingCache], func(t *testing.T) {
			cfg := &config.Config{Pricing: config.PricingConfig{
				RemoteURL: solDefaultURL, DataDir: t.TempDir(), UpdateIntervalHours: 24,
			}}
			cfg.Pricing.FallbackFile = filepath.Join(cfg.Pricing.DataDir, "fallback.json")
			require.NoError(t, os.WriteFile(cfg.Pricing.FallbackFile, []byte(obsoleteSolCatalog(t, nil)), 0644))
			svc := NewPricingService(cfg, stubPricingRemoteClient{body: "{"})
			require.NoError(t, os.Mkdir(svc.fallbackPricingMarkerPath(), 0755))
			if existingCache {
				require.NoError(t, os.WriteFile(svc.getPricingFilePath(), []byte("{"), 0644))
			}
			for _, current := range []*PricingService{svc, NewPricingService(cfg, stubPricingRemoteClient{body: "{"})} {
				require.NoError(t, current.Initialize())
				t.Cleanup(current.Stop)
				require.InDelta(t, 5e-6, current.GetModelPricing("gpt-5.6-sol").InputCostPerToken, 1e-12)
				cached, err := os.ReadFile(current.getPricingFilePath())
				if existingCache {
					require.NoError(t, err)
					require.Equal(t, "{", string(cached))
				} else {
					require.ErrorIs(t, err, os.ErrNotExist)
				}
			}
		})
	}
}
