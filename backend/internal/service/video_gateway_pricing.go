package service

import (
	"sort"
	"strings"
)

const (
	VideoPricingCurrencyCNY           = BillingCurrencyCNY
	VideoPricingVersionSeedance202603 = "ark-seedance-2026-03"
	videoSeedancePricingVersion       = VideoPricingVersionSeedance202603
	videoTokensPerMillion             = 1000000.0
)

type VideoPricingEntry struct {
	Provider                   string
	ModelMatch                 string
	DefaultCNYPerMillionTokens float64
	NoVideoCNYPerMillionTokens float64
	VideoCNYPerMillionTokens   float64
	NoAudioCNYPerMillionTokens float64
	AudioCNYPerMillionTokens   float64
	PricingVersion             string
}

type VideoPricingCatalog struct {
	entries []VideoPricingEntry
}

func NewVideoPricingCatalog(overrides []VideoPricingEntry) *VideoPricingCatalog {
	entries := append([]VideoPricingEntry(nil), defaultVideoPricingEntries()...)
	entries = append(entries, overrides...)
	sort.SliceStable(entries, func(i, j int) bool {
		return len(entries[i].ModelMatch) > len(entries[j].ModelMatch)
	})
	return &VideoPricingCatalog{entries: entries}
}

func (s *VideoGatewayService) SetVideoPricingCatalog(catalog *VideoPricingCatalog) {
	if catalog == nil {
		catalog = NewVideoPricingCatalog(nil)
	}
	s.videoPricing = catalog
}

func (s *VideoGatewayService) videoPricingCatalog() *VideoPricingCatalog {
	if s == nil || s.videoPricing == nil {
		return NewVideoPricingCatalog(nil)
	}
	return s.videoPricing
}

func (s *VideoGatewayService) applyVideoBillingMetadata(task *VideoTask) {
	if task == nil {
		return
	}
	task.Currency = NormalizeBillingCurrency(task.Currency)
	task.PricingSource = NormalizeBillingPricingSource(task.PricingSource)
	task.PricingVersion = NormalizeBillingPricingVersion(task.PricingVersion)
	if strings.EqualFold(strings.TrimSpace(task.Provider), VideoProviderSeedance) {
		task.Currency = BillingCurrencyCNY
		task.PricingSource = PricingSourceProviderUsage
		if _, version, ok := s.videoPricingCatalog().RateCNYPerMillionTokens(task); ok && strings.TrimSpace(version) != "" {
			task.PricingVersion = version
		}
		if task.PricingVersion == "" {
			task.PricingVersion = VideoPricingVersionSeedance202603
		}
		return
	}
	if task.PricingVersion == "" {
		task.PricingVersion = PricingVersionForSource(task.PricingSource)
	}
}

func defaultVideoPricingEntries() []VideoPricingEntry {
	return []VideoPricingEntry{
		{
			Provider:                   VideoProviderSeedance,
			ModelMatch:                 "doubao-seedance-2-0-fast",
			DefaultCNYPerMillionTokens: 37,
			NoVideoCNYPerMillionTokens: 37,
			VideoCNYPerMillionTokens:   22,
			PricingVersion:             videoSeedancePricingVersion,
		},
		{
			Provider:                   VideoProviderSeedance,
			ModelMatch:                 "doubao-seedance-2-0",
			DefaultCNYPerMillionTokens: 46,
			NoVideoCNYPerMillionTokens: 46,
			VideoCNYPerMillionTokens:   28,
			PricingVersion:             videoSeedancePricingVersion,
		},
		{
			Provider:                   VideoProviderSeedance,
			ModelMatch:                 "doubao-seedance-1-5-pro",
			DefaultCNYPerMillionTokens: 8,
			NoAudioCNYPerMillionTokens: 8,
			AudioCNYPerMillionTokens:   16,
			PricingVersion:             videoSeedancePricingVersion,
		},
		{
			Provider:                   VideoProviderSeedance,
			ModelMatch:                 "doubao-seedance-1-0-pro-fast",
			DefaultCNYPerMillionTokens: 4.2,
			PricingVersion:             videoSeedancePricingVersion,
		},
		{
			Provider:                   VideoProviderSeedance,
			ModelMatch:                 "doubao-seedance-1-0-pro",
			DefaultCNYPerMillionTokens: 15,
			PricingVersion:             videoSeedancePricingVersion,
		},
		{
			Provider:                   VideoProviderSeedance,
			ModelMatch:                 "doubao-seedance-1-0-lite",
			DefaultCNYPerMillionTokens: 10,
			PricingVersion:             videoSeedancePricingVersion,
		},
	}
}

func (c *VideoPricingCatalog) RateCNYPerMillionTokens(task *VideoTask) (float64, string, bool) {
	if c == nil || task == nil {
		return 0, "", false
	}
	provider := strings.ToLower(strings.TrimSpace(task.Provider))
	model := normalizeVideoPricingModel(task.Model)
	for _, entry := range c.entries {
		if strings.ToLower(strings.TrimSpace(entry.Provider)) != provider {
			continue
		}
		if !strings.Contains(model, normalizeVideoPricingModel(entry.ModelMatch)) {
			continue
		}
		return entry.rateForTask(task), firstNonEmptyVideo(entry.PricingVersion, videoSeedancePricingVersion), true
	}
	return 0, "", false
}

func (e VideoPricingEntry) rateForTask(task *VideoTask) float64 {
	if task != nil && task.HasVideoInput && e.VideoCNYPerMillionTokens > 0 {
		return e.VideoCNYPerMillionTokens
	}
	if task != nil && !task.HasVideoInput && e.NoVideoCNYPerMillionTokens > 0 {
		return e.NoVideoCNYPerMillionTokens
	}
	if task != nil && videoBoolPtrValue(task.GenerateAudio) && e.AudioCNYPerMillionTokens > 0 {
		return e.AudioCNYPerMillionTokens
	}
	if task != nil && !videoBoolPtrValue(task.GenerateAudio) && e.NoAudioCNYPerMillionTokens > 0 {
		return e.NoAudioCNYPerMillionTokens
	}
	return e.DefaultCNYPerMillionTokens
}

func normalizeVideoPricingModel(model string) string {
	out := strings.ToLower(strings.TrimSpace(model))
	out = strings.ReplaceAll(out, ".", "-")
	out = strings.ReplaceAll(out, "_", "-")
	return out
}

func videoBoolPtrValue(v *bool) bool {
	return v != nil && *v
}
