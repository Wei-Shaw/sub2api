package service

// openAIPoolChannelPriorityTier returns only the highest-priority candidates
// when at least one eligible account uses OpenAI API-key pool mode. Keeping
// this as a strict tier prevents load-based scoring from selecting a lower
// priority pool account while a higher-priority account remains eligible.
func openAIPoolChannelPriorityTier(candidates []*Account) ([]*Account, bool) {
	if len(candidates) == 0 {
		return nil, false
	}

	minPriority := 0
	found := false
	hasPoolAccount := false
	for _, account := range candidates {
		if account == nil {
			continue
		}
		if account.IsPoolMode() {
			hasPoolAccount = true
		}
		if !found || account.Priority < minPriority {
			minPriority = account.Priority
			found = true
		}
	}
	if !found || !hasPoolAccount {
		return nil, false
	}

	tier := make([]*Account, 0, len(candidates))
	for _, account := range candidates {
		if account != nil && account.Priority == minPriority {
			tier = append(tier, account)
		}
	}
	return tier, len(tier) > 0
}
