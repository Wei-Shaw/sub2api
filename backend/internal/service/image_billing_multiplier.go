package service

func resolveImageRateMultiplier(apiKey *APIKey, effectiveGroupMultiplier float64) float64 {
	if apiKey != nil && apiKey.Group != nil {
		cfg := apiKey.Group.ImageConfig()
		if cfg.RateIndependent {
			if cfg.RateMultiplier < 0 {
				return 0
			}
			return cfg.RateMultiplier
		}
	}
	return effectiveGroupMultiplier
}
