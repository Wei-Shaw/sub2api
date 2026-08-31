package service

import (
	"context"
	"errors"
	mathrand "math/rand"
	"sort"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
)

// SmartRoutingCandidate 是智能路由调度计划中的一个候选成员分组。
// 计划已按「优先级分层 + 层内加权随机排列」排好顺序：调用方按序尝试，
// 首个成功的候选即为实际服务分组；失败的候选触发向下一个候选回退。
type SmartRoutingCandidate struct {
	Group    *Group
	Priority int
	Weight   int
}

// SmartRoutingMemberIDs 返回成员分组 ID（去重、保持配置顺序）。
func smartRoutingMemberIDs(members []domain.SmartRoutingMember) []int64 {
	if len(members) == 0 {
		return nil
	}
	seen := make(map[int64]struct{}, len(members))
	ids := make([]int64, 0, len(members))
	for _, m := range members {
		if m.GroupID <= 0 {
			continue
		}
		if _, dup := seen[m.GroupID]; dup {
			continue
		}
		seen[m.GroupID] = struct{}{}
		ids = append(ids, m.GroupID)
	}
	return ids
}

// loadSmartRoutingMemberGroups 按成员配置逐个加载分组。
// 使用 GetByIDLite（主键单行查询），并保留配置中的优先级/权重元数据。
// 不存在或已删除的成员返回的 map 中缺失，调用方负责跳过。
func (s *GatewayService) loadSmartRoutingMemberGroups(ctx context.Context, members []domain.SmartRoutingMember) (map[int64]*Group, error) {
	ids := smartRoutingMemberIDs(members)
	if len(ids) == 0 {
		return map[int64]*Group{}, nil
	}
	out := make(map[int64]*Group, len(ids))
	for _, id := range ids {
		g, err := s.groupRepo.GetByIDLite(ctx, id)
		if err != nil {
			if errors.Is(err, ErrGroupNotFound) {
				continue // 成员已被删除：静默跳过
			}
			return nil, err
		}
		if g != nil {
			out[id] = g
		}
	}
	return out, nil
}

// BuildSmartRoutingPlan 依据智能路由分组配置构建有序候选调度计划。
//
// 规则：
//  1. 仅保留存在且状态为 active 的成员；跳过嵌套的智能路由分组（禁止递归）。
//  2. 当请求携带模型名且候选 > 1 时，做尽力而为的模型可用性预过滤，
//     使权重分流只作用于真正可能提供该模型的成员；过滤后为空则回退到全量候选。
//  3. 按优先级升序分层（数值越小优先级越高），层内按权重做加权随机排列。
//
// 返回 nil 表示该分组不是有效的智能路由分组或没有可用成员。
func (s *GatewayService) BuildSmartRoutingPlan(ctx context.Context, smartGroup *Group, requestedModel string) ([]SmartRoutingCandidate, error) {
	if s == nil || smartGroup == nil || smartGroup.Platform != PlatformSmartRouting {
		return nil, nil
	}
	members := domain.NormalizeSmartRoutingMembers(smartGroup.SmartRoutingMembers)
	if len(members) == 0 {
		return nil, nil
	}

	byID, err := s.loadSmartRoutingMemberGroups(ctx, members)
	if err != nil {
		return nil, err
	}

	cands := make([]SmartRoutingCandidate, 0, len(members))
	for _, m := range members {
		g := byID[m.GroupID]
		if g == nil {
			continue // 成员不存在/已删除
		}
		if !g.IsActive() {
			continue // 成员被停用
		}
		if g.Platform == PlatformSmartRouting {
			continue // 禁止嵌套，防止递归调度
		}
		cands = append(cands, SmartRoutingCandidate{Group: g, Priority: m.Priority, Weight: m.Weight})
	}
	if len(cands) == 0 {
		return nil, nil
	}

	// 模型可用性预过滤（尽力而为）。仅在多于一个候选时过滤：单候选时过滤
	// 无意义（过滤掉就完全没有候选了），交给实际调度给出准确错误。
	if model := strings.TrimSpace(requestedModel); model != "" && len(cands) > 1 {
		if filtered := s.filterSmartRoutingCandidatesByModel(ctx, cands, model); len(filtered) > 0 {
			cands = filtered
		}
	}

	return orderSmartRoutingCandidates(cands, nil), nil
}

// filterSmartRoutingCandidatesByModel 移除「当前显然无法提供该模型」的成员。
// 判定是乐观的：任何不确定（快照缺失、解析失败等）都保留成员，交给实际调度裁决。
func (s *GatewayService) filterSmartRoutingCandidatesByModel(ctx context.Context, cands []SmartRoutingCandidate, model string) []SmartRoutingCandidate {
	out := make([]SmartRoutingCandidate, 0, len(cands))
	for _, c := range cands {
		if s.smartRoutingMemberMayServeModel(ctx, c.Group, model) {
			out = append(out, c)
		}
	}
	return out
}

// smartRoutingMemberMayServeModel 判断成员分组是否可能提供指定模型。
// 失败开放（返回 true）：只有明确判断「不能」时才返回 false。
func (s *GatewayService) smartRoutingMemberMayServeModel(ctx context.Context, member *Group, model string) bool {
	if member == nil {
		return false
	}
	switch member.Platform {
	case PlatformSmartRouting:
		return false
	case PlatformComposite:
		// Composite 成员依赖模型路由表；解析失败时保留（乐观）。
		if s.compositeResolver == nil {
			return true
		}
		decision, err := s.compositeResolver.Resolve(ctx, member.ID, model, CompositeRouteEndpointAny)
		if err != nil {
			return true
		}
		return decision.Matched
	}

	if s.schedulerSnapshot == nil {
		return true
	}
	platform := member.Platform
	if forced, ok := ctx.Value(ctxkey.ForcePlatform).(string); ok && strings.TrimSpace(forced) != "" {
		platform = strings.TrimSpace(forced)
	}
	hasForce := platform != member.Platform
	groupID := member.ID
	accounts, _, err := s.schedulerSnapshot.ListSchedulableAccounts(ctx, &groupID, platform, hasForce)
	if err != nil {
		return true // 快照不可用：保留
	}
	if len(accounts) == 0 {
		// 明确没有可调度账号：当前无法提供，排除。
		return false
	}
	for i := range accounts {
		if s.isModelSupportedByAccountWithContext(ctx, &accounts[i], model) {
			return true
		}
	}
	return false
}

// orderSmartRoutingCandidates 按优先级升序分层，并在每一层内做加权随机排列。
// rnd 为 nil 时使用全局 math/rand（Go>=1.20 自动播种，并发安全）。
func orderSmartRoutingCandidates(cands []SmartRoutingCandidate, rnd *mathrand.Rand) []SmartRoutingCandidate {
	if len(cands) <= 1 {
		return cands
	}
	sorted := append([]SmartRoutingCandidate(nil), cands...)
	sort.SliceStable(sorted, func(i, j int) bool {
		if sorted[i].Priority != sorted[j].Priority {
			return sorted[i].Priority < sorted[j].Priority
		}
		return sorted[i].Group.ID < sorted[j].Group.ID
	})

	out := make([]SmartRoutingCandidate, 0, len(sorted))
	for start := 0; start < len(sorted); {
		end := start + 1
		for end < len(sorted) && sorted[end].Priority == sorted[start].Priority {
			end++
		}
		out = append(out, weightedShuffleSmartRouting(sorted[start:end], rnd)...)
		start = end
	}
	return out
}

// weightedShuffleSmartRouting 对同一优先级层内的候选做加权随机排列。
// 采用「按剩余权重比例逐个抽取」的方式：首个候选按权重占比选出，
// 其余候选作为失败回退顺序，同样遵循权重分布。
func weightedShuffleSmartRouting(items []SmartRoutingCandidate, rnd *mathrand.Rand) []SmartRoutingCandidate {
	remaining := append([]SmartRoutingCandidate(nil), items...)
	out := make([]SmartRoutingCandidate, 0, len(remaining))
	for len(remaining) > 0 {
		total := 0
		for i := range remaining {
			if remaining[i].Weight > 0 {
				total += remaining[i].Weight
			}
		}
		if total <= 0 {
			// 全部权重非正：按稳定顺序追加，避免死循环。
			out = append(out, remaining...)
			break
		}
		pick := smartRoutingIntn(rnd, total)
		idx := 0
		for i := range remaining {
			w := remaining[i].Weight
			if w <= 0 {
				continue
			}
			if pick < w {
				idx = i
				break
			}
			pick -= w
		}
		out = append(out, remaining[idx])
		remaining = append(remaining[:idx], remaining[idx+1:]...)
	}
	return out
}

func smartRoutingIntn(rnd *mathrand.Rand, n int) int {
	if n <= 0 {
		return 0
	}
	if rnd != nil {
		return rnd.Intn(n)
	}
	return mathrand.Intn(n)
}
