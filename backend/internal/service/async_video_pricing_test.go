//go:build unit

package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

type asyncVideoPricingGroupRepo struct {
	GroupRepository
	group *Group
}

func (r *asyncVideoPricingGroupRepo) GetByIDLite(_ context.Context, id int64) (*Group, error) {
	if r.group != nil && r.group.ID == id {
		return r.group, nil
	}
	return nil, nil
}

func TestAsyncVideoResolvePricingUsesGroupPerModelPrice(t *testing.T) {
	const model = "bytedance/seedance-2.5/text-to-video"
	groupID := int64(38)
	price720P := 0.12
	group := &Group{
		ID: groupID,
		ModelPricing: []ChannelModelPricing{{
			Models:      []string{model},
			BillingMode: BillingModeVideo,
			Intervals: []PricingInterval{{
				TierLabel:       "720p",
				PerRequestPrice: &price720P,
			}},
		}},
	}
	resolver := NewModelPricingResolver(nil, &BillingService{})
	service := &AsyncVideoService{
		pricingResolver: resolver,
		groupRepo:       &asyncVideoPricingGroupRepo{group: group},
	}

	resolved, err := service.resolveVideoPricing(context.Background(), model, &groupID)

	require.NoError(t, err)
	require.NotNil(t, resolved)
	require.Equal(t, PricingSourceGroup, resolved.Source)
	require.Equal(t, BillingModeVideo, resolved.Mode)
	require.InDelta(t, price720P, resolver.GetRequestTierPrice(resolved, "720p"), 1e-9)
}
