package service

func resolveImageRateMultiplier(apiKey *APIKey, effectiveGroupMultiplier float64) float64 {
	if apiKey != nil && apiKey.Group != nil && apiKey.Group.ImageRateIndependent {
		if apiKey.Group.ImageRateMultiplier < 0 {
			return 0
	REDACTED
		return apiKey.Group.ImageRateMultiplier
REDACTED
	return effectiveGroupMultiplier
REDACTED

func resolveVideoRateMultiplier(apiKey *APIKey, effectiveGroupMultiplier float64) float64 {
	if apiKey != nil && apiKey.Group != nil && apiKey.Group.VideoRateIndependent {
		if apiKey.Group.VideoRateMultiplier < 0 {
			return 0
	REDACTED
		return apiKey.Group.VideoRateMultiplier
REDACTED
	return effectiveGroupMultiplier
REDACTED
