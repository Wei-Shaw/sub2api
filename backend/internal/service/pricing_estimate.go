package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

var ErrImagePricingModelUnsupported = errors.New("group does not support model")

type ImagePricingEstimate struct {
	Endpoint       string          `json:"endpoint"`
	BillingMode    string          `json:"billing_mode"`
	PricingSource  string          `json:"pricing_source"`
	Tier           string          `json:"tier"`
	Resolution     ImageDimensions `json:"resolution"`
	ImageCount     int             `json:"image_count"`
	UnitPrice      float64         `json:"unit_price"`
	TotalCost      float64         `json:"total_cost"`
	RateMultiplier float64         `json:"rate_multiplier"`
	EstimatedPrice float64         `json:"estimated_price"`
}

func (s *GatewayService) EstimateImagePricing(
	ctx context.Context,
	apiKey *APIKey,
	endpoint string,
	dimensions ImageDimensions,
	quality string,
	imageCount int,
) (*ImagePricingEstimate, error) {
	if s == nil || s.billingService == nil {
		return nil, fmt.Errorf("billing service is unavailable")
	}
	if apiKey == nil || apiKey.Group == nil {
		return nil, fmt.Errorf("API key group is required")
	}
	endpoint = strings.Trim(strings.TrimSpace(endpoint), "/")
	if endpoint == "" {
		return nil, fmt.Errorf("model endpoint is required")
	}
	if err := s.validateGroupSupportsPricingModel(ctx, apiKey, endpoint); err != nil {
		return nil, err
	}
	if imageCount <= 0 {
		imageCount = 1
	}
	multiplier := resolveImageRateMultiplier(apiKey, apiKey.Group.RateMultiplier)
	gid := apiKey.Group.ID

	if s.resolver != nil {
		resolved := s.resolver.Resolve(ctx, PricingInput{Model: endpoint, GroupID: &gid, Group: apiKey.Group})
		if resolved != nil && (resolved.Mode == BillingModeImage || resolved.Mode == BillingModePerRequest) &&
			(len(resolved.RequestTiers) > 0 || resolved.DefaultPerRequestPrice > 0) {
			tier := ""
			if resolved.Mode == BillingModeImage && len(resolved.RequestTiers) > 0 {
				_, matchedTier, err := s.resolver.GetImageTierPrice(resolved, dimensions, quality)
				if err != nil {
					return nil, err
				}
				tier = matchedTier
			} else {
				matched, err := MatchImagePricingTier(dimensions, apiKey.Group.ImagePricingTiers())
				if err != nil {
					return nil, err
				}
				tier = matched.Label
			}
			cost, err := s.billingService.CalculateCostUnified(CostInput{
				Ctx: ctx, Model: endpoint, GroupID: &gid, Group: apiKey.Group,
				RequestCount: imageCount, SizeTier: dimensions.String(), Quality: quality,
				RateMultiplier: multiplier, Resolver: s.resolver, Resolved: resolved,
			})
			if err != nil {
				return nil, err
			}
			return newImagePricingEstimate(endpoint, dimensions, imageCount, multiplier, tier, resolved.Source, cost), nil
		}
	}

	matched, err := MatchImagePricingTier(dimensions, apiKey.Group.ImagePricingTiers())
	if err != nil {
		return nil, err
	}
	groupConfig := apiKey.Group.BuildImagePriceConfig(dimensions.Width, dimensions.Height, quality)
	cost, err := s.billingService.CalculateImageCostWithQualityValidated(
		endpoint, dimensions.String(), quality, imageCount, groupConfig, multiplier,
	)
	if err != nil {
		return nil, err
	}
	return newImagePricingEstimate(endpoint, dimensions, imageCount, multiplier, matched.Label, PricingSourceGroup, cost), nil
}

func (s *GatewayService) validateGroupSupportsPricingModel(ctx context.Context, apiKey *APIKey, endpoint string) error {
	if s == nil || s.accountRepo == nil {
		return fmt.Errorf("group model validation is unavailable")
	}
	groupID := apiKey.GroupID
	if groupID == nil && apiKey.Group != nil {
		id := apiKey.Group.ID
		groupID = &id
	}
	requestedModel := strings.ToLower(strings.Trim(strings.TrimSpace(endpoint), "/"))
	for _, supportedModel := range s.GetAvailableModels(ctx, groupID, "") {
		pattern := strings.ToLower(strings.Trim(strings.TrimSpace(supportedModel), "/"))
		if pattern == requestedModel || matchWildcard(pattern, requestedModel) {
			return nil
		}
	}
	return fmt.Errorf("%w: %s", ErrImagePricingModelUnsupported, endpoint)
}

func newImagePricingEstimate(endpoint string, dimensions ImageDimensions, imageCount int, multiplier float64, tier, source string, cost *CostBreakdown) *ImagePricingEstimate {
	estimate := &ImagePricingEstimate{
		Endpoint: endpoint, BillingMode: string(BillingModeImage), PricingSource: source,
		Tier: tier, Resolution: dimensions, ImageCount: imageCount, RateMultiplier: multiplier,
	}
	if cost != nil {
		estimate.TotalCost = cost.TotalCost
		estimate.EstimatedPrice = cost.ActualCost
		if imageCount > 0 {
			estimate.UnitPrice = cost.TotalCost / float64(imageCount)
		}
		if cost.BillingMode != "" {
			estimate.BillingMode = cost.BillingMode
		}
	}
	return estimate
}
