package service

import (
	"context"
	"strings"
)

// GetCompositeCodexRouteModelIDs 补充合成路由公开的可调用别名。
// 前缀路由只展开白名单中的具体 ID，不能把无限模式当成模型名展示。
func (s *GatewayService) GetCompositeCodexRouteModelIDs(ctx context.Context, group *Group) []string {
	if s == nil || group == nil || group.Platform != PlatformComposite || s.accountRepo == nil || s.compositeResolver == nil || s.compositeResolver.repo == nil {
		return nil
	}
	routes, err := s.compositeResolver.repo.ListByGroup(ctx, group.ID, false)
	if err != nil || len(routes) == 0 {
		return nil
	}
	accounts, err := s.accountRepo.ListSchedulableByGroupID(ctx, group.ID)
	if err != nil {
		return nil
	}
	candidates := make([]string, 0, len(routes))
	for _, route := range routes {
		if normalizeCompositeRouteMatchType(route.MatchType) == CompositeRouteMatchExact {
			candidates = append(candidates, route.PublicModel)
		}
	}
	if group.ModelAllowlistEnabled() {
		candidates = append(candidates, group.ModelAllowlist.Models...)
	}
	result := make([]string, 0)
	for _, model := range dedupeAndSortModelIDs(candidates) {
		if strings.Contains(model, "*") || isCodexDedicatedMediaModel(model) {
			continue
		}
		platform, target, ok := resolveCodexCompositeModelTarget(model, accounts, routes, true)
		if !ok || isCodexDedicatedMediaModel(target) {
			continue
		}
		for i := range accounts {
			account := &accounts[i]
			if account.Platform == platform && account.IsActive() && account.Schedulable && account.IsModelSupported(target) {
				result = append(result, model)
				break
			}
		}
	}
	return result
}
