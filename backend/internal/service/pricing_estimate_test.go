//go:build unit

package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

type pricingEstimateAccountRepo struct {
	AccountRepository
	accounts         []Account
	err              error
	requestedGroupID int64
}

func (r *pricingEstimateAccountRepo) ListSchedulableByGroupID(_ context.Context, groupID int64) ([]Account, error) {
	r.requestedGroupID = groupID
	return r.accounts, r.err
}

func pricingEstimateFALAccount(endpoint string) Account {
	return Account{
		Platform: PlatformFal,
		Credentials: map[string]any{
			"model_mapping": map[string]any{endpoint: "upstream/provider/model"},
		},
	}
}

func TestGatewayServiceEstimateImagePricingUsesGroupTiers(t *testing.T) {
	price1K, price2K, price4K := 0.10, 0.20, 0.40
	repo := &pricingEstimateAccountRepo{accounts: []Account{pricingEstimateFALAccount("fal-ai/flux/dev")}}
	svc := &GatewayService{
		billingService: &BillingService{},
		accountRepo:    repo,
	}
	groupID := int64(9)
	apiKey := &APIKey{GroupID: &groupID, Group: &Group{
		ID: 9, RateMultiplier: 1.5,
		ImagePrice1K: &price1K, ImagePrice2K: &price2K, ImagePrice4K: &price4K,
		ImageResolution1K: "1024x1024",
		ImageResolution2K: "2048x2048",
		ImageResolution4K: "4096x4096",
	}}

	estimate, err := svc.EstimateImagePricing(
		context.Background(), apiKey, "fal-ai/flux/dev",
		ImageDimensions{Width: 800, Height: 800}, "high", 2,
	)
	require.NoError(t, err)
	require.Equal(t, "1K", estimate.Tier)
	require.Equal(t, PricingSourceGroup, estimate.PricingSource)
	require.InDelta(t, 0.10, estimate.UnitPrice, 1e-9)
	require.InDelta(t, 0.20, estimate.TotalCost, 1e-9)
	require.InDelta(t, 0.30, estimate.EstimatedPrice, 1e-9)
	require.Equal(t, groupID, repo.requestedGroupID)
}

func TestGatewayServiceEstimateImagePricingRejectsInvalidDimensions(t *testing.T) {
	svc := &GatewayService{
		billingService: &BillingService{},
		accountRepo:    &pricingEstimateAccountRepo{accounts: []Account{pricingEstimateFALAccount("fal-ai/flux/dev")}},
	}
	apiKey := &APIKey{Group: &Group{ID: 9, RateMultiplier: 1}}
	_, err := svc.EstimateImagePricing(
		context.Background(), apiKey, "fal-ai/flux/dev",
		ImageDimensions{Width: 1000, Height: 3001}, "", 1,
	)
	require.ErrorContains(t, err, "1:3")
}

func TestGatewayServiceEstimateImagePricingRejectsModelUnsupportedByAPIKeyGroup(t *testing.T) {
	price := 0.1
	svc := &GatewayService{
		billingService: &BillingService{},
		accountRepo: &pricingEstimateAccountRepo{accounts: []Account{{
			Platform: PlatformFal,
			Credentials: map[string]any{
				"model_mapping": map[string]any{"public-image-model": "made-up/provider/model"},
			},
		}}},
	}
	apiKey := &APIKey{Group: &Group{
		ID: 9, ImagePrice1K: &price,
		ImageResolution1K: "1024x1024", ImageResolution2K: "2048x2048", ImageResolution4K: "4096x4096",
	}}

	_, err := svc.EstimateImagePricing(
		context.Background(), apiKey, "made-up/provider/model",
		ImageDimensions{Width: 800, Height: 800}, "high", 1,
	)
	require.ErrorIs(t, err, ErrImagePricingModelUnsupported)
}

func TestGatewayServiceEstimateImagePricingAcceptsGroupModelWildcard(t *testing.T) {
	price := 0.1
	svc := &GatewayService{
		billingService: &BillingService{},
		accountRepo:    &pricingEstimateAccountRepo{accounts: []Account{pricingEstimateFALAccount("fal-ai/flux/*")}},
	}
	apiKey := &APIKey{Group: &Group{
		ID: 9, ImagePrice1K: &price,
		ImageResolution1K: "1024x1024", ImageResolution2K: "2048x2048", ImageResolution4K: "4096x4096",
	}}

	estimate, err := svc.EstimateImagePricing(
		context.Background(), apiKey, "fal-ai/flux/dev",
		ImageDimensions{Width: 800, Height: 800}, "high", 1,
	)
	require.NoError(t, err)
	require.Equal(t, "1K", estimate.Tier)
}

func TestModelPricingResolverGetImageTierPrice(t *testing.T) {
	price1K, price2K, price4K := 0.1, 0.2, 0.4
	resolved := &ResolvedPricing{Mode: BillingModeImage, RequestTiers: []PricingInterval{
		{TierLabel: "1K", Resolution: "1024x1536", PerRequestPrice: &price1K},
		{TierLabel: "2K", Resolution: "2048x3072", PerRequestPrice: &price2K},
		{TierLabel: "4K", Resolution: "4096x6144", PerRequestPrice: &price4K},
	}}

	price, label, err := (&ModelPricingResolver{}).GetImageTierPrice(
		resolved, ImageDimensions{Width: 1200, Height: 1800}, "",
	)
	require.NoError(t, err)
	require.Equal(t, "2K", label)
	require.InDelta(t, 0.2, price, 1e-9)
}

func TestModelPricingResolverGetImageTierPriceNormalizesConfiguredCase(t *testing.T) {
	price := 0.2
	resolved := &ResolvedPricing{Mode: BillingModeImage, RequestTiers: []PricingInterval{
		{TierLabel: "2k", Resolution: "2048x3072", Quality: "HIGH", PerRequestPrice: &price},
	}}

	got, label, err := (&ModelPricingResolver{}).GetImageTierPrice(
		resolved, ImageDimensions{Width: 1200, Height: 1800}, "high",
	)
	require.NoError(t, err)
	require.Equal(t, "2K", label)
	require.InDelta(t, 0.2, got, 1e-9)
}
