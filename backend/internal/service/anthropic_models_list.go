package service

import (
	"context"
	"slices"
	"strings"
)

func (s *GatewayService) ExpandAnthropicLongContextModels(ctx context.Context, group *Group, models []string) []string {
	var accounts []Account
	var err error
	if group != nil {
		accounts, err = s.accountRepo.ListSchedulableByGroupID(ctx, group.ID)
	} else {
		accounts, err = s.accountRepo.ListSchedulable(ctx)
	}
	if err != nil {
		return models
	}
	result := cloneStringSlice(models)
	seen := make(map[string]bool, len(models))
	for _, model := range models {
		seen[strings.ToLower(model)] = true
	}
	for _, model := range models {
		if strings.HasSuffix(strings.ToLower(model), claudeCodeLongContextModelSuffix) || strings.Contains(model, "*") {
			continue
		}
		var routedAccountIDs []int64
		if group != nil {
			routedAccountIDs = group.GetRoutingAccountIDs(model)
		}
		for _, account := range accounts {
			if !account.IsAnthropic() || !account.IsModelSupported(model) ||
				(len(routedAccountIDs) > 0 && !slices.Contains(routedAccountIDs, account.ID)) {
				continue
			}
			metadata, ok := account.GetUpstreamModelMetadata(account.GetMappedModel(model))
			variant := model + claudeCodeLongContextModelSuffix
			if ok && metadata.ContextWindow >= 900_000 && !seen[strings.ToLower(variant)] {
				result = append(result, variant)
				seen[strings.ToLower(variant)] = true
			}
		}
	}
	return result
}
