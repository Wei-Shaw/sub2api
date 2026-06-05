package repository

import (
	"context"
	"fmt"

	"github.com/Wei-Shaw/sub2api/plugins/channel-management/ent"
	"github.com/Wei-Shaw/sub2api/plugins/channel-management/service"
)

// GetByIDEnt is the ent-based implementation of GetByID (POC).
//
// It validates that the SDK SQL driver + ent codegen stack works end-to-end
// through the gRPC proxy. Once proven, the full repository can be migrated
// to ent incrementally.
//
// Differences from the hand-written GetByID:
//   - Only loads the channels row (no group IDs, model pricing, or
//     account-stats pricing rules). The ent schema deliberately omits
//     edges, so those associations remain hand-managed until a full
//     migration.
//   - Returns a *service.Channel with GroupIDs/ModelPricing/
//     AccountStatsPricingRules left as nil.
func (r *channelRepository) GetByIDEnt(ctx context.Context, id int64) (*service.Channel, error) {
	if r.entClient == nil {
		return nil, fmt.Errorf("ent client not initialised")
	}
	ch, err := r.entClient.Channel.Get(ctx, id)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, service.ErrChannelNotFound
		}
		return nil, fmt.Errorf("ent get channel: %w", err)
	}
	return entChannelToDomain(ch), nil
}

// entChannelToDomain converts an ent Channel entity to the service-layer
// domain struct. Only the flat columns are mapped; association fields
// (GroupIDs, ModelPricing, AccountStatsPricingRules) are left nil.
func entChannelToDomain(e *ent.Channel) *service.Channel {
	return &service.Channel{
		ID:                         e.ID,
		Name:                       e.Name,
		Description:                e.Description,
		Status:                     e.Status,
		ModelMapping:               e.ModelMapping,
		BillingModelSource:         e.BillingModelSource,
		RestrictModels:             e.RestrictModels,
		Features:                   e.Features,
		FeaturesConfig:             e.FeaturesConfig,
		ApplyPricingToAccountStats: e.ApplyPricingToAccountStats,
		CreatedAt:                  e.CreatedAt,
		UpdatedAt:                  e.UpdatedAt,
	}
}
