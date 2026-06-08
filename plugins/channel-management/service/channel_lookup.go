package service

import (
	"context"
	"log/slog"
	"strings"
)

// ChannelMappingResult 渠道映射查找结果
type ChannelMappingResult struct {
	MappedModel        string // 映射后的模型名（无映射时等于原始模型名）
	ChannelID          int64  // 渠道 ID（0 = 无渠道关联）
	Mapped             bool   // 是否发生了映射
	BillingModelSource string // 计费模型来源（"requested" / "upstream" / "channel_mapped"）
}

// BuildModelMappingChain renders a "a→b→c" string describing the model
// transformations applied. Returns "" when no mapping happened.
func (r ChannelMappingResult) BuildModelMappingChain(reqModel, upstreamModel string) string {
	if !r.Mapped {
		if upstreamModel != "" && upstreamModel != reqModel {
			return reqModel + "→" + upstreamModel
		}
		return ""
	}
	if upstreamModel != "" && upstreamModel != r.MappedModel {
		return reqModel + "→" + r.MappedModel + "→" + upstreamModel
	}
	return reqModel + "→" + r.MappedModel
}

// ToUsageFields converts the mapping result into the fields downstream
// usage-recording code expects.
func (r ChannelMappingResult) ToUsageFields(reqModel, upstreamModel string) ChannelUsageFields {
	channelMappedModel := reqModel
	if r.Mapped {
		channelMappedModel = r.MappedModel
	}
	return ChannelUsageFields{
		ChannelID:          r.ChannelID,
		OriginalModel:      reqModel,
		ChannelMappedModel: channelMappedModel,
		BillingModelSource: r.BillingModelSource,
		ModelMappingChain:  r.BuildModelMappingChain(reqModel, upstreamModel),
	}
}

// GetChannelForGroup returns a clone of the active channel attached to the
// group, or nil when none is configured.
func (s *ChannelService) GetChannelForGroup(ctx context.Context, groupID int64) (*Channel, error) {
	cache, err := s.loadCache(ctx)
	if err != nil {
		return nil, err
	}

	ch, ok := cache.channelByGroupID[groupID]
	if !ok || !ch.IsActive() {
		return nil, nil
	}
	return ch.Clone(), nil
}

type channelLookup struct {
	cache    *channelCache
	channel  *Channel
	platform string
}

// lookupGroupChannel is the shared hot-path setup: load the cache, locate the
// active channel, and return the snapshot + platform metadata for downstream
// pricing/mapping calls.
func (s *ChannelService) lookupGroupChannel(ctx context.Context, groupID int64) (*channelLookup, error) {
	cache, err := s.loadCache(ctx)
	if err != nil {
		return nil, err
	}
	ch, ok := cache.channelByGroupID[groupID]
	if !ok || !ch.IsActive() {
		return nil, nil
	}
	return &channelLookup{
		cache:    cache,
		channel:  ch,
		platform: cache.groupPlatform[groupID],
	}, nil
}

// GetChannelModelPricing returns a clone of the pricing row matching
// (groupID, model). Lookup is O(1) via the cache.
func (s *ChannelService) GetChannelModelPricing(ctx context.Context, groupID int64, model string) *ChannelModelPricing {
	lk, err := s.lookupGroupChannel(ctx, groupID)
	if err != nil {
		slog.Warn("failed to load channel cache", "group_id", groupID, "error", err)
		return nil
	}
	if lk == nil {
		return nil
	}

	modelLower := strings.ToLower(model)
	pricing := lookupPricingAcrossPlatforms(lk.cache, groupID, lk.platform, modelLower)
	if pricing == nil {
		return nil
	}
	cp := pricing.Clone()
	return &cp
}

// ResolveChannelMapping resolves the channel-level model mapping for
// (groupID, model). Returns the original model wrapped in a default result
// when no channel/mapping exists.
func (s *ChannelService) ResolveChannelMapping(ctx context.Context, groupID int64, model string) ChannelMappingResult {
	lk, err := s.lookupGroupChannel(ctx, groupID)
	if err != nil {
		slog.Warn("failed to load channel cache for mapping", "group_id", groupID, "error", err)
	}
	if lk == nil {
		return ChannelMappingResult{MappedModel: model}
	}
	return resolveMapping(lk, groupID, model)
}

// IsModelRestricted reports whether model is forbidden by the channel's
// restriction policy. Returns false when the channel does not enable
// restriction or the group has no associated channel.
func (s *ChannelService) IsModelRestricted(ctx context.Context, groupID int64, model string) bool {
	lk, err := s.lookupGroupChannel(ctx, groupID)
	if err != nil {
		slog.Warn("failed to load channel cache for model restriction check", "group_id", groupID, "error", err)
	}
	if lk == nil {
		return false
	}
	return checkRestricted(lk, groupID, model)
}

// ResolveChannelMappingAndRestrict mirrors ResolveChannelMapping but accepts a
// nullable groupID. Restriction checking has moved to the scheduler; the
// second return value is always false but kept for signature compatibility.
func (s *ChannelService) ResolveChannelMappingAndRestrict(ctx context.Context, groupID *int64, model string) (ChannelMappingResult, bool) {
	if groupID == nil {
		return ChannelMappingResult{MappedModel: model}, false
	}
	lk, _ := s.lookupGroupChannel(ctx, *groupID)
	if lk == nil {
		return ChannelMappingResult{MappedModel: model}, false
	}
	return resolveMapping(lk, *groupID, model), false
}

func resolveMapping(lk *channelLookup, groupID int64, model string) ChannelMappingResult {
	result := ChannelMappingResult{
		MappedModel:        model,
		ChannelID:          lk.channel.ID,
		BillingModelSource: lk.channel.BillingModelSource,
	}
	if result.BillingModelSource == "" {
		result.BillingModelSource = BillingModelSourceChannelMapped
	}

	modelLower := strings.ToLower(model)
	if mapped := lookupMappingAcrossPlatforms(lk.cache, groupID, lk.platform, modelLower); mapped != "" {
		result.MappedModel = mapped
		result.Mapped = true
	}
	return result
}

func checkRestricted(lk *channelLookup, groupID int64, model string) bool {
	if !lk.channel.RestrictModels {
		return false
	}
	modelLower := strings.ToLower(model)
	return lookupPricingAcrossPlatforms(lk.cache, groupID, lk.platform, modelLower) == nil
}
