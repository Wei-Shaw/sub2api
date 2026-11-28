package service

import (
	"context"
	"fmt"
	"sort"
	"strings"
)

// KeyGroupModel describes request aliases for one actual upstream model. It
// deliberately contains no account identifiers or upstream credentials.
type KeyGroupModel struct {
	ID            string   `json:"id"`
	UpstreamModel string   `json:"upstream_model"`
	Aliases       []string `json:"aliases"`
}

// GetKeyGroupModels reads the group's current account mappings directly. Model
// discovery must not retain a cached catalogue after an operator changes a key's
// group, its allowlist, or the accounts serving that group.
func (s *GatewayService) GetKeyGroupModels(ctx context.Context, group *Group) ([]KeyGroupModel, error) {
	if s == nil || s.accountRepo == nil || group == nil || group.ID <= 0 {
		return nil, fmt.Errorf("key group model configuration is unavailable")
	}
	accounts, err := s.accountRepo.ListSchedulableByGroupID(ctx, group.ID)
	if err != nil {
		return nil, fmt.Errorf("read key group models: %w", err)
	}
	return keyGroupModelsFromAccounts(group, accounts), nil
}

func keyGroupModelsFromAccounts(group *Group, accounts []Account) []KeyGroupModel {
	byTarget := make(map[string]map[string]struct{})
	for i := range accounts {
		account := &accounts[i]
		if account.Status != StatusActive {
			continue
		}
		if group.Platform != PlatformComposite && account.Platform != group.Platform && !mixedListingAccountAllowed(group.Platform, account) {
			continue
		}
		candidates := make(map[string]struct{})
		for alias, target := range account.GetModelMapping() {
			candidates[alias] = struct{}{}
			// A mapping target may also be a valid identity request (including
			// a concrete model covered by a wildcard source). Validate it below.
			candidates[target] = struct{}{}
		}
		// A concrete group allowlist also identifies models for unmapped or
		// wildcard-mapped accounts. Wildcards themselves are not request IDs.
		if group.ModelAllowlistEnabled() {
			for _, alias := range group.ModelAllowlist.Models {
				candidates[alias] = struct{}{}
			}
		}
		for candidate := range candidates {
			alias := strings.TrimSpace(candidate)
			if alias == "" || strings.Contains(alias, "*") || !group.ModelAllowlist.Allows(alias) || !account.IsModelSupported(alias) {
				continue
			}
			target := strings.TrimSpace(account.GetMappedModel(alias))
			// OpenAI passthrough forwards the original model and ignores mapping
			// targets, even if an account retains an old model_mapping.
			if account.IsOpenAIPassthroughEnabled() {
				target = alias
			}
			if target == "" || strings.Contains(target, "*") {
				continue
			}
			if byTarget[target] == nil {
				byTarget[target] = make(map[string]struct{})
			}
			byTarget[target][alias] = struct{}{}
		}
	}

	models := make([]KeyGroupModel, 0, len(byTarget))
	for target, aliasesSet := range byTarget {
		aliases := make([]string, 0, len(aliasesSet))
		for alias := range aliasesSet {
			aliases = append(aliases, alias)
		}
		sort.Strings(aliases)
		// The identity alias is the canonical request ID when it is allowed.
		// Otherwise all aliases are equivalent, so use a stable request ID;
		// clients can preserve any of the returned aliases they already use.
		id := aliases[0]
		if _, ok := aliasesSet[target]; ok {
			id = target
		}
		models = append(models, KeyGroupModel{ID: id, UpstreamModel: target, Aliases: aliases})
	}
	sort.Slice(models, func(i, j int) bool { return models[i].UpstreamModel < models[j].UpstreamModel })
	return models
}
