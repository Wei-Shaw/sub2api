//go:build unit

package service

// newPricingServiceFromModelPricing 把计费侧价卡转成目录条目（nil 条目跳过），
// 供需要精确控制单张价卡字段（含 5m/1h 缓存写入价与长上下文阶梯）的夹具使用。
func newPricingServiceFromModelPricing(prices map[string]*ModelPricing) *PricingService {
	data := make(map[string]*LiteLLMModelPricing, len(prices))
	for name, p := range prices {
		if p == nil {
			continue
		}
		cacheWrite := p.CacheCreationPricePerToken
		if p.CacheCreation5mPrice > 0 {
			cacheWrite = p.CacheCreation5mPrice
		}
		data[name] = &LiteLLMModelPricing{
			InputCostPerToken:                   p.InputPricePerToken,
			InputCostPerTokenPriority:           p.InputPricePerTokenPriority,
			OutputCostPerToken:                  p.OutputPricePerToken,
			OutputCostPerTokenPriority:          p.OutputPricePerTokenPriority,
			CacheCreationInputTokenCost:         cacheWrite,
			CacheCreationInputTokenCostPriority: p.CacheCreationPricePerTokenPriority,
			CacheCreationInputTokenCostAbove1hr: p.CacheCreation1hPrice,
			CacheReadInputTokenCost:             p.CacheReadPricePerToken,
			CacheReadInputTokenCostPriority:     p.CacheReadPricePerTokenPriority,
			LongContextInputTokenThreshold:      p.LongContextInputThreshold,
			LongContextInputCostMultiplier:      p.LongContextInputMultiplier,
			LongContextOutputCostMultiplier:     p.LongContextOutputMultiplier,
			InputCostPerImageToken:              p.ImageInputPricePerToken,
			OutputCostPerImageToken:             p.ImageOutputPricePerToken,
		}
	}
	return &PricingService{pricingData: data}
}
