//go:build unit

package admin

import (
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

func float64Ptr(v float64) *float64 { return &v }
func intPtr(v int) *int             { return &v }

// ---------------------------------------------------------------------------
// 1. channelToResponse
// ---------------------------------------------------------------------------

func TestChannelToResponse_NilInput(t *testing.T) {
	require.Nil(t, channelToResponse(nil))
}

func TestChannelToResponse_FullChannel(t *testing.T) {
	now := time.Date(2025, 6, 1, 12, 0, 0, 0, time.UTC)
	ch := &service.Channel{
		ID:                 42,
		Name:               "test-channel",
		Description:        "desc",
		Status:             "active",
		BillingModelSource: "upstream",
		RestrictModels:     true,
		CreatedAt:          now,
		UpdatedAt:          now.Add(time.Hour),
		GroupIDs:           []int64{1, 2, 3},
		ModelPricing: []service.ChannelModelPricing{
			{
				ID:              10,
				Platform:        "openai",
				Models:          []string{"gpt-4"},
				BillingMode:     service.BillingModeToken,
				InputPrice:      float64Ptr(0.01),
				OutputPrice:     float64Ptr(0.03),
				CacheWritePrice: float64Ptr(0.005),
				CacheReadPrice:  float64Ptr(0.002),
				PerRequestPrice: float64Ptr(0.5),
			},
		},
		ModelMapping: map[string]map[string]string{
			"anthropic": {"claude-3-haiku": "claude-haiku-3"},
		},
	}

	resp := channelToResponse(ch)
	require.NotNil(t, resp)
	require.Equal(t, int64(42), resp.ID)
	require.Equal(t, "test-channel", resp.Name)
	require.Equal(t, "desc", resp.Description)
	require.Equal(t, "active", resp.Status)
	require.Equal(t, "upstream", resp.BillingModelSource)
	require.True(t, resp.RestrictModels)
	require.Equal(t, []int64{1, 2, 3}, resp.GroupIDs)
	require.Equal(t, "2025-06-01T12:00:00Z", resp.CreatedAt)
	require.Equal(t, "2025-06-01T13:00:00Z", resp.UpdatedAt)

	// model mapping
	require.Len(t, resp.ModelMapping, 1)
	require.Equal(t, "claude-haiku-3", resp.ModelMapping["anthropic"]["claude-3-haiku"])

	// pricing
	require.Len(t, resp.ModelPricing, 1)
	p := resp.ModelPricing[0]
	require.Equal(t, int64(10), p.ID)
	require.Equal(t, "openai", p.Platform)
	require.Equal(t, []string{"gpt-4"}, p.Models)
	require.Equal(t, "token", p.BillingMode)
	require.Equal(t, float64Ptr(0.01), p.InputPrice)
	require.Equal(t, float64Ptr(0.03), p.OutputPrice)
	require.Equal(t, float64Ptr(0.005), p.CacheWritePrice)
	require.Equal(t, float64Ptr(0.002), p.CacheReadPrice)
	require.Equal(t, float64Ptr(0.5), p.PerRequestPrice)
	require.Empty(t, p.Intervals)
}

func TestChannelToResponse_EmptyDefaults(t *testing.T) {
	now := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	ch := &service.Channel{
		ID:                 1,
		Name:               "ch",
		BillingModelSource: "",
		CreatedAt:          now,
		UpdatedAt:          now,
		GroupIDs:           nil,
		ModelMapping:       nil,
		ModelPricing: []service.ChannelModelPricing{
			{
				Platform:    "",
				BillingMode: "",
				Models:      []string{"m1"},
			},
		},
	}

	resp := channelToResponse(ch)
	require.Equal(t, "channel_mapped", resp.BillingModelSource)
	require.NotNil(t, resp.GroupIDs)
	require.Empty(t, resp.GroupIDs)
	require.NotNil(t, resp.ModelMapping)
	require.Empty(t, resp.ModelMapping)

	require.Len(t, resp.ModelPricing, 1)
	require.Equal(t, "anthropic", resp.ModelPricing[0].Platform)
	require.Equal(t, "token", resp.ModelPricing[0].BillingMode)
}

func TestChannelToResponse_NilModels(t *testing.T) {
	now := time.Now()
	ch := &service.Channel{
		ID:        1,
		Name:      "ch",
		CreatedAt: now,
		UpdatedAt: now,
		ModelPricing: []service.ChannelModelPricing{
			{
				Models: nil,
			},
		},
	}

	resp := channelToResponse(ch)
	require.Len(t, resp.ModelPricing, 1)
	require.NotNil(t, resp.ModelPricing[0].Models)
	require.Empty(t, resp.ModelPricing[0].Models)
}

func TestChannelToResponse_WithIntervals(t *testing.T) {
	now := time.Now()
	ch := &service.Channel{
		ID:        1,
		Name:      "ch",
		CreatedAt: now,
		UpdatedAt: now,
		ModelPricing: []service.ChannelModelPricing{
			{
				Models:      []string{"m1"},
				BillingMode: service.BillingModePerRequest,
				Intervals: []service.PricingInterval{
					{
						ID:              100,
						MinTokens:       0,
						MaxTokens:       intPtr(1000),
						TierLabel:       "1K",
						InputPrice:      float64Ptr(0.01),
						OutputPrice:     float64Ptr(0.02),
						CacheWritePrice: float64Ptr(0.003),
						CacheReadPrice:  float64Ptr(0.001),
						PerRequestPrice: float64Ptr(0.1),
						SortOrder:       1,
					},
					{
						ID:        101,
						MinTokens: 1000,
						MaxTokens: nil,
						TierLabel: "unlimited",
						SortOrder: 2,
					},
				},
			},
		},
	}

	resp := channelToResponse(ch)
	require.Len(t, resp.ModelPricing, 1)
	intervals := resp.ModelPricing[0].Intervals
	require.Len(t, intervals, 2)

	iv0 := intervals[0]
	require.Equal(t, int64(100), iv0.ID)
	require.Equal(t, 0, iv0.MinTokens)
	require.Equal(t, intPtr(1000), iv0.MaxTokens)
	require.Equal(t, "1K", iv0.TierLabel)
	require.Equal(t, float64Ptr(0.01), iv0.InputPrice)
	require.Equal(t, float64Ptr(0.02), iv0.OutputPrice)
	require.Equal(t, float64Ptr(0.003), iv0.CacheWritePrice)
	require.Equal(t, float64Ptr(0.001), iv0.CacheReadPrice)
	require.Equal(t, float64Ptr(0.1), iv0.PerRequestPrice)
	require.Equal(t, 1, iv0.SortOrder)

	iv1 := intervals[1]
	require.Equal(t, int64(101), iv1.ID)
	require.Equal(t, 1000, iv1.MinTokens)
	require.Nil(t, iv1.MaxTokens)
	require.Equal(t, "unlimited", iv1.TierLabel)
	require.Equal(t, 2, iv1.SortOrder)
}

func TestChannelToResponse_MultipleEntries(t *testing.T) {
	now := time.Now()
	ch := &service.Channel{
		ID:        1,
		Name:      "multi",
		CreatedAt: now,
		UpdatedAt: now,
		ModelPricing: []service.ChannelModelPricing{
			{
				ID:          1,
				Platform:    "anthropic",
				Models:      []string{"claude-sonnet-4"},
				BillingMode: service.BillingModeToken,
				InputPrice:  float64Ptr(0.003),
				OutputPrice: float64Ptr(0.015),
			},
			{
				ID:              2,
				Platform:        "openai",
				Models:          []string{"gpt-4", "gpt-4o"},
				BillingMode:     service.BillingModePerRequest,
				PerRequestPrice: float64Ptr(1.0),
			},
			{
				ID:               3,
				Platform:         "gemini",
				Models:           []string{"gemini-2.5-pro"},
				BillingMode:      service.BillingModeImage,
				ImageOutputPrice: float64Ptr(0.05),
				PerRequestPrice:  float64Ptr(0.2),
			},
		},
	}

	resp := channelToResponse(ch)
	require.Len(t, resp.ModelPricing, 3)

	require.Equal(t, int64(1), resp.ModelPricing[0].ID)
	require.Equal(t, "anthropic", resp.ModelPricing[0].Platform)
	require.Equal(t, []string{"claude-sonnet-4"}, resp.ModelPricing[0].Models)
	require.Equal(t, "token", resp.ModelPricing[0].BillingMode)

	require.Equal(t, int64(2), resp.ModelPricing[1].ID)
	require.Equal(t, "openai", resp.ModelPricing[1].Platform)
	require.Equal(t, []string{"gpt-4", "gpt-4o"}, resp.ModelPricing[1].Models)
	require.Equal(t, "per_request", resp.ModelPricing[1].BillingMode)

	require.Equal(t, int64(3), resp.ModelPricing[2].ID)
	require.Equal(t, "gemini", resp.ModelPricing[2].Platform)
	require.Equal(t, []string{"gemini-2.5-pro"}, resp.ModelPricing[2].Models)
	require.Equal(t, "image", resp.ModelPricing[2].BillingMode)
	require.Equal(t, float64Ptr(0.05), resp.ModelPricing[2].ImageOutputPrice)
}

// ---------------------------------------------------------------------------
// 2. pricingRequestToService
// ---------------------------------------------------------------------------

func TestPricingRequestToService_Defaults(t *testing.T) {
	tests := []struct {
		name      string
		req       channelModelPricingRequest
		wantField string // which default field to check
		wantValue string
	}{
		{
			name: "empty billing mode defaults to token",
			req: channelModelPricingRequest{
				Models:      []string{"m1"},
				BillingMode: "",
			},
			wantField: "BillingMode",
			wantValue: string(service.BillingModeToken),
		},
		{
			name: "empty platform defaults to anthropic",
			req: channelModelPricingRequest{
				Models:   []string{"m1"},
				Platform: "",
			},
			wantField: "Platform",
			wantValue: "anthropic",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := pricingRequestToService([]channelModelPricingRequest{tt.req})
			require.Len(t, result, 1)
			switch tt.wantField {
			case "BillingMode":
				require.Equal(t, service.BillingMode(tt.wantValue), result[0].BillingMode)
			case "Platform":
				require.Equal(t, tt.wantValue, result[0].Platform)
			}
		})
	}
}

func TestPricingRequestToService_WithAllFields(t *testing.T) {
	reqs := []channelModelPricingRequest{
		{
			Platform:         "openai",
			Models:           []string{"gpt-4", "gpt-4o"},
			BillingMode:      "per_request",
			InputPrice:       float64Ptr(0.01),
			OutputPrice:      float64Ptr(0.03),
			CacheWritePrice:  float64Ptr(0.005),
			CacheReadPrice:   float64Ptr(0.002),
			ImageOutputPrice: float64Ptr(0.04),
			PerRequestPrice:  float64Ptr(0.5),
		},
	}

	result := pricingRequestToService(reqs)
	require.Len(t, result, 1)
	r := result[0]
	require.Equal(t, "openai", r.Platform)
	require.Equal(t, []string{"gpt-4", "gpt-4o"}, r.Models)
	require.Equal(t, service.BillingModePerRequest, r.BillingMode)
	require.Equal(t, float64Ptr(0.01), r.InputPrice)
	require.Equal(t, float64Ptr(0.03), r.OutputPrice)
	require.Equal(t, float64Ptr(0.005), r.CacheWritePrice)
	require.Equal(t, float64Ptr(0.002), r.CacheReadPrice)
	require.Equal(t, float64Ptr(0.04), r.ImageOutputPrice)
	require.Equal(t, float64Ptr(0.5), r.PerRequestPrice)
}

func TestPricingRequestToService_WithIntervals(t *testing.T) {
	reqs := []channelModelPricingRequest{
		{
			Models:      []string{"m1"},
			BillingMode: "per_request",
			Intervals: []pricingIntervalRequest{
				{
					MinTokens:       0,
					MaxTokens:       intPtr(2000),
					TierLabel:       "small",
					InputPrice:      float64Ptr(0.01),
					OutputPrice:     float64Ptr(0.02),
					CacheWritePrice: float64Ptr(0.003),
					CacheReadPrice:  float64Ptr(0.001),
					PerRequestPrice: float64Ptr(0.1),
					SortOrder:       1,
				},
				{
					MinTokens: 2000,
					MaxTokens: nil,
					TierLabel: "large",
					SortOrder: 2,
				},
			},
		},
	}

	result := pricingRequestToService(reqs)
	require.Len(t, result, 1)
	require.Len(t, result[0].Intervals, 2)

	iv0 := result[0].Intervals[0]
	require.Equal(t, 0, iv0.MinTokens)
	require.Equal(t, intPtr(2000), iv0.MaxTokens)
	require.Equal(t, "small", iv0.TierLabel)
	require.Equal(t, float64Ptr(0.01), iv0.InputPrice)
	require.Equal(t, float64Ptr(0.02), iv0.OutputPrice)
	require.Equal(t, float64Ptr(0.003), iv0.CacheWritePrice)
	require.Equal(t, float64Ptr(0.001), iv0.CacheReadPrice)
	require.Equal(t, float64Ptr(0.1), iv0.PerRequestPrice)
	require.Equal(t, 1, iv0.SortOrder)

	iv1 := result[0].Intervals[1]
	require.Equal(t, 2000, iv1.MinTokens)
	require.Nil(t, iv1.MaxTokens)
	require.Equal(t, "large", iv1.TierLabel)
	require.Equal(t, 2, iv1.SortOrder)
}

func TestPricingRequestToService_EmptySlice(t *testing.T) {
	result := pricingRequestToService([]channelModelPricingRequest{})
	require.NotNil(t, result)
	require.Empty(t, result)
}

func TestPricingRequestToService_NilPriceFields(t *testing.T) {
	reqs := []channelModelPricingRequest{
		{
			Models:      []string{"m1"},
			BillingMode: "token",
			// all price fields are nil by default
		},
	}

	result := pricingRequestToService(reqs)
	require.Len(t, result, 1)
	r := result[0]
	require.Nil(t, r.InputPrice)
	require.Nil(t, r.OutputPrice)
	require.Nil(t, r.CacheWritePrice)
	require.Nil(t, r.CacheReadPrice)
	require.Nil(t, r.ImageOutputPrice)
	require.Nil(t, r.PerRequestPrice)
}

// ---------------------------------------------------------------------------
// 3. validatePricingBillingMode
// ---------------------------------------------------------------------------

func TestValidatePricingBillingMode(t *testing.T) {
	tests := []struct {
		name    string
		pricing []service.ChannelModelPricing
		wantErr bool
	}{
		{
			name: "token mode - valid",
			pricing: []service.ChannelModelPricing{
				{BillingMode: service.BillingModeToken},
			},
			wantErr: false,
		},
		{
			name: "per_request with price - valid",
			pricing: []service.ChannelModelPricing{
				{
					BillingMode:     service.BillingModePerRequest,
					PerRequestPrice: float64Ptr(0.5),
				},
			},
			wantErr: false,
		},
		{
			name: "per_request with intervals - valid",
			pricing: []service.ChannelModelPricing{
				{
					BillingMode: service.BillingModePerRequest,
					Intervals: []service.PricingInterval{
						{MinTokens: 0, MaxTokens: intPtr(1000), PerRequestPrice: float64Ptr(0.1)},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "per_request no price no intervals - invalid",
			pricing: []service.ChannelModelPricing{
				{BillingMode: service.BillingModePerRequest},
			},
			wantErr: true,
		},
		{
			name: "image with price - valid",
			pricing: []service.ChannelModelPricing{
				{
					BillingMode:     service.BillingModeImage,
					PerRequestPrice: float64Ptr(0.2),
				},
			},
			wantErr: false,
		},
		{
			name: "image no price no intervals - invalid",
			pricing: []service.ChannelModelPricing{
				{BillingMode: service.BillingModeImage},
			},
			wantErr: true,
		},
		{
			name:    "empty list - valid",
			pricing: []service.ChannelModelPricing{},
			wantErr: false,
		},
		{
			name: "mixed modes with invalid image - invalid",
			pricing: []service.ChannelModelPricing{
				{
					BillingMode: service.BillingModeToken,
					InputPrice:  float64Ptr(0.01),
				},
				{
					BillingMode:     service.BillingModePerRequest,
					PerRequestPrice: float64Ptr(0.5),
				},
				{
					BillingMode: service.BillingModeImage,
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validatePricingBillingMode(tt.pricing)
			if tt.wantErr {
				require.Error(t, err)
				require.Contains(t, err.Error(), "per-request price or intervals required")
			} else {
				require.NoError(t, err)
			}
		})
	}
}
