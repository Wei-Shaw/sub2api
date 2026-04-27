// Package service — available-channels aggregation.
//
// The user-facing "available channels" view exposes each active channel
// together with the groups it belongs to and the supported models that
// emerge from the channel's mapping ∪ pricing data. The host previously
// implemented this in backend/internal/service/channel_available.go;
// W8 of the V5 plugin migration moves the logic into this plugin.
//
// Compared with the host implementation we deliberately drop the LiteLLM
// global-pricing fallback (synthesizePricingFromLiteLLM): pricing data is
// not yet exposed through the SDK, so unconfigured models simply return
// nil pricing. V5-CURATE §6 acknowledges this trade-off; restoring the
// fallback is left to V6 once HostServiceProxyCapability is in place.
package service

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/Wei-Shaw/sub2api/plugins/channel-management/internal/domain"
)

// AvailableGroupRef is the slim group projection embedded in the
// available-channels view. Mirrors host service.AvailableGroupRef.
type AvailableGroupRef struct {
	ID               int64
	Name             string
	Platform         string
	SubscriptionType string
	RateMultiplier   float64
	IsExclusive      bool
}

// AvailableChannel is the channel-with-context payload the user-facing
// handler reshapes into a DTO. Pricing fallback (host's LiteLLM step) is
// intentionally absent in the plugin port — see file header.
type AvailableChannel struct {
	ID                 int64
	Name               string
	Description        string
	Status             string
	BillingModelSource string
	RestrictModels     bool
	Groups             []AvailableGroupRef
	SupportedModels    []domain.SupportedModel
}

// AvailableChannelsRepository is the read interface the available-channels
// service depends on. The plugin's repository package implements it on the
// SDK *sql.DB.
type AvailableChannelsRepository interface {
	ListActiveGroups(ctx context.Context) ([]AvailableGroupRef, error)
	ListUserAllowedGroupIDs(ctx context.Context, userID int64) ([]int64, error)
	ListUserActiveSubscriptionGroupIDs(ctx context.Context, userID int64) ([]int64, error)
}

// AvailableChannelsService aggregates channel + group data for the
// user-facing available-channels view. It depends on the existing
// ChannelRepository (for ListAll) plus a slim AvailableChannelsRepository
// for the group / user-permission reads that don't belong to the channel
// CRUD path.
type AvailableChannelsService struct {
	channelRepo   ChannelRepository
	availableRepo AvailableChannelsRepository
}

// NewAvailableChannelsService constructs the service. Both repositories are
// required; nil-argument calls panic at construction time so misconfigured
// wiring is caught at start-up rather than on the first request.
func NewAvailableChannelsService(
	channelRepo ChannelRepository,
	availableRepo AvailableChannelsRepository,
) *AvailableChannelsService {
	if channelRepo == nil {
		panic("channel-management: NewAvailableChannelsService requires a non-nil ChannelRepository")
	}
	if availableRepo == nil {
		panic("channel-management: NewAvailableChannelsService requires a non-nil AvailableChannelsRepository")
	}
	return &AvailableChannelsService{channelRepo: channelRepo, availableRepo: availableRepo}
}

// ListAvailable returns all channels paired with their associated active
// groups and computed supported-model list (mapping ∪ pricing via the
// vendored domain.ChannelView). Channels that point at deleted / inactive
// groups still appear, but the orphaned group ids are silently dropped from
// the output (the host behaves the same way).
//
// The result is sorted by lower-case channel name for stable rendering.
func (s *AvailableChannelsService) ListAvailable(ctx context.Context) ([]AvailableChannel, error) {
	channels, err := s.channelRepo.ListAll(ctx)
	if err != nil {
		return nil, fmt.Errorf("list channels: %w", err)
	}

	groups, err := s.availableRepo.ListActiveGroups(ctx)
	if err != nil {
		return nil, fmt.Errorf("list active groups: %w", err)
	}
	groupByID := make(map[int64]AvailableGroupRef, len(groups))
	for i := range groups {
		groupByID[groups[i].ID] = groups[i]
	}

	out := make([]AvailableChannel, 0, len(channels))
	for i := range channels {
		ch := &channels[i]
		assoc := make([]AvailableGroupRef, 0, len(ch.GroupIDs))
		for _, gid := range ch.GroupIDs {
			if ref, ok := groupByID[gid]; ok {
				assoc = append(assoc, ref)
			}
		}
		sort.SliceStable(assoc, func(i, j int) bool { return assoc[i].Name < assoc[j].Name })

		normalizeBillingModelSource(ch)

		view := toChannelView(ch)
		supported := view.SupportedModels()

		out = append(out, AvailableChannel{
			ID:                 ch.ID,
			Name:               ch.Name,
			Description:        ch.Description,
			Status:             ch.Status,
			BillingModelSource: ch.BillingModelSource,
			RestrictModels:     ch.RestrictModels,
			Groups:             assoc,
			SupportedModels:    supported,
		})
	}

	sort.SliceStable(out, func(i, j int) bool {
		return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
	})
	return out, nil
}

// VisibleGroupIDs computes the set of group ids userID can see, applying the
// same rules as host APIKeyService.GetAvailableGroups:
//
//   - Public standard groups (is_exclusive = false) are visible to everyone.
//   - Exclusive standard groups are visible only when the user appears in
//     user_allowed_groups for that group.
//   - Subscription-type groups are visible only while the user holds a
//     non-expired active subscription on that group.
//
// Returns a set keyed by group id for O(1) intersection in the handler.
func (s *AvailableChannelsService) VisibleGroupIDs(
	ctx context.Context,
	userID int64,
) (map[int64]struct{}, error) {
	if userID == 0 {
		// Anonymous users see nothing — handler should reject before this,
		// but we keep the contract defensive.
		return map[int64]struct{}{}, nil
	}

	groups, err := s.availableRepo.ListActiveGroups(ctx)
	if err != nil {
		return nil, fmt.Errorf("list active groups: %w", err)
	}
	allowedIDs, err := s.availableRepo.ListUserAllowedGroupIDs(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("list user allowed groups: %w", err)
	}
	allowedSet := make(map[int64]struct{}, len(allowedIDs))
	for _, id := range allowedIDs {
		allowedSet[id] = struct{}{}
	}
	subIDs, err := s.availableRepo.ListUserActiveSubscriptionGroupIDs(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("list user subscriptions: %w", err)
	}
	subSet := make(map[int64]struct{}, len(subIDs))
	for _, id := range subIDs {
		subSet[id] = struct{}{}
	}

	visible := make(map[int64]struct{}, len(groups))
	for i := range groups {
		g := &groups[i]
		if g.SubscriptionType == subscriptionTypeSubscription {
			if _, ok := subSet[g.ID]; ok {
				visible[g.ID] = struct{}{}
			}
			continue
		}
		// Standard group path.
		if !g.IsExclusive {
			visible[g.ID] = struct{}{}
			continue
		}
		if _, ok := allowedSet[g.ID]; ok {
			visible[g.ID] = struct{}{}
		}
	}
	return visible, nil
}

// subscriptionTypeSubscription mirrors host domain.SubscriptionTypeSubscription
// without dragging the host import in. Kept package-private so it cannot be
// confused with a publicly stable contract.
const subscriptionTypeSubscription = "subscription"

// SubscriptionStatusActive mirrors host domain.SubscriptionStatusActive. It is
// used by the repository layer when filtering active rows.
const SubscriptionStatusActive = "active"

// normalizeBillingModelSource fills in BillingModelSourceChannelMapped when
// the channel was persisted before the field existed. Mirrors host
// (*Channel).normalizeBillingModelSource.
func normalizeBillingModelSource(c *Channel) {
	if c == nil {
		return
	}
	if c.BillingModelSource == "" {
		c.BillingModelSource = BillingModelSourceChannelMapped
	}
}

// toChannelView converts a service.Channel to the vendored domain.ChannelView
// the SupportedModels algorithm operates on. The conversion lives here (not
// in domain) so the vendored snapshot stays free of plugin-specific imports.
func toChannelView(c *Channel) *domain.ChannelView {
	if c == nil {
		return nil
	}
	view := &domain.ChannelView{
		ModelMapping: c.ModelMapping,
		ModelPricing: make([]domain.ChannelModelPricing, 0, len(c.ModelPricing)),
	}
	for i := range c.ModelPricing {
		p := &c.ModelPricing[i]
		dp := domain.ChannelModelPricing{
			ID:               p.ID,
			ChannelID:        p.ChannelID,
			Platform:         p.Platform,
			Models:           p.Models,
			BillingMode:      string(p.BillingMode),
			InputPrice:       p.InputPrice,
			OutputPrice:      p.OutputPrice,
			CacheWritePrice:  p.CacheWritePrice,
			CacheReadPrice:   p.CacheReadPrice,
			ImageOutputPrice: p.ImageOutputPrice,
			PerRequestPrice:  p.PerRequestPrice,
			Intervals:        make([]domain.PricingInterval, 0, len(p.Intervals)),
		}
		for j := range p.Intervals {
			iv := &p.Intervals[j]
			dp.Intervals = append(dp.Intervals, domain.PricingInterval{
				ID:              iv.ID,
				PricingID:       iv.PricingID,
				MinTokens:       iv.MinTokens,
				MaxTokens:       iv.MaxTokens,
				TierLabel:       iv.TierLabel,
				InputPrice:      iv.InputPrice,
				OutputPrice:     iv.OutputPrice,
				CacheWritePrice: iv.CacheWritePrice,
				CacheReadPrice:  iv.CacheReadPrice,
				PerRequestPrice: iv.PerRequestPrice,
				SortOrder:       iv.SortOrder,
			})
		}
		view.ModelPricing = append(view.ModelPricing, dp)
	}
	return view
}
