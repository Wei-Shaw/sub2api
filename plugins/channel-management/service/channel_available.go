// Package service — available-channels aggregation.
//
// The user-facing "available channels" view exposes each active channel
// together with the groups it belongs to and the supported models that
// emerge from the channel's mapping ∪ pricing data. The host previously
// implemented this in backend/internal/service/channel_available.go;
// W8 of the V5 plugin migration moves the logic into this plugin.
//
// Pricing fallback (ADR-UPSTREAM-SYNC-115-121 §4): after W8 moved the
// view out of the host, unconfigured models rendered as "未配置" because
// LiteLLM global pricing was only reachable inside the host process.
// The HostService reverse-gRPC channel restores that fallback: for every
// SupportedModel with nil Pricing, the view calls
// ctx.Host().ResolveModelPricing(name) and synthesises a display-only
// ChannelModelPricing row from whatever the host returns. Host errors /
// Unimplemented are silently tolerated (caller keeps nil pricing), so
// older hosts without HostService keep the W8 behaviour instead of
// breaking the page.
package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"sync/atomic"

	"github.com/shopspring/decimal"

	pluginsdk "github.com/Wei-Shaw/sub2api/plugin-sdk"
	"github.com/Wei-Shaw/sub2api/plugins/channel-management/internal/decimalx"
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
//
// The host field is the plugin-sdk HostClient — set to nil in tests
// that don't need pricing fallback; ListAvailable tolerates nil by
// skipping the enrichment step.
type AvailableChannelsService struct {
	channelRepo   ChannelRepository
	availableRepo AvailableChannelsRepository
	host          pluginsdk.HostClient
	logger        *slog.Logger

	// hostUnavailableLogged makes the "host pricing unavailable" log
	// a one-shot — older hosts never ship HostService, so the
	// fallback enrichment loop would spam the plugin log on every
	// page render without this latch. 0 = not logged, 1 = logged.
	hostUnavailableLogged atomic.Uint32
}

// NewAvailableChannelsService constructs the service. The channel + group
// repositories are required; nil-argument calls panic at construction
// time so misconfigured wiring is caught at start-up rather than on the
// first request. The host client is optional: pass nil to disable the
// LiteLLM pricing fallback (tests, or plugin deployments that opt out).
func NewAvailableChannelsService(
	channelRepo ChannelRepository,
	availableRepo AvailableChannelsRepository,
	host pluginsdk.HostClient,
	logger *slog.Logger,
) *AvailableChannelsService {
	if channelRepo == nil {
		panic("channel-management: NewAvailableChannelsService requires a non-nil ChannelRepository")
	}
	if availableRepo == nil {
		panic("channel-management: NewAvailableChannelsService requires a non-nil AvailableChannelsRepository")
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &AvailableChannelsService{
		channelRepo:   channelRepo,
		availableRepo: availableRepo,
		host:          host,
		logger:        logger,
	}
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
		s.fillGlobalPricingFallback(ctx, supported)

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

// fillGlobalPricingFallback fills display-only pricing for SupportedModels
// that emerged from mapping/pricing without a per-channel pricing row
// (Pass A "mapping without pricing" and the upstream "pricing-only but
// absent from channel pricing" cases). For each such entry the service
// asks the host for globally-known pricing (LiteLLM + host fallbacks)
// via HostService.ResolveModelPricing and synthesises a display-only
// ChannelModelPricing row. Matches upstream
// backend/internal/service/channel_available.go behaviour before W8
// moved the view out of the host.
//
// Failure modes are deliberately silent:
//   - Nil host client (tests, or plugin explicitly opts out): no-op.
//   - ErrHostPricingUnavailable (older host): logged once at Info,
//     subsequent calls stay silent so page renders never spam logs.
//   - Any other error (ctx canceled, host transiently down): logged at
//     Debug and the model keeps nil pricing. The popover falls back to
//     "未配置" which matches the pre-fallback behaviour.
//
// Never returns an error: pricing enrichment is strictly a rendering
// nicety and must not abort the "Available Channels" page.
func (s *AvailableChannelsService) fillGlobalPricingFallback(
	ctx context.Context,
	models []domain.SupportedModel,
) {
	if s == nil || s.host == nil || len(models) == 0 {
		return
	}
	for i := range models {
		if models[i].Pricing != nil {
			continue
		}
		pricing, err := s.host.ResolveModelPricing(ctx, models[i].Name)
		if err != nil {
			s.noteHostPricingError(err)
			continue
		}
		if pricing == nil {
			// Host has no data for this model — keep nil pricing, the
			// popover renders it as "未配置" just like before.
			continue
		}
		models[i].Pricing = synthesizePricingFromHost(pricing)
	}
}

// noteHostPricingError logs host pricing failures conservatively: the
// "older host / feature not wired" case is logged once at Info so
// operators can spot missing wiring, everything else is per-call Debug.
// We deliberately do NOT log at Warn/Error — pricing fallback is a
// cosmetic enhancement and noisy logs would train operators to ignore
// the channel-management plugin.
func (s *AvailableChannelsService) noteHostPricingError(err error) {
	if errors.Is(err, pluginsdk.ErrHostPricingUnavailable) {
		if s.hostUnavailableLogged.CompareAndSwap(0, 1) {
			s.logger.Info("host pricing fallback unavailable; available-channels view will render LiteLLM-less popovers",
				"error", err)
		}
		return
	}
	s.logger.Debug("host pricing lookup failed; skipping fallback for this model",
		"error", err)
}

// synthesizePricingFromHost converts the SDK's HostModelPricing into the
// vendored domain.ChannelModelPricing shape used by SupportedModels
// consumers. Mirrors host synthesizePricingFromLiteLLM from
// backend/internal/service/channel_available.go@6cd7c6054: BillingMode is
// fixed to token (the LiteLLM fallback is always per-token), zero values
// stay absent (*float64(nil)), and only price fields are populated —
// interval tiers and platform come from real channel-level pricing rows
// which this path intentionally does not have.
func synthesizePricingFromHost(hp *pluginsdk.HostModelPricing) *domain.ChannelModelPricing {
	if hp == nil {
		return nil
	}
	return &domain.ChannelModelPricing{
		BillingMode:      string(BillingModeToken),
		InputPrice:       hostDecimalToFloat64Ptr(hp.InputPrice),
		OutputPrice:      hostDecimalToFloat64Ptr(hp.OutputPrice),
		CacheWritePrice:  hostDecimalToFloat64Ptr(hp.CacheWritePrice),
		CacheReadPrice:   hostDecimalToFloat64Ptr(hp.CacheReadPrice),
		ImageOutputPrice: hostDecimalToFloat64Ptr(hp.ImageOutputPrice),
	}
}

// hostDecimalToFloat64Ptr demotes the SDK's *decimal.Decimal into the
// vendored domain.ChannelModelPricing's *float64 fields. nil / zero
// collapse to nil so the popover renders "未配置" for missing dimensions
// (LiteLLM leaves e.g. CacheWritePrice unset for most models).
//
// Float demotion is acceptable here because the value only drives the
// display popover — nothing on the real billing hot path consumes it.
// The real billing path still goes through BillingService inside the
// host with full decimal precision.
func hostDecimalToFloat64Ptr(d *decimal.Decimal) *float64 {
	if d == nil || d.IsZero() {
		return nil
	}
	f, _ := d.Float64()
	if f <= 0 {
		return nil
	}
	return &f
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
		// domain.ChannelModelPricing is a vendored byte-for-byte snapshot
		// of the host's struct (see internal/domain/channel_view.go), so
		// the price fields stay *float64. We translate decimal →
		// *float64 here at the conversion boundary; the SupportedModels
		// algorithm only inspects model names, not prices, so the
		// inexact float demotion does not influence its output.
		dp := domain.ChannelModelPricing{
			ID:               p.ID,
			ChannelID:        p.ChannelID,
			Platform:         p.Platform,
			Models:           p.Models,
			BillingMode:      string(p.BillingMode),
			InputPrice:       decimalx.ToFloat64Ptr(p.InputPrice),
			OutputPrice:      decimalx.ToFloat64Ptr(p.OutputPrice),
			CacheWritePrice:  decimalx.ToFloat64Ptr(p.CacheWritePrice),
			CacheReadPrice:   decimalx.ToFloat64Ptr(p.CacheReadPrice),
			ImageOutputPrice: decimalx.ToFloat64Ptr(p.ImageOutputPrice),
			PerRequestPrice:  decimalx.ToFloat64Ptr(p.PerRequestPrice),
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
				InputPrice:      decimalx.ToFloat64Ptr(iv.InputPrice),
				OutputPrice:     decimalx.ToFloat64Ptr(iv.OutputPrice),
				CacheWritePrice: decimalx.ToFloat64Ptr(iv.CacheWritePrice),
				CacheReadPrice:  decimalx.ToFloat64Ptr(iv.CacheReadPrice),
				PerRequestPrice: decimalx.ToFloat64Ptr(iv.PerRequestPrice),
				SortOrder:       iv.SortOrder,
			})
		}
		view.ModelPricing = append(view.ModelPricing, dp)
	}
	return view
}
