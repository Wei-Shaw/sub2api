package service

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type stubDisplayPricingRepo struct {
	settings      DisplayPricingSettings
	providers     []DisplayPricingProvider
	models        []DisplayModelPrice
	settingsReads int
}

func (r *stubDisplayPricingRepo) GetSettings(context.Context) (*DisplayPricingSettings, error) {
	r.settingsReads++
	v := r.settings
	return &v, nil
}
func (r *stubDisplayPricingRepo) UpdateSettings(_ context.Context, v *DisplayPricingSettings) error {
	v.UpdatedAt = time.Now()
	r.settings = *v
	return nil
}
func (r *stubDisplayPricingRepo) ListProviders(context.Context) ([]DisplayPricingProvider, error) {
	return append([]DisplayPricingProvider(nil), r.providers...), nil
}
func (r *stubDisplayPricingRepo) CreateProvider(_ context.Context, p *DisplayPricingProvider) error {
	for i := range r.providers {
		if r.providers[i].Provider == p.Provider {
			return ErrDisplayProviderExists
		}
	}
	p.UpdatedAt = time.Now()
	r.providers = append(r.providers, *p)
	return nil
}
func (r *stubDisplayPricingRepo) UpdateProvider(_ context.Context, p *DisplayPricingProvider) error {
	for i := range r.providers {
		if r.providers[i].Provider == p.Provider {
			p.UpdatedAt = time.Now()
			r.providers[i] = *p
			return nil
		}
	}
	return ErrDisplayProviderNotFound
}
func (r *stubDisplayPricingRepo) DeleteProvider(_ context.Context, provider string) (int64, error) {
	providerIndex := -1
	for i := range r.providers {
		if r.providers[i].Provider == provider {
			providerIndex = i
			break
		}
	}
	if providerIndex < 0 {
		return 0, ErrDisplayProviderNotFound
	}
	r.providers = append(r.providers[:providerIndex], r.providers[providerIndex+1:]...)
	kept := r.models[:0]
	var deleted int64
	for i := range r.models {
		if r.models[i].Provider == provider {
			deleted++
			continue
		}
		kept = append(kept, r.models[i])
	}
	r.models = kept
	return deleted, nil
}
func (r *stubDisplayPricingRepo) ListModels(context.Context) ([]DisplayModelPrice, error) {
	return append([]DisplayModelPrice(nil), r.models...), nil
}
func (r *stubDisplayPricingRepo) GetModel(context.Context, int64) (*DisplayModelPrice, error) {
	return nil, ErrDisplayPriceNotFound
}
func (r *stubDisplayPricingRepo) UpsertModel(_ context.Context, p *DisplayModelPrice) error {
	p.ID = 1
	return nil
}
func (r *stubDisplayPricingRepo) UpdateModel(context.Context, *DisplayModelPrice) error { return nil }
func (r *stubDisplayPricingRepo) DeleteModel(context.Context, int64) error              { return nil }

func TestDisplayPricingBuildCatalogUsesPresentationPricesOnly(t *testing.T) {
	providerRate := 0.2
	modelRate := 0.3
	input, output := 10.0, 20.0
	base := 0.01
	repo := &stubDisplayPricingRepo{
		settings: DisplayPricingSettings{GlobalMultiplier: 1.25, UpdatedAt: time.Unix(1, 0)},
		providers: []DisplayPricingProvider{{
			Provider: "deepseek", DisplayName: "DeepSeek", ProviderNote: "  Peak price note  ",
			PerRequestNote: "Per-request note", ImageNote: "Image note", Currency: "CNY", Multiplier: &providerRate,
		}},
		models: []DisplayModelPrice{
			{ID: 1, Platform: "openai", ModelName: "deepseek-token", ModelNote: "Model launch note", Provider: "deepseek", BillingMode: DisplayBillingModeToken, Currency: "CNY", Enabled: true, OfficialInputPerMillion: &input, OfficialOutputPerMillion: &output, ModelMultiplier: &modelRate},
			{ID: 2, Platform: "openai", ModelName: "deepseek-once", Provider: "deepseek", BillingMode: DisplayBillingModePerRequest, Currency: "CNY", Enabled: true, PerRequestLTE256K: &base},
		},
	}
	groups := []PlazaGroup{{Models: []PlazaModel{
		{Name: "deepseek-token", Platform: "openai", Pricing: &ChannelModelPricing{BillingMode: BillingMode(DisplayBillingModeToken)}},
		{Name: "deepseek-once", Platform: "openai", Pricing: &ChannelModelPricing{BillingMode: BillingMode(DisplayBillingModePerRequest)}},
	}}}
	catalog, err := NewDisplayPricingService(repo).BuildCatalog(context.Background(), groups)
	require.NoError(t, err)
	require.Len(t, catalog.Providers, 1)
	require.Len(t, catalog.Providers[0].Models, 2)
	require.Equal(t, "  Peak price note  ", catalog.Providers[0].ProviderNote)
	require.Equal(t, "Per-request note", catalog.Providers[0].PerRequestNote)
	require.Equal(t, "Image note", catalog.Providers[0].ImageNote)

	var token, once DisplayCatalogModel
	for _, m := range catalog.Providers[0].Models {
		if m.BillingMode == DisplayBillingModeToken {
			token = m
		} else {
			once = m
		}
	}
	require.NotNil(t, token.DisplayPrices)
	require.Equal(t, "Model launch note", token.ModelNote)
	require.InDelta(t, 3, *token.DisplayPrices.InputPerMillion, 1e-12)
	require.InDelta(t, 6, *token.DisplayPrices.OutputPerMillion, 1e-12)
	require.InDelta(t, 0.3, *token.EffectiveMultiplier, 1e-12)
	require.InDelta(t, 0.25, catalog.Providers[0].EffectiveMultiplier, 1e-12)
	require.Equal(t, &DisplayPerRequestPrices{LTE256K: 0.01, From256K512K: 0.015, GT512K: 0.02}, once.PerRequest)
	require.Nil(t, once.EffectiveMultiplier)
	require.Nil(t, once.ModelMultiplier)

	encoded, err := json.Marshal(once)
	require.NoError(t, err)
	require.NotContains(t, string(encoded), "multiplier")
}

func TestDisplayPricingBuildCatalogHidesUnconfiguredAndDisabledModels(t *testing.T) {
	repo := &stubDisplayPricingRepo{
		settings: DisplayPricingSettings{GlobalMultiplier: 1},
		models:   []DisplayModelPrice{{ID: 1, Platform: "openai", ModelName: "disabled", Provider: "openai", BillingMode: DisplayBillingModeToken, Currency: "USD", Enabled: false}},
	}
	groups := []PlazaGroup{{Models: []PlazaModel{
		{Name: "new-unconfigured", Platform: "openai", Pricing: &ChannelModelPricing{BillingMode: BillingMode(DisplayBillingModeToken)}},
		{Name: "disabled", Platform: "openai", Pricing: &ChannelModelPricing{BillingMode: BillingMode(DisplayBillingModeToken)}},
	}}}
	catalog, err := NewDisplayPricingService(repo).BuildCatalog(context.Background(), groups)
	require.NoError(t, err)
	require.Empty(t, catalog.Providers)

	discovered, err := NewDisplayPricingService(repo).ListDiscovered(context.Background(), groups)
	require.NoError(t, err)
	require.Len(t, discovered, 2)
	require.False(t, discovered[1].Configured)
}

func TestDisplayPricingPerRequestOverridesAndDropsMultiplier(t *testing.T) {
	base, mid, high, multiplier := 2.0, 7.0, 9.0, 4.0
	repo := &stubDisplayPricingRepo{providers: []DisplayPricingProvider{{Provider: "openai", DisplayName: "OpenAI", Currency: "USD"}}}
	svc := NewDisplayPricingService(repo)
	item, err := svc.UpsertModel(context.Background(), DisplayModelPrice{
		Platform: "openai", ModelName: "once", Provider: "openai", BillingMode: DisplayBillingModePerRequest,
		Currency: "USD", Enabled: true, ModelMultiplier: &multiplier, PerRequestLTE256K: &base,
		PerRequest256K512KOverride: &mid, PerRequestGT512KOverride: &high,
	})
	require.NoError(t, err)
	require.Nil(t, item.ModelMultiplier)
}

func TestDisplayPricingImageUsesDisplayMultiplier(t *testing.T) {
	providerRate := 0.5
	repo := &stubDisplayPricingRepo{
		settings:  DisplayPricingSettings{GlobalMultiplier: 1.25},
		providers: []DisplayPricingProvider{{Provider: "openai", DisplayName: "OpenAI", Currency: "USD", Multiplier: &providerRate}},
		models:    []DisplayModelPrice{{ID: 1, Platform: "openai", ModelName: "gpt-image-2", Provider: "openai", BillingMode: DisplayBillingModeImage, Currency: "USD", Enabled: true, ImagePrices: []DisplayImagePrice{{Label: "standard", Price: 0.04}}}},
	}
	groups := []PlazaGroup{{Models: []PlazaModel{{Name: "gpt-image-2", Platform: "openai", Pricing: &ChannelModelPricing{BillingMode: BillingMode(DisplayBillingModeImage)}}}}}
	catalog, err := NewDisplayPricingService(repo).BuildCatalog(context.Background(), groups)
	require.NoError(t, err)
	require.InDelta(t, 0.025, catalog.Providers[0].Models[0].ImagePrices[0].Price, 1e-12)
	require.InDelta(t, 0.04, catalog.Providers[0].Models[0].ImageBasePrices[0].Price, 1e-12)
}

func TestDisplayPricingBuildCatalogRecomputesLatestGlobalMultiplier(t *testing.T) {
	providerRate := 0.125
	input := 10.0
	settingsUpdated := time.Unix(10, 0)
	providerUpdated := time.Unix(30, 0)
	modelUpdated := time.Unix(20, 0)
	repo := &stubDisplayPricingRepo{
		settings:  DisplayPricingSettings{GlobalMultiplier: 1, UpdatedAt: settingsUpdated},
		providers: []DisplayPricingProvider{{Provider: "deepseek", DisplayName: "DeepSeek", Currency: "CNY", Multiplier: &providerRate, UpdatedAt: providerUpdated}},
		models:    []DisplayModelPrice{{ID: 1, Platform: "openai", ModelName: "deepseek-live", Provider: "deepseek", BillingMode: DisplayBillingModeToken, Currency: "CNY", Enabled: true, OfficialInputPerMillion: &input, UpdatedAt: modelUpdated}},
	}
	groups := []PlazaGroup{{Models: []PlazaModel{{Name: "deepseek-live", Platform: "openai", Pricing: &ChannelModelPricing{BillingMode: BillingMode(DisplayBillingModeToken)}}}}}
	svc := NewDisplayPricingService(repo)

	first, err := svc.BuildCatalog(context.Background(), groups)
	require.NoError(t, err)
	require.InDelta(t, 0.125, *first.Providers[0].Models[0].EffectiveMultiplier, 1e-12)
	require.InDelta(t, 1.25, *first.Providers[0].Models[0].DisplayPrices.InputPerMillion, 1e-12)
	require.Equal(t, providerUpdated, first.UpdatedAt)

	repo.settings.GlobalMultiplier = 1.2
	repo.settings.UpdatedAt = time.Unix(40, 0)
	second, err := svc.BuildCatalog(context.Background(), groups)
	require.NoError(t, err)
	require.Equal(t, 2, repo.settingsReads)
	require.InDelta(t, 0.15, second.Providers[0].EffectiveMultiplier, 1e-12)
	require.InDelta(t, 0.15, *second.Providers[0].Models[0].EffectiveMultiplier, 1e-12)
	require.InDelta(t, 1.5, *second.Providers[0].Models[0].DisplayPrices.InputPerMillion, 1e-12)
	require.Equal(t, repo.settings.UpdatedAt, second.UpdatedAt)
}

func TestDisplayPricingModelCurrencyMustMatchProvider(t *testing.T) {
	repo := &stubDisplayPricingRepo{providers: []DisplayPricingProvider{{Provider: "deepseek", DisplayName: "DeepSeek", Currency: "CNY"}}}
	input := 1.0
	_, err := NewDisplayPricingService(repo).UpsertModel(context.Background(), DisplayModelPrice{
		Platform: "openai", ModelName: "deepseek-test", Provider: "deepseek", BillingMode: DisplayBillingModeToken,
		Currency: "USD", Enabled: true, OfficialInputPerMillion: &input,
	})
	require.ErrorIs(t, err, ErrDisplayPriceInvalid)
}

func TestDisplayPricingModelMultiplierIsAbsoluteOverride(t *testing.T) {
	providerRate, modelRate, input := 0.125, 0.2, 10.0
	repo := &stubDisplayPricingRepo{
		settings:  DisplayPricingSettings{GlobalMultiplier: 1.2},
		providers: []DisplayPricingProvider{{Provider: "deepseek", DisplayName: "DeepSeek", Currency: "CNY", Multiplier: &providerRate}},
		models:    []DisplayModelPrice{{ID: 1, Platform: "openai", ModelName: "deepseek-fixed", Provider: "deepseek", BillingMode: DisplayBillingModeToken, Currency: "CNY", Enabled: true, OfficialInputPerMillion: &input, ModelMultiplier: &modelRate}},
	}
	groups := []PlazaGroup{{Models: []PlazaModel{{Name: "deepseek-fixed", Platform: "openai", Pricing: &ChannelModelPricing{BillingMode: BillingMode(DisplayBillingModeToken)}}}}}

	catalog, err := NewDisplayPricingService(repo).BuildCatalog(context.Background(), groups)
	require.NoError(t, err)
	require.InDelta(t, 0.2, *catalog.Providers[0].Models[0].EffectiveMultiplier, 1e-12)
	require.InDelta(t, 2, *catalog.Providers[0].Models[0].DisplayPrices.InputPerMillion, 1e-12)
}

func TestDisplayPricingProviderCRUDAndLogoValidation(t *testing.T) {
	repo := &stubDisplayPricingRepo{}
	svc := NewDisplayPricingService(repo)
	rate := 0.125
	created, err := svc.CreateProvider(context.Background(), DisplayPricingProvider{
		Provider: "Custom_AI", DisplayName: "Custom AI", Currency: "cny", Multiplier: &rate,
		ProviderNote: "  Token note  ", PerRequestNote: "  Request note  ", ImageNote: "  Image note  ",
		LogoKey: "Custom-AI", LogoURL: "https://cdn.example.com/logo.svg", SortOrder: 9,
	})
	require.NoError(t, err)
	require.Equal(t, "custom_ai", created.Provider)
	require.Equal(t, "custom-ai", created.LogoKey)
	require.Equal(t, "Token note", created.ProviderNote)
	require.Equal(t, "Request note", created.PerRequestNote)
	require.Equal(t, "Image note", created.ImageNote)

	created.DisplayName = "Custom AI 2"
	created.LogoURL = "/assets/providers/custom.svg"
	updated, err := svc.UpdateProvider(context.Background(), created.Provider, *created)
	require.NoError(t, err)
	require.Equal(t, "Custom AI 2", updated.DisplayName)

	repo.models = []DisplayModelPrice{{Provider: created.Provider}, {Provider: created.Provider}, {Provider: "other"}}
	deleted, err := svc.DeleteProvider(context.Background(), created.Provider)
	require.NoError(t, err)
	require.EqualValues(t, 2, deleted)
	require.Len(t, repo.models, 1)

	for _, invalid := range []DisplayPricingProvider{
		{Provider: "bad provider", DisplayName: "Bad", Currency: "USD"},
		{Provider: "safe", DisplayName: "Bad", Currency: "USD", LogoKey: "../bad"},
		{Provider: "safe", DisplayName: "Bad", Currency: "USD", LogoURL: "javascript:alert(1)"},
		{Provider: "safe", DisplayName: "Bad", Currency: "USD", LogoURL: "http://example.com/logo.svg"},
		{Provider: "safe", DisplayName: "Bad", Currency: "USD", LogoURL: "../secrets/logo.svg"},
		{Provider: "safe", DisplayName: "Bad", Currency: "USD", LogoURL: "/%5c%5cevil.example/logo.svg"},
	} {
		_, err := svc.CreateProvider(context.Background(), invalid)
		require.ErrorIs(t, err, ErrDisplayProviderInvalid)
	}
}

func TestDisplayPricingNotesAreTrimmedAndLengthLimited(t *testing.T) {
	repo := &stubDisplayPricingRepo{providers: []DisplayPricingProvider{{Provider: "deepseek", DisplayName: "DeepSeek", Currency: "CNY"}}}
	svc := NewDisplayPricingService(repo)
	input := 1.0

	model, err := svc.UpsertModel(context.Background(), DisplayModelPrice{
		Platform: "openai", ModelName: "deepseek-test", Provider: "deepseek", BillingMode: DisplayBillingModeToken,
		Currency: "CNY", Enabled: true, ModelNote: "  model note  ", OfficialInputPerMillion: &input,
	})
	require.NoError(t, err)
	require.Equal(t, "model note", model.ModelNote)

	_, err = svc.UpsertModel(context.Background(), DisplayModelPrice{
		Platform: "openai", ModelName: "too-long", Provider: "deepseek", BillingMode: DisplayBillingModeToken,
		Currency: "CNY", Enabled: true, ModelNote: strings.Repeat("备", maxDisplayModelNoteLength+1), OfficialInputPerMillion: &input,
	})
	require.ErrorIs(t, err, ErrDisplayPriceInvalid)

	_, err = svc.CreateProvider(context.Background(), DisplayPricingProvider{
		Provider: "long-note", DisplayName: "Long", ProviderNote: strings.Repeat("n", maxDisplayProviderNoteLength+1), Currency: "USD",
	})
	require.ErrorIs(t, err, ErrDisplayProviderInvalid)

	for _, provider := range []DisplayPricingProvider{
		{Provider: "long-request-note", DisplayName: "Long", PerRequestNote: strings.Repeat("n", maxDisplayProviderNoteLength+1), Currency: "USD"},
		{Provider: "long-image-note", DisplayName: "Long", ImageNote: strings.Repeat("n", maxDisplayProviderNoteLength+1), Currency: "USD"},
	} {
		_, err = svc.CreateProvider(context.Background(), provider)
		require.ErrorIs(t, err, ErrDisplayProviderInvalid)
	}
}
