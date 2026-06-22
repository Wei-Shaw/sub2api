package service

import (
	"context"
	"fmt"
	"sort"
	"strings"
)

// AvailableGroupRef 渠道视图中关联分组的简要信息。
//
// 用户侧「可用渠道」页面据此展示：专属分组 vs 公开分组（IsExclusive）、
// 订阅 vs 标准（SubscriptionType）、默认倍率（RateMultiplier）。用户专属倍率
// 不在这里暴露，前端自己通过 /groups/rates 拉取，和 API 密钥页面保持一致。
type AvailableGroupRef struct {
	ID               int64
	Name             string
	Platform         string
	SubscriptionType string
	RateMultiplier   float64
	IsExclusive      bool
	ImagePrice1K     *float64
	ImagePrice2K     *float64
	ImagePrice4K     *float64
}

// AvailableChannel 可用渠道视图：用于「可用渠道」页面展示渠道基础信息 +
// 关联的分组 + 推导出的支持模型列表（无通配符）。
type AvailableChannel struct {
	ID                 int64
	Name               string
	Description        string
	Status             string
	BillingModelSource string
	RestrictModels     bool
	Groups             []AvailableGroupRef
	SupportedModels    []SupportedModel
}

type channelAccountMappingModelLister interface {
	ListAccountMappingModelsByGroupIDs(ctx context.Context, groupIDs []int64) (map[int64][]string, error)
}

// ListAvailable 返回所有渠道的可用视图：每个渠道附带关联分组信息与支持模型列表。
//
// 支持模型通过 (*Channel).SupportedModels() 计算（mapping ∪ pricing 并联）。
// 对于渠道未配置定价的模型，进一步用 PricingService 的全局 LiteLLM 数据合成
// 一份展示用定价，让用户看到默认价格而非"未配置"。
//
// 关联分组信息通过 groupRepo.ListActive 查询后按 ID 映射；渠道 GroupIDs 中未在活跃列表中
// 的分组（已停用或删除）会被忽略。
//
// 前置条件：s.groupRepo 必须非 nil（由 wire DI 保证）。直接 nil-deref 用于 fail-fast，
// 避免静默掩盖注入缺失。
func (s *ChannelService) ListAvailable(ctx context.Context) ([]AvailableChannel, error) {
	channels, err := s.repo.ListAll(ctx)
	if err != nil {
		return nil, fmt.Errorf("list channels: %w", err)
	}

	groups, err := s.groupRepo.ListActive(ctx)
	if err != nil {
		return nil, fmt.Errorf("list active groups: %w", err)
	}
	groupByID := make(map[int64]AvailableGroupRef, len(groups))
	groupModelsListByID := make(map[int64]GroupModelsListConfig, len(groups))
	for i := range groups {
		g := groups[i]
		groupByID[g.ID] = AvailableGroupRef{
			ID:               g.ID,
			Name:             g.Name,
			Platform:         g.Platform,
			SubscriptionType: g.SubscriptionType,
			RateMultiplier:   g.RateMultiplier,
			IsExclusive:      g.IsExclusive,
			ImagePrice1K:     g.ImagePrice1K,
			ImagePrice2K:     g.ImagePrice2K,
			ImagePrice4K:     g.ImagePrice4K,
		}
		groupModelsListByID[g.ID] = g.ModelsListConfig
	}

	accountMappedModelsByGroupID := map[int64][]string{}
	if lister, ok := s.repo.(channelAccountMappingModelLister); ok {
		groupIDs := collectActiveGroupIDs(groupByID)
		if len(groupIDs) > 0 {
			accountMappedModelsByGroupID, err = lister.ListAccountMappingModelsByGroupIDs(ctx, groupIDs)
			if err != nil {
				return nil, fmt.Errorf("list account mapping models: %w", err)
			}
		}
	}

	out := make([]AvailableChannel, 0, len(channels))
	channelLinkedGroupIDs := make(map[int64]struct{}, len(groupByID))
	for i := range channels {
		ch := &channels[i]
		groups := make([]AvailableGroupRef, 0, len(ch.GroupIDs))
		for _, gid := range ch.GroupIDs {
			if ref, ok := groupByID[gid]; ok {
				channelLinkedGroupIDs[gid] = struct{}{}
				groups = append(groups, ref)
			}
		}
		sort.SliceStable(groups, func(i, j int) bool { return groups[i].Name < groups[j].Name })

		ch.normalizeBillingModelSource()

		supported := ch.SupportedModels()
		if !ch.RestrictModels {
			supported = appendAccountMappedSupportedModels(supported, groups, accountMappedModelsByGroupID)
		}
		s.fillGlobalPricingFallback(supported)

		out = append(out, AvailableChannel{
			ID:                 ch.ID,
			Name:               ch.Name,
			Description:        ch.Description,
			Status:             ch.Status,
			BillingModelSource: ch.BillingModelSource,
			RestrictModels:     ch.RestrictModels,
			Groups:             groups,
			SupportedModels:    supported,
		})
	}
	out = append(out, s.buildDirectGroupAvailableChannels(groupByID, groupModelsListByID, channelLinkedGroupIDs, accountMappedModelsByGroupID)...)

	sort.SliceStable(out, func(i, j int) bool {
		return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
	})
	return out, nil
}

func collectActiveGroupIDs(groupByID map[int64]AvailableGroupRef) []int64 {
	groupIDs := make([]int64, 0, len(groupByID))
	for id := range groupByID {
		groupIDs = append(groupIDs, id)
	}
	sort.Slice(groupIDs, func(i, j int) bool { return groupIDs[i] < groupIDs[j] })
	return groupIDs
}

func (s *ChannelService) buildDirectGroupAvailableChannels(
	groupByID map[int64]AvailableGroupRef,
	groupModelsListByID map[int64]GroupModelsListConfig,
	channelLinkedGroupIDs map[int64]struct{},
	mappedByGroupID map[int64][]string,
) []AvailableChannel {
	out := make([]AvailableChannel, 0)
	groupIDs := collectActiveGroupIDs(groupByID)
	for _, groupID := range groupIDs {
		if _, linked := channelLinkedGroupIDs[groupID]; linked {
			continue
		}
		group := groupByID[groupID]
		models := directGroupSupportedModels(group, groupModelsListByID[groupID], mappedByGroupID[groupID])
		if len(models) == 0 {
			continue
		}
		s.fillGlobalPricingFallback(models)
		out = append(out, AvailableChannel{
			ID:              -groupID,
			Name:            group.Name,
			Description:     "分组直连",
			Status:          StatusActive,
			RestrictModels:  groupModelsListByID[groupID].Enabled,
			Groups:          []AvailableGroupRef{group},
			SupportedModels: models,
		})
	}
	return out
}

func directGroupSupportedModels(group AvailableGroupRef, cfg GroupModelsListConfig, mappedModels []string) []SupportedModel {
	modelNames := mappedModels
	if cfg.Enabled && len(cfg.Models) > 0 {
		modelNames = cfg.Models
	}
	seen := make(map[string]struct{}, len(modelNames))
	out := make([]SupportedModel, 0, len(modelNames))
	for _, model := range modelNames {
		name := strings.TrimSpace(model)
		if name == "" {
			continue
		}
		key := strings.ToLower(name)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, SupportedModel{Name: name, Platform: group.Platform})
	}
	sort.SliceStable(out, func(i, j int) bool { return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name) })
	return out
}

func appendAccountMappedSupportedModels(supported []SupportedModel, groups []AvailableGroupRef, mappedByGroupID map[int64][]string) []SupportedModel {
	if len(groups) == 0 || len(mappedByGroupID) == 0 {
		return supported
	}
	type modelKey struct {
		platform string
		model    string
	}
	seen := make(map[modelKey]struct{}, len(supported))
	for _, model := range supported {
		seen[modelKey{platform: model.Platform, model: strings.ToLower(model.Name)}] = struct{}{}
	}
	out := append([]SupportedModel(nil), supported...)
	for _, group := range groups {
		models := mappedByGroupID[group.ID]
		for _, model := range models {
			name := strings.TrimSpace(model)
			if name == "" {
				continue
			}
			key := modelKey{platform: group.Platform, model: strings.ToLower(name)}
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			out = append(out, SupportedModel{Name: name, Platform: group.Platform})
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Platform != out[j].Platform {
			return out[i].Platform < out[j].Platform
		}
		return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
	})
	return out
}

// fillGlobalPricingFallback 对未命中渠道定价的支持模型，从全局 LiteLLM 数据合成一份
// 展示用定价。仅用于「可用渠道」展示，不影响真实计费链路。
//
// 触发条件：
//  1. Pricing == nil（渠道完全没声明该模型的定价条目）
//  2. Pricing 非 nil 但所有价格字段为空（admin UI 建了条目但没填价格）
//
// 当 s.pricingService 为 nil（测试场景），跳过回落。
func (s *ChannelService) fillGlobalPricingFallback(models []SupportedModel) {
	if s.pricingService == nil {
		return
	}
	for i := range models {
		if !pricingNeedsFallback(models[i].Pricing) {
			continue
		}
		lp := s.pricingService.GetModelPricing(models[i].Name)
		if lp == nil && isOpenAIImageGenerationModel(models[i].Name) {
			models[i].Pricing = synthesizeDefaultImagePricing(models[i].Pricing)
			continue
		}
		if lp == nil {
			continue
		}
		models[i].Pricing = synthesizePricingFromLiteLLM(lp, models[i].Pricing)
	}
}

// pricingNeedsFallback 判定一个 ChannelModelPricing 是否需要走全局回落。
// 价格全部缺失（无 flat 字段且无任何带价 interval）即视为未配置。
func pricingNeedsFallback(p *ChannelModelPricing) bool {
	if p == nil {
		return true
	}
	if p.InputPrice != nil || p.OutputPrice != nil ||
		p.CacheWritePrice != nil || p.CacheReadPrice != nil ||
		p.ImageOutputPrice != nil || p.PerRequestPrice != nil {
		return false
	}
	for _, iv := range p.Intervals {
		if iv.InputPrice != nil || iv.OutputPrice != nil ||
			iv.CacheWritePrice != nil || iv.CacheReadPrice != nil ||
			iv.PerRequestPrice != nil {
			return false
		}
	}
	return true
}

// synthesizePricingFromLiteLLM 把 LiteLLM 的定价数据转成 ChannelModelPricing 形态，
// 仅用于展示。
//
// 计费模式优先级：
//  1. 渠道已选 BillingMode（admin 在 UI 里选了 image / per_request 但没填价的场景，
//     按选定模式合成对应字段）
//  2. LiteLLM mode="image_generation" → image
//  3. 默认 token
//
// LiteLLM 中字段 0 视为未配置，不带入展示。
func synthesizePricingFromLiteLLM(lp *LiteLLMModelPricing, existing *ChannelModelPricing) *ChannelModelPricing {
	if lp == nil {
		return existing
	}

	mode := BillingModeToken
	switch {
	case existing != nil && existing.BillingMode != "":
		mode = existing.BillingMode
	case lp.Mode == "image_generation":
		mode = BillingModeImage
	}

	if mode == BillingModeImage || mode == BillingModePerRequest {
		perRequestPrice := nonZeroPtr(lp.OutputCostPerImage)
		if mode == BillingModeImage && perRequestPrice == nil {
			perRequestPrice = availableFloat64Ptr(defaultImagePrice1K)
		}
		return &ChannelModelPricing{
			BillingMode:      mode,
			PerRequestPrice:  perRequestPrice,
			ImageOutputPrice: nonZeroPtr(lp.OutputCostPerImageToken),
			InputPrice:       nonZeroPtr(lp.InputCostPerToken),
			OutputPrice:      nonZeroPtr(lp.OutputCostPerToken),
			Intervals:        synthesizeImagePriceTiers(mode, perRequestPrice),
		}
	}
	return &ChannelModelPricing{
		BillingMode:      mode,
		InputPrice:       nonZeroPtr(lp.InputCostPerToken),
		OutputPrice:      nonZeroPtr(lp.OutputCostPerToken),
		CacheWritePrice:  nonZeroPtr(lp.CacheCreationInputTokenCost),
		CacheReadPrice:   nonZeroPtr(lp.CacheReadInputTokenCost),
		ImageOutputPrice: nonZeroPtr(lp.OutputCostPerImageToken),
	}
}

const defaultImagePrice1K = 0.134

func synthesizeDefaultImagePricing(existing *ChannelModelPricing) *ChannelModelPricing {
	mode := BillingModeImage
	if existing != nil && existing.BillingMode != "" {
		mode = existing.BillingMode
	}
	base := availableFloat64Ptr(defaultImagePrice1K)
	return &ChannelModelPricing{
		BillingMode:     mode,
		PerRequestPrice: base,
		Intervals:       synthesizeImagePriceTiers(mode, base),
	}
}

func nonZeroPtr(v float64) *float64 {
	if v == 0 {
		return nil
	}
	return &v
}

func availableFloat64Ptr(v float64) *float64 {
	return &v
}

func synthesizeImagePriceTiers(mode BillingMode, base *float64) []PricingInterval {
	if mode != BillingModeImage || base == nil {
		return nil
	}
	price1K := *base
	price2K := price1K * 1.5
	price4K := price1K * 2
	return []PricingInterval{
		{TierLabel: "1K", PerRequestPrice: &price1K, SortOrder: 1},
		{TierLabel: "2K", PerRequestPrice: &price2K, SortOrder: 2},
		{TierLabel: "4K", PerRequestPrice: &price4K, SortOrder: 3},
	}
}
