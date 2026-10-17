package service

import (
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// stubModelsDevServer starts a stub server serving the given registry JSON and
// points the package-level URL variable at it (prior art: the manifest sync
// tests stub a package-level URL variable with httptest).
func stubModelsDevServer(t *testing.T, registry map[string]modelsDevProvider, status int) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if status != 0 {
			w.WriteHeader(status)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(registry)
	}))
	t.Cleanup(srv.Close)
	t.Setenv("SUB2API_MODELSDEV_PRICING_URL", srv.URL)
	return srv
}

func newTestBillingServiceWithModelsDev(t *testing.T) *BillingService {
	t.Helper()
	s := NewBillingService(nil, nil)
	s.modelsDev = newModelsDevPricingSource(&http.Client{})
	return s
}

// modelsDevTestRegistry builds a synthetic models.dev registry with one provider
// ("openai") holding three models: a fully priced one, a plain one, and one with
// no token prices (image-only analogue).
func modelsDevTestRegistry() map[string]modelsDevProvider {
	return map[string]modelsDevProvider{
		"openai": {
			ID:  "openai",
			API: "https://api.openai.com/v1",
			Models: map[string]modelsDevModel{
				"gpt-99-fresh": {
					ID:   "gpt-99-fresh",
					Name: "GPT-99 Fresh",
					Cost: modelsDevCost{
						Input:      7,
						Output:     21,
						CacheRead:  0.4,
						CacheWrite: 5,
						Tiers: []modelsDevCostTier{
							{Input: 8, Output: 30, Tier: modelsDevTier{Type: "context", Size: 272000}},
						},
					},
				},
				"gpt-5.6-luna": {
					ID:   "gpt-5.6-luna",
					Name: "GPT-5.6 Luna",
					Cost: modelsDevCost{Input: 9, Output: 29},
				},
				"gpt-99-imageonly": {
					ID:   "gpt-99-imageonly",
					Name: "GPT-99 Image Only",
					Cost: modelsDevCost{},
				},
			},
		},
	}
}

func TestBillingServiceModelsDevPricing(t *testing.T) {
	// 每个子测试独立构造 BillingService（models.dev 缓存按实例隔离，避免互相污染）。
	setup := func(t *testing.T) *BillingService {
		t.Helper()
		stubModelsDevServer(t, modelsDevTestRegistry(), 0)
		return newTestBillingServiceWithModelsDev(t)
	}

	t.Run("new model absent from LiteLLM gets models.dev price", func(t *testing.T) {
		s := setup(t)
		pricing, err := s.GetModelPricing("gpt-99-fresh")
		if err != nil {
			t.Fatalf("GetModelPricing() error = %v, want nil", err)
		}
		if pricing == nil {
			t.Fatal("GetModelPricing() = nil, want pricing")
		}
		// $7/M → 7e-6 per token（独立换算，非复算）
		if pricing.InputPricePerToken != 7e-6 {
			t.Errorf("InputPricePerToken = %v, want 7e-6", pricing.InputPricePerToken)
		}
		if pricing.OutputPricePerToken != 21e-6 {
			t.Errorf("OutputPricePerToken = %v, want 21e-6", pricing.OutputPricePerToken)
		}
		if math.Abs(pricing.CacheReadPricePerToken-0.4e-6) > 1e-15 {
			t.Errorf("CacheReadPricePerToken = 0.4e-6, got %v", pricing.CacheReadPricePerToken)
		}
	})

	t.Run("cache write becomes cache creation with breakdown", func(t *testing.T) {
		s := setup(t)
		pricing, err := s.GetModelPricing("gpt-99-fresh")
		if err != nil {
			t.Fatalf("GetModelPricing() error = %v", err)
		}
		if pricing.CacheCreationPricePerToken != 5e-6 {
			t.Errorf("CacheCreationPricePerToken = %v, want 5e-6", pricing.CacheCreationPricePerToken)
		}
		if pricing.CacheCreation5mPrice != 5e-6 {
			t.Errorf("CacheCreation5mPrice = %v, want 5e-6", pricing.CacheCreation5mPrice)
		}
		if !pricing.SupportsCacheBreakdown {
			t.Error("SupportsCacheBreakdown = false, want true (cache prices present)")
		}
	})

	t.Run("context tier becomes long-context ladder", func(t *testing.T) {
		s := setup(t)
		pricing, err := s.GetModelPricing("gpt-99-fresh")
		if err != nil {
			t.Fatalf("GetModelPricing() error = %v", err)
		}
		// multiplier = tier price / base price；threshold 取 tier size
		if pricing.LongContextInputThreshold != 272000 {
			t.Errorf("LongContextInputThreshold = %v, want 272000", pricing.LongContextInputThreshold)
		}
		if pricing.LongContextInputMultiplier != 8.0/7.0 {
			t.Errorf("LongContextInputMultiplier = %v, want %v", pricing.LongContextInputMultiplier, 8.0/7.0)
		}
		if pricing.LongContextOutputMultiplier != 30.0/21.0 {
			t.Errorf("LongContextOutputMultiplier = %v, want %v", pricing.LongContextOutputMultiplier, 30.0/21.0)
		}
	})

	t.Run("effort-suffix variant matches base model", func(t *testing.T) {
		s := setup(t)
		pricing, err := s.GetModelPricing("gpt-5.6-luna-high")
		if err != nil {
			t.Fatalf("GetModelPricing(gpt-5.6-luna-high) error = %v", err)
		}
		if pricing.InputPricePerToken != 9e-6 {
			t.Errorf("InputPricePerToken = %v, want 9e-6 (models.dev gpt-5.6-luna)", pricing.InputPricePerToken)
		}
	})

	t.Run("entry without token prices is skipped (fail-closed)", func(t *testing.T) {
		s := setup(t)
		_, err := s.GetModelPricing("gpt-99-imageonly")
		if err == nil {
			t.Fatal("GetModelPricing(gpt-99-imageonly) error = nil, want pricing unavailable error")
		}
	})

	t.Run("fetch failure falls back to hardcoded fallback chain", func(t *testing.T) {
		stubModelsDevServer(t, nil, http.StatusInternalServerError)
		s := newTestBillingServiceWithModelsDev(t)
		pricing, err := s.GetModelPricing("gpt-5.4")
		if err != nil {
			t.Fatalf("GetModelPricing(gpt-5.4) error = %v, want nil (fallback chain)", err)
		}
		if pricing == nil {
			t.Fatal("pricing = nil, want fallback pricing")
		}
	})

	t.Run("largest context tier wins for the single ladder", func(t *testing.T) {
		s := newTestBillingServiceWithModelsDev(t)
		src := s.modelsDev
		src.mu.Lock()
		src.registry = map[string]modelsDevProvider{
			"openai": {
				ID:  "openai",
				API: "https://api.openai.com/v1",
				Models: map[string]modelsDevModel{
					"gpt-99-tiered": {
						ID: "gpt-99-tiered",
						Cost: modelsDevCost{
							Input:  4,
							Output: 20,
							Tiers: []modelsDevCostTier{
								{Input: 5, Output: 22, Tier: modelsDevTier{Type: "context", Size: 128000}},
								{Input: 8, Output: 30, Tier: modelsDevTier{Type: "context", Size: 272000}},
							},
						},
					},
				},
			},
		}
		src.fetchedAt = time.Now()
		src.mu.Unlock()

		pricing, err := s.GetModelPricing("gpt-99-tiered")
		if err != nil {
			t.Fatalf("GetModelPricing() error = %v", err)
		}
		if pricing.LongContextInputThreshold != 272000 {
			t.Errorf("LongContextInputThreshold = %v, want 272000 (largest tier)", pricing.LongContextInputThreshold)
		}
		if pricing.LongContextInputMultiplier != 8.0/4.0 {
			t.Errorf("LongContextInputMultiplier = %v, want %v", pricing.LongContextInputMultiplier, 8.0/4.0)
		}
	})

	t.Run("stale registry is served when refresh fails", func(t *testing.T) {
		var requests int32
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if atomic.AddInt32(&requests, 1) == 1 {
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(modelsDevTestRegistry())
				return
			}
			w.WriteHeader(http.StatusInternalServerError)
		}))
		t.Cleanup(srv.Close)
		t.Setenv("SUB2API_MODELSDEV_PRICING_URL", srv.URL)
		s := newTestBillingServiceWithModelsDev(t)

		// 第一次：成功拉取
		if _, err := s.GetModelPricing("gpt-99-fresh"); err != nil {
			t.Fatalf("initial GetModelPricing() error = %v", err)
		}
		// 回拨 fetchedAt 使缓存过期；第二次拉取失败 → 必须回退旧缓存
		s.modelsDev.mu.Lock()
		s.modelsDev.fetchedAt = time.Now().Add(-2 * modelsDevPricingTTL)
		s.modelsDev.mu.Unlock()
		pricing, err := s.GetModelPricing("gpt-99-fresh")
		if err != nil {
			t.Fatalf("stale GetModelPricing() error = %v, want nil", err)
		}
		if pricing == nil || pricing.InputPricePerToken != 7e-6 {
			t.Fatalf("stale pricing = %v, want models.dev 7e-6", pricing)
		}
	})

	t.Run("concurrent misses fetch the registry exactly once", func(t *testing.T) {
		var requests int32
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			atomic.AddInt32(&requests, 1)
			time.Sleep(50 * time.Millisecond)
			w.WriteHeader(http.StatusInternalServerError)
		}))
		t.Cleanup(srv.Close)
		t.Setenv("SUB2API_MODELSDEV_PRICING_URL", srv.URL)
		s := newTestBillingServiceWithModelsDev(t)

		const n = 10
		start := make(chan struct{})
		var wg sync.WaitGroup
		for i := 0; i < n; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				<-start
				_, _ = s.GetModelPricing("gpt-99-fresh")
			}()
		}
		close(start)
		wg.Wait()
		if got := atomic.LoadInt32(&requests); got != 1 {
			t.Errorf("upstream requests = %d, want 1 (singleflight dedup)", got)
		}
	})

	t.Run("failed fetch is backed off not retried per request", func(t *testing.T) {
		var requests int32
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			atomic.AddInt32(&requests, 1)
			w.WriteHeader(http.StatusInternalServerError)
		}))
		t.Cleanup(srv.Close)
		t.Setenv("SUB2API_MODELSDEV_PRICING_URL", srv.URL)
		s := newTestBillingServiceWithModelsDev(t)
		_, _ = s.GetModelPricing("gpt-99-fresh")
		_, _ = s.GetModelPricing("gpt-99-fresh")
		_, _ = s.GetModelPricing("gpt-99-fresh")
		if got := atomic.LoadInt32(&requests); got != 1 {
			t.Errorf("upstream requests = %d, want 1 (backoff within window)", got)
		}
	})
}
