// Package handler — user-facing available channels endpoint.
//
// V5 / W8.3 of the plugin migration: the user-side `available channels`
// view used to live in backend/internal/handler/available_channel_handler.go;
// it now ships as part of the channel-management plugin and reads the user
// identity from the V4 X-Plugin-User-* headers (parsed via the SDK's
// RequestMetadata helper) instead of the host JWT middleware.
//
// Feature switch: the endpoint is gated behind the
// `availableChannelsEnabled` setting (defaults to false / opt-in). When
// disabled the handler still authenticates the caller and then returns an
// empty list — the 401-precedes-feature-check ordering preserves prior
// behaviour for clients that hit the endpoint without auth. Mirrors host
// commit 9ba42aa55 (backend/internal/handler/available_channel_handler.go).
package handler

import (
	"net/http"
	"sort"

	pluginsdk "github.com/Wei-Shaw/sub2api/plugin-sdk"
	"github.com/Wei-Shaw/sub2api/plugins/channel-management/internal/domain"
	"github.com/Wei-Shaw/sub2api/plugins/channel-management/internal/response"
	"github.com/Wei-Shaw/sub2api/plugins/channel-management/service"

	"github.com/gin-gonic/gin"
)

// AvailableChannelHandler exposes the user-facing channel list.
//
// Filtering happens in three layers (mirrors host behaviour):
//  1. Row filter — only Active channels with at least one group the user can
//     see.
//  2. Group filter — within a channel, only the groups the user can see
//     remain.
//  3. Platform filter — supported models are clipped to the platforms of the
//     visible groups, so a channel sitting on both `antigravity` and
//     `anthropic` groups does not leak the unwanted side to a user that
//     only has the `antigravity` group.
//  4. Field whitelist — the response DTO is the user-visible projection
//     (no internal ids, no billing-source, no status flags).
type AvailableChannelHandler struct {
	availableService *service.AvailableChannelsService
	settings         pluginsdk.SettingsClient
}

// NewAvailableChannelHandler constructs the handler. availableService must
// be non-nil; the plugin entry point ensures wiring is complete before the
// route is registered. settings is optional — passing nil disables the
// runtime feature flag check, matching the V5 W8.3 fallback for older
// hosts that do not implement SettingsExtension. Production wires the SDK
// SettingsClient so the admin can flip the master switch without
// redeploying.
func NewAvailableChannelHandler(
	availableService *service.AvailableChannelsService,
	settings pluginsdk.SettingsClient,
) *AvailableChannelHandler {
	if availableService == nil {
		panic("channel-management: NewAvailableChannelHandler requires a non-nil AvailableChannelsService")
	}
	return &AvailableChannelHandler{
		availableService: availableService,
		settings:         settings,
	}
}

// featureEnabled returns the runtime "availableChannelsEnabled" master
// switch. nil settings (older host without SettingsExtension wired) is
// treated as disabled to keep the opt-in semantics consistent — operators
// must flip the switch on intentionally regardless of host version.
func (h *AvailableChannelHandler) featureEnabled(ctx *gin.Context) bool {
	return service.LoadAvailableChannelsRuntime(ctx.Request.Context(), h.settings).Enabled
}

// userAvailableGroup is the slim group projection emitted to the user. The
// host's user_id-specific rate multiplier is intentionally not exposed here
// — the frontend pulls it from /groups/rates so the API-key page and the
// available-channels page stay in lock-step.
type userAvailableGroup struct {
	ID               int64   `json:"id"`
	Name             string  `json:"name"`
	Platform         string  `json:"platform"`
	SubscriptionType string  `json:"subscription_type"`
	RateMultiplier   float64 `json:"rate_multiplier"`
	IsExclusive      bool    `json:"is_exclusive"`
}

// userPricingIntervalDTO is the user-facing tiered-pricing band.
type userPricingIntervalDTO struct {
	MinTokens       int      `json:"min_tokens"`
	MaxTokens       *int     `json:"max_tokens"`
	TierLabel       *string  `json:"tier_label,omitempty"`
	InputPrice      *float64 `json:"input_price"`
	OutputPrice     *float64 `json:"output_price"`
	CacheWritePrice *float64 `json:"cache_write_price"`
	CacheReadPrice  *float64 `json:"cache_read_price"`
	PerRequestPrice *float64 `json:"per_request_price"`
}

// userSupportedModelPricing is the per-model pricing block.
type userSupportedModelPricing struct {
	BillingMode      string                   `json:"billing_mode"`
	InputPrice       *float64                 `json:"input_price"`
	OutputPrice      *float64                 `json:"output_price"`
	CacheWritePrice  *float64                 `json:"cache_write_price"`
	CacheReadPrice   *float64                 `json:"cache_read_price"`
	ImageOutputPrice *float64                 `json:"image_output_price"`
	PerRequestPrice  *float64                 `json:"per_request_price"`
	Intervals        []userPricingIntervalDTO `json:"intervals"`
}

// userSupportedModel is one supported-model entry in the user view.
type userSupportedModel struct {
	Name     string                     `json:"name"`
	Platform string                     `json:"platform"`
	Pricing  *userSupportedModelPricing `json:"pricing"`
}

// userChannelPlatformSection groups the visible groups + supported models of
// a single platform inside one channel record. Aggregating per-platform
// keeps the existing frontend rendering layout untouched.
type userChannelPlatformSection struct {
	Platform        string               `json:"platform"`
	Groups          []userAvailableGroup `json:"groups"`
	SupportedModels []userSupportedModel `json:"supported_models"`
}

// userAvailableChannel is the top-level user-facing channel record.
type userAvailableChannel struct {
	Name        string                       `json:"name"`
	Description string                       `json:"description"`
	Platforms   []userChannelPlatformSection `json:"platforms"`
}

// List handles GET .../available-channels — return the channels the caller
// is permitted to see.
func (h *AvailableChannelHandler) List(c *gin.Context) {
	meta := pluginsdk.RequestMetadata(c.Request)
	if meta.UserID == 0 {
		c.JSON(http.StatusUnauthorized, response.Response{
			Code:    http.StatusUnauthorized,
			Message: "user not authenticated",
			Reason:  "UNAUTHENTICATED",
		})
		return
	}

	// Feature 未启用时返回空数组（不暴露渠道信息）。检查放在认证之后，
	// 保持与未开关前的 401 行为一致：未登录先 401，登录后再按开关决定。
	// 与 host commit 9ba42aa55 行为一致。
	if !h.featureEnabled(c) {
		response.Success(c, []userAvailableChannel{})
		return
	}

	visibleGroups, err := h.availableService.VisibleGroupIDs(c.Request.Context(), meta.UserID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	channels, err := h.availableService.ListAvailable(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	out := make([]userAvailableChannel, 0, len(channels))
	for _, ch := range channels {
		if ch.Status != service.StatusActive {
			continue
		}
		userGroups := filterUserVisibleGroups(ch.Groups, visibleGroups)
		if len(userGroups) == 0 {
			continue
		}
		sections := buildPlatformSections(ch.SupportedModels, userGroups)
		if len(sections) == 0 {
			continue
		}
		out = append(out, userAvailableChannel{
			Name:        ch.Name,
			Description: ch.Description,
			Platforms:   sections,
		})
	}

	response.Success(c, out)
}

// filterUserVisibleGroups keeps only the groups whose id is in `visible`.
func filterUserVisibleGroups(
	groups []service.AvailableGroupRef,
	visible map[int64]struct{},
) []userAvailableGroup {
	out := make([]userAvailableGroup, 0, len(groups))
	for _, g := range groups {
		if _, ok := visible[g.ID]; !ok {
			continue
		}
		out = append(out, userAvailableGroup{
			ID:               g.ID,
			Name:             g.Name,
			Platform:         g.Platform,
			SubscriptionType: g.SubscriptionType,
			RateMultiplier:   g.RateMultiplier,
			IsExclusive:      g.IsExclusive,
		})
	}
	return out
}

// buildPlatformSections splits a channel's models / groups by platform so
// the frontend can render a row-group per (channel, platform) tuple.
// Platforms are sorted alphabetically for stable rendering.
func buildPlatformSections(
	supportedModels []domain.SupportedModel,
	visibleGroups []userAvailableGroup,
) []userChannelPlatformSection {
	groupsByPlatform := make(map[string][]userAvailableGroup, 4)
	for _, g := range visibleGroups {
		if g.Platform == "" {
			continue
		}
		groupsByPlatform[g.Platform] = append(groupsByPlatform[g.Platform], g)
	}
	if len(groupsByPlatform) == 0 {
		return nil
	}

	platforms := make([]string, 0, len(groupsByPlatform))
	for p := range groupsByPlatform {
		platforms = append(platforms, p)
	}
	sort.Strings(platforms)

	sections := make([]userChannelPlatformSection, 0, len(platforms))
	for _, platform := range platforms {
		platformSet := map[string]struct{}{platform: {}}
		sections = append(sections, userChannelPlatformSection{
			Platform:        platform,
			Groups:          groupsByPlatform[platform],
			SupportedModels: toUserSupportedModels(supportedModels, platformSet),
		})
	}
	return sections
}

// toUserSupportedModels projects domain.SupportedModel into the user DTO
// slice, filtering by the allowed platforms.
func toUserSupportedModels(
	src []domain.SupportedModel,
	allowedPlatforms map[string]struct{},
) []userSupportedModel {
	out := make([]userSupportedModel, 0, len(src))
	for i := range src {
		m := src[i]
		if _, ok := allowedPlatforms[m.Platform]; !ok {
			continue
		}
		out = append(out, userSupportedModel{
			Name:     m.Name,
			Platform: m.Platform,
			Pricing:  toUserPricing(m.Pricing),
		})
	}
	return out
}

// toUserPricing projects domain.ChannelModelPricing into the pricing DTO,
// returning nil when the source is nil (unconfigured pricing).
func toUserPricing(p *domain.ChannelModelPricing) *userSupportedModelPricing {
	if p == nil {
		return nil
	}
	intervals := make([]userPricingIntervalDTO, 0, len(p.Intervals))
	for _, iv := range p.Intervals {
		intervals = append(intervals, userPricingIntervalDTO{
			MinTokens:       iv.MinTokens,
			MaxTokens:       iv.MaxTokens,
			TierLabel:       iv.TierLabel,
			InputPrice:      iv.InputPrice,
			OutputPrice:     iv.OutputPrice,
			CacheWritePrice: iv.CacheWritePrice,
			CacheReadPrice:  iv.CacheReadPrice,
			PerRequestPrice: iv.PerRequestPrice,
		})
	}
	billingMode := p.BillingMode
	if billingMode == "" {
		billingMode = string(service.BillingModeToken)
	}
	return &userSupportedModelPricing{
		BillingMode:      billingMode,
		InputPrice:       p.InputPrice,
		OutputPrice:      p.OutputPrice,
		CacheWritePrice:  p.CacheWritePrice,
		CacheReadPrice:   p.CacheReadPrice,
		ImageOutputPrice: p.ImageOutputPrice,
		PerRequestPrice:  p.PerRequestPrice,
		Intervals:        intervals,
	}
}
