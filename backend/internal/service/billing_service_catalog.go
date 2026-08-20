package service

import "github.com/Wei-Shaw/sub2api/internal/modelcatalog"

func tokenRatesToModelPricing(rates modelcatalog.TokenRates) *ModelPricing {
	return &ModelPricing{
		InputPricePerToken:                 rates.Input,
		OutputPricePerToken:                rates.Output,
		InputPricePerTokenPriority:         rates.InputPriority,
		OutputPricePerTokenPriority:        rates.OutputPriority,
		CacheCreationPricePerToken:         rates.CacheWrite,
		CacheCreationPricePerTokenPriority: rates.CacheWritePriority,
		CacheReadPricePerToken:             rates.CacheRead,
		CacheReadPricePerTokenPriority:     rates.CacheReadPriority,
		ImageInputPricePerToken:            rates.ImageInput,
		ImageOutputPricePerToken:           rates.ImageOutput,
		LongContextInputThreshold:          rates.LongContextInputThreshold,
		LongContextInputMultiplier:         rates.LongContextInputMultiplier,
		LongContextOutputMultiplier:        rates.LongContextOutputMultiplier,
		LongContextThresholdInclusive:      rates.LongContextThresholdInclusive,
	}
}

func tokenRatesToLiteLLMPricing(rates modelcatalog.TokenRates) *LiteLLMModelPricing {
	return &LiteLLMModelPricing{
		InputCostPerToken:                   rates.Input,
		OutputCostPerToken:                  rates.Output,
		InputCostPerTokenPriority:           rates.InputPriority,
		OutputCostPerTokenPriority:          rates.OutputPriority,
		CacheCreationInputTokenCost:         rates.CacheWrite,
		CacheCreationInputTokenCostPriority: rates.CacheWritePriority,
		CacheReadInputTokenCost:             rates.CacheRead,
		CacheReadInputTokenCostPriority:     rates.CacheReadPriority,
		InputCostPerImageToken:              rates.ImageInput,
		OutputCostPerImageToken:             rates.ImageOutput,
		LongContextInputTokenThreshold:      rates.LongContextInputThreshold,
		LongContextInputCostMultiplier:      rates.LongContextInputMultiplier,
		LongContextOutputCostMultiplier:     rates.LongContextOutputMultiplier,
		LongContextThresholdInclusive:       rates.LongContextThresholdInclusive,
		Mode:                                "chat",
	}
}

func overlayModelPricingFromCatalog(dst *ModelPricing, price *modelcatalog.Price) {
	if dst == nil || price == nil {
		return
	}
	if price.InputPerMTok != nil {
		dst.InputPricePerToken = modelcatalog.PerToken(*price.InputPerMTok)
	}
	if price.OutputPerMTok != nil {
		dst.OutputPricePerToken = modelcatalog.PerToken(*price.OutputPerMTok)
	}
	if price.InputPriorityPerMTok != nil {
		dst.InputPricePerTokenPriority = modelcatalog.PerToken(*price.InputPriorityPerMTok)
	}
	if price.OutputPriorityPerMTok != nil {
		dst.OutputPricePerTokenPriority = modelcatalog.PerToken(*price.OutputPriorityPerMTok)
	}
	if price.CacheWritePerMTok != nil {
		dst.CacheCreationPricePerToken = modelcatalog.PerToken(*price.CacheWritePerMTok)
	}
	if price.CacheWritePriorityPerMTok != nil {
		dst.CacheCreationPricePerTokenPriority = modelcatalog.PerToken(*price.CacheWritePriorityPerMTok)
	}
	if price.CacheReadPerMTok != nil {
		dst.CacheReadPricePerToken = modelcatalog.PerToken(*price.CacheReadPerMTok)
	}
	if price.CacheReadPriorityPerMTok != nil {
		dst.CacheReadPricePerTokenPriority = modelcatalog.PerToken(*price.CacheReadPriorityPerMTok)
	}
	if price.ImageInputPerMTok != nil {
		dst.ImageInputPricePerToken = modelcatalog.PerToken(*price.ImageInputPerMTok)
	}
	if price.ImageOutputPerMTok != nil {
		dst.ImageOutputPricePerToken = modelcatalog.PerToken(*price.ImageOutputPerMTok)
	}
	if price.LongContextInputThreshold > 0 {
		dst.LongContextInputThreshold = price.LongContextInputThreshold
	}
	if price.LongContextInputMultiplier != 0 {
		dst.LongContextInputMultiplier = price.LongContextInputMultiplier
	}
	if price.LongContextOutputMultiplier != 0 {
		dst.LongContextOutputMultiplier = price.LongContextOutputMultiplier
	}
	if price.LongContextThresholdInclusive {
		dst.LongContextThresholdInclusive = true
	}
}

func overlayLiteLLMFromCatalog(dst *LiteLLMModelPricing, price *modelcatalog.Price) {
	if dst == nil || price == nil {
		return
	}
	if price.InputPerMTok != nil {
		dst.InputCostPerToken = modelcatalog.PerToken(*price.InputPerMTok)
	}
	if price.OutputPerMTok != nil {
		dst.OutputCostPerToken = modelcatalog.PerToken(*price.OutputPerMTok)
	}
	if price.InputPriorityPerMTok != nil {
		dst.InputCostPerTokenPriority = modelcatalog.PerToken(*price.InputPriorityPerMTok)
	}
	if price.OutputPriorityPerMTok != nil {
		dst.OutputCostPerTokenPriority = modelcatalog.PerToken(*price.OutputPriorityPerMTok)
	}
	if price.CacheWritePerMTok != nil {
		dst.CacheCreationInputTokenCost = modelcatalog.PerToken(*price.CacheWritePerMTok)
	}
	if price.CacheWritePriorityPerMTok != nil {
		dst.CacheCreationInputTokenCostPriority = modelcatalog.PerToken(*price.CacheWritePriorityPerMTok)
	}
	if price.CacheReadPerMTok != nil {
		dst.CacheReadInputTokenCost = modelcatalog.PerToken(*price.CacheReadPerMTok)
	}
	if price.CacheReadPriorityPerMTok != nil {
		dst.CacheReadInputTokenCostPriority = modelcatalog.PerToken(*price.CacheReadPriorityPerMTok)
	}
	if price.ImageInputPerMTok != nil {
		dst.InputCostPerImageToken = modelcatalog.PerToken(*price.ImageInputPerMTok)
	}
	if price.ImageOutputPerMTok != nil {
		dst.OutputCostPerImageToken = modelcatalog.PerToken(*price.ImageOutputPerMTok)
	}
	if price.LongContextInputThreshold > 0 {
		dst.LongContextInputTokenThreshold = price.LongContextInputThreshold
	}
	if price.LongContextInputMultiplier != 0 {
		dst.LongContextInputCostMultiplier = price.LongContextInputMultiplier
	}
	if price.LongContextOutputMultiplier != 0 {
		dst.LongContextOutputCostMultiplier = price.LongContextOutputMultiplier
	}
	if price.LongContextThresholdInclusive {
		dst.LongContextThresholdInclusive = true
	}
}

func catalogShouldWriteFallback(entry *modelcatalog.Entry) bool {
	if entry == nil || entry.Price == nil {
		return false
	}
	return entry.IsCanonical() || modelcatalog.SharedRateCardID(entry.ID) != ""
}

func (s *BillingService) applyCatalogFallbackPricing() {
	if s == nil {
		return
	}
	if s.fallbackPrices == nil {
		s.fallbackPrices = make(map[string]*ModelPricing)
	}
	for _, entry := range modelcatalog.Default().Entries() {
		if !catalogShouldWriteFallback(entry) {
			continue
		}
		if existing, ok := s.fallbackPrices[entry.ID]; ok {
			if entry.LockPrice {
				overlayModelPricingFromCatalog(existing, entry.Price)
			}
			continue
		}
		s.fallbackPrices[entry.ID] = tokenRatesToModelPricing(entry.Rates())
	}
}

func (s *BillingService) lookupExactFallbackPricing(model string) *ModelPricing {
	if s == nil || s.fallbackPrices == nil {
		return nil
	}
	if pricing, ok := s.fallbackPrices[model]; ok {
		return pricing
	}
	if card := modelcatalog.SharedRateCardID(model); card != "" && card != model {
		if pricing, ok := s.fallbackPrices[card]; ok {
			return pricing
		}
	}
	return nil
}
