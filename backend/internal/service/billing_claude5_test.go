package service

import (
	"math"
	"testing"
)

func TestBillingFallbackPricing_Claude5(t *testing.T) {
	s := &BillingService{fallbackPrices: map[string]*ModelPricing{}}
	s.initFallbackPricing()

	cases := []struct {
		model      string
		inPerMTok  float64
		outPerMTok float64
	}{
		{"claude-opus-5-5", 4, 20},
		{"claude-opus-5", 5, 25},
		{"claude-opus-4-8", 5, 25},
		{"claude-sonnet-5-5", 2, 10},
		{"claude-sonnet-5", 2, 10},
		{"claude-haiku-5-5", 0.1, 0.5},
		{"claude-fable-5-2", 8, 40},
		{"claude-fable-5-1", 10, 50},
		{"claude-fable-5", 10, 50},
		{"us.anthropic.claude-opus-5-5", 4, 20},
		// 旧模型保持原有行为
		{"claude-opus-4-6", 5, 25},
		{"claude-3-opus-20240229", 15, 75},
	}
	for _, c := range cases {
		p := s.getFallbackPricing(c.model)
		if p == nil {
			t.Fatalf("%s: no fallback pricing", c.model)
		}
		if math.Abs(p.InputPricePerToken*1e6-c.inPerMTok) > 1e-9 || math.Abs(p.OutputPricePerToken*1e6-c.outPerMTok) > 1e-9 {
			t.Fatalf("%s: got $%v/$%v per MTok, want $%v/$%v", c.model, p.InputPricePerToken*1e6, p.OutputPricePerToken*1e6, c.inPerMTok, c.outPerMTok)
		}
	}
}

func TestPricingFamilyMatch_SkipsGen5(t *testing.T) {
	s := &PricingService{pricingData: map[string]*LiteLLMModelPricing{
		"claude-opus-4-20250514": {InputCostPerToken: 15e-6, OutputCostPerToken: 75e-6},
		"claude-3-haiku":         {InputCostPerToken: 0.25e-6, OutputCostPerToken: 1.25e-6},
	}}
	for _, m := range []string{"claude-opus-5-5", "claude-haiku-5-5", "claude-fable-5-2"} {
		if p := s.GetModelPricing(m); p != nil {
			t.Fatalf("%s should not fuzzy-match old-generation pricing, got %+v", m, p)
		}
	}
	if p := s.GetModelPricing("claude-opus-4-1"); p == nil {
		t.Fatal("opus 4.1 should still family-match")
	}
}

func TestApplyModelCatalogPricing(t *testing.T) {
	in, out := 1e-6, 2e-6
	base := &ModelPricing{InputPricePerToken: 5e-6, OutputPricePerToken: 25e-6, CacheReadPricePerToken: 0.5e-6}
	got := applyModelCatalogPricing(base, &ModelCatalogEntry{InputPrice: &in, OutputPrice: &out})
	if got.InputPricePerToken != in || got.OutputPricePerToken != out {
		t.Fatalf("catalog prices not applied: %+v", got)
	}
	if got.CacheReadPricePerToken != 0.5e-6 {
		t.Fatalf("unset catalog fields should keep base: %+v", got)
	}
	if base.InputPricePerToken != 5e-6 {
		t.Fatal("base must not be mutated")
	}
	got = applyModelCatalogPricing(nil, &ModelCatalogEntry{InputPrice: &in})
	if got.InputPricePerToken != in || got.OutputPricePerToken != 0 {
		t.Fatalf("nil base: %+v", got)
	}
}
