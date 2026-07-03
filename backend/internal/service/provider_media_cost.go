package service

func applyProviderMediaCostMultiplier(cost *CostBreakdown, multiplier float64) *CostBreakdown {
	if cost == nil {
		return nil
	}
	if multiplier < 0 {
		multiplier = 0
	}
	next := *cost
	next.TotalCost = next.InputCost + next.OutputCost + next.ImageOutputCost + next.CacheCreationCost + next.CacheReadCost + next.ToolInvocationCost
	next.ActualCost = next.TotalCost * multiplier
	return &next
}
