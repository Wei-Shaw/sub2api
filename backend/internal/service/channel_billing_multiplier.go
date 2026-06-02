package service

import "context"

// ResolveBillingMultiplier returns the first channel billing multiplier rule
// matching the selected group/account. A missing rule is neutral (1x).
func (s *ChannelService) ResolveBillingMultiplier(ctx context.Context, groupID int64, accountID int64) float64 {
	if s == nil || groupID <= 0 {
		return 1
	}
	if cached, ok := s.cache.Load().(*channelCache); ok && cached != nil {
		channel, ok := cached.channelByGroupID[groupID]
		if ok && channel != nil && channel.IsActive() {
			return channel.resolveBillingMultiplierFromRules(groupID, accountID)
		}
	}
	if s.repo == nil {
		return 1
	}
	channel, err := s.GetChannelForGroup(ctx, groupID)
	if err != nil || channel == nil {
		return 1
	}
	return channel.resolveBillingMultiplierFromRules(groupID, accountID)
}

func (c *Channel) resolveBillingMultiplierFromRules(groupID int64, accountID int64) float64 {
	if c == nil {
		return 1
	}
	for _, rule := range c.BillingMultiplierRules {
		if matchBillingMultiplierRule(&rule, groupID, accountID) {
			if rule.RateMultiplier < 0 {
				return 0
			}
			return rule.RateMultiplier
		}
	}
	return 1
}

func matchBillingMultiplierRule(rule *ChannelBillingMultiplierRule, groupID, accountID int64) bool {
	if rule == nil || len(rule.GroupIDs) == 0 {
		return false
	}

	groupMatched := false
	for _, id := range rule.GroupIDs {
		if id == groupID {
			groupMatched = true
			break
		}
	}
	if !groupMatched {
		return false
	}

	if len(rule.AccountIDs) == 0 {
		return true
	}
	for _, id := range rule.AccountIDs {
		if id == accountID {
			return true
		}
	}
	return false
}

func applyChannelBillingMultiplier(
	ctx context.Context,
	channelService *ChannelService,
	groupID *int64,
	accountID int64,
	baseMultiplier float64,
) float64 {
	if channelService == nil || groupID == nil {
		return baseMultiplier
	}
	channelMultiplier := channelService.ResolveBillingMultiplier(ctx, *groupID, accountID)
	if channelMultiplier < 0 {
		channelMultiplier = 0
	}
	return baseMultiplier * channelMultiplier
}
