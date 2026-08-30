package service

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"sort"

	"github.com/Wei-Shaw/sub2api/internal/domain"
)

// ErrSmartRoutingNoEligibleGroup 表示智能路由开启后，没有任何候选分组能服务所请求的模型。
var ErrSmartRoutingNoEligibleGroup = errors.New("smart routing: no eligible group can serve the requested model")

// SmartRoutingEligibilityFilter 让上层注入模型支持判定，便于复用已有的可用模型列表逻辑。
type SmartRoutingEligibilityFilter func(ctx context.Context, group *Group, model string) bool

// ResolveSmartRoutingCandidates 为启用了智能路由的 API Key 解析本次请求的候选分组序列。
//
// 返回的候选序列语义（分组级 failover 的顺序依据）：
//  1. 候选分组 = 用户可用分组（ListActive + 订阅校验 + CanBindGroup） - 排除分组；
//  2. 过滤出能服务请求模型的候选（使用 GetAvailableModels，无显式 model_mapping 的分组视为可服务任意模型）；
//  3. 按优先级（config.Priorities，高者优先）分组排序；
//  4. 同一优先级层内按权重（config.Weights，缺省 1）做加权随机排列——
//     即每次请求在同优先级内按权重随机决定尝试顺序，权重越大越靠前被尝试；
//  5. 返回扁平化后的完整候选序列：调用方按序尝试，前一个分组失败（failover 耗尽）
//     后自动降级到下一个分组，实现「优先级 = 故障转移顺序」。
//
// 未启用智能路由时返回 (nil, nil)，调用方应沿用 apiKey.GroupID。
func (s *GatewayService) ResolveSmartRoutingCandidates(ctx context.Context, apiKey *APIKey, requestedModel string) ([]*Group, error) {
	if apiKey == nil || !apiKey.SmartRoutingEnabled {
		return nil, nil
	}
	cfg := apiKey.SmartRoutingConfig
	if cfg == nil {
		cfg = &domain.SmartRoutingConfig{}
	}

	excluded := make(map[int64]struct{}, len(cfg.ExcludeGroupIDs))
	for _, gid := range cfg.ExcludeGroupIDs {
		excluded[gid] = struct{}{}
	}

	// 候选分组：复用 GetAvailableGroups 的权限语义。
	user, err := s.userRepo.GetByID(ctx, apiKey.UserID)
	if err != nil {
		return nil, fmt.Errorf("smart routing: get user: %w", err)
	}
	allGroups, err := s.groupRepo.ListActive(ctx)
	if err != nil {
		return nil, fmt.Errorf("smart routing: list active groups: %w", err)
	}
	activeSubs, err := s.userSubRepo.ListActiveByUserID(ctx, apiKey.UserID)
	if err != nil {
		return nil, fmt.Errorf("smart routing: list subscriptions: %w", err)
	}
	subscribedGroupIDs := make(map[int64]bool, len(activeSubs))
	for _, sub := range activeSubs {
		subscribedGroupIDs[sub.GroupID] = true
	}

	var candidates []*Group
	for i := range allGroups {
		g := &allGroups[i]
		if _, skip := excluded[g.ID]; skip {
			continue
		}
		if !s.canUserBindSmartRoutingGroup(user, g, subscribedGroupIDs) {
			continue
		}
		candidates = append(candidates, g)
	}

	// 过滤出能服务该模型的分组。
	var eligible []*Group
	for _, g := range candidates {
		if s.smartRoutingGroupCanServeModel(ctx, g, requestedModel) {
			eligible = append(eligible, g)
		}
	}
	if len(eligible) == 0 {
		return nil, ErrSmartRoutingNoEligibleGroup
	}

	// 按优先级降序排序（稳定），同优先级保持相对顺序。
	sort.SliceStable(eligible, func(i, j int) bool {
		return smartRoutingPriority(cfg, eligible[i].ID) > smartRoutingPriority(cfg, eligible[j].ID)
	})

	// 同优先级层内按权重加权随机排列；不同优先级层按优先级降序拼接。
	var ordered []*Group
	start := 0
	for start < len(eligible) {
		end := start + 1
		prio := smartRoutingPriority(cfg, eligible[start].ID)
		for end < len(eligible) && smartRoutingPriority(cfg, eligible[end].ID) == prio {
			end++
		}
		tier := eligible[start:end]
		ordered = append(ordered, weightedShuffleSmartRoutingTier(tier, cfg)...)
		start = end
	}
	return ordered, nil
}

// ResolveSmartRoutingGroup 为启用了智能路由的 API Key 选择本次请求首选生效的分组。
// 等价于 ResolveSmartRoutingCandidates 序列的第一个候选（权重最高的优先级层内加权随机首个）。
// 未启用智能路由时返回 (nil, nil)。
func (s *GatewayService) ResolveSmartRoutingGroup(ctx context.Context, apiKey *APIKey, requestedModel string) (*Group, error) {
	candidates, err := s.ResolveSmartRoutingCandidates(ctx, apiKey, requestedModel)
	if err != nil {
		return nil, err
	}
	if len(candidates) == 0 {
		return nil, nil
	}
	return candidates[0], nil
}

// canUserBindSmartRoutingGroup 与 GetAvailableGroups 的权限过滤保持一致。
func (s *GatewayService) canUserBindSmartRoutingGroup(user *User, group *Group, subscribedGroupIDs map[int64]bool) bool {
	if group == nil || user == nil {
		return false
	}
	if group.IsSubscriptionType() {
		return subscribedGroupIDs[group.ID]
	}
	return user.CanBindGroup(group.ID, group.IsExclusive)
}

// smartRoutingGroupCanServeModel 判定分组能否服务请求模型。
// GetAvailableModels 返回 nil 表示该分组没有任何账号配置显式 model_mapping（默认模型集），
// 此时视为可服务任意模型；否则要求请求模型命中可用模型列表。
func (s *GatewayService) smartRoutingGroupCanServeModel(ctx context.Context, group *Group, model string) bool {
	if group == nil {
		return false
	}
	models := s.GetAvailableModels(ctx, &group.ID, "")
	if len(models) == 0 {
		return true
	}
	for _, m := range models {
		if m == model {
			return true
		}
	}
	return false
}

func smartRoutingPriority(cfg *domain.SmartRoutingConfig, groupID int64) int {
	if cfg == nil || cfg.Priorities == nil {
		return 0
	}
	return cfg.Priorities[groupID]
}

func smartRoutingWeight(cfg *domain.SmartRoutingConfig, groupID int64) int {
	if cfg == nil || cfg.Weights == nil {
		return 1
	}
	if v, ok := cfg.Weights[groupID]; ok {
		if v < 0 {
			return 0
		}
		return v
	}
	return 1
}

// weightedShuffleSmartRoutingTier 对同一优先级层内的分组做权重加权随机排列。
// 权重越大越靠前（越先被尝试），实现「同优先级下多个请求按权重分配」。
func weightedShuffleSmartRoutingTier(groups []*Group, cfg *domain.SmartRoutingConfig) []*Group {
	if len(groups) <= 1 {
		return groups
	}
	// 权重随机轮盘：每轮按权重比例抽取一个分组，抽出后从剩余集合中移除，
	// 直到所有分组排定顺序。权重为 0 的分组排最后（权重 0 表示不参与分配，但
	// 仍作为 failover 后备保留在序列末尾）。
	remaining := make([]*Group, len(groups))
	copy(remaining, groups)
	ordered := make([]*Group, 0, len(groups))

	for len(remaining) > 0 {
		total := 0
		for _, g := range remaining {
			total += smartRoutingWeight(cfg, g.ID)
		}
		if total <= 0 {
			// 全部权重为 0：退化为均匀随机排列。
			perm := rand.Perm(len(remaining))
			for _, idx := range perm {
				ordered = append(ordered, remaining[idx])
			}
			break
		}
		r := rand.Intn(total)
		idx := 0
		for i, g := range remaining {
			w := smartRoutingWeight(cfg, g.ID)
			r -= w
			if r < 0 {
				idx = i
				break
			}
		}
		ordered = append(ordered, remaining[idx])
		remaining = append(remaining[:idx], remaining[idx+1:]...)
	}
	return ordered
}

// weightedRandomSmartRoutingGroup 在同一优先级层内按权重随机选一个分组（旧接口，保留兼容）。
func weightedRandomSmartRoutingGroup(groups []*Group, cfg *domain.SmartRoutingConfig) *Group {
	if len(groups) == 1 {
		return groups[0]
	}
	shuffled := weightedShuffleSmartRoutingTier(groups, cfg)
	return shuffled[0]
}
