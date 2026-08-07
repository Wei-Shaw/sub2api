package service

func imagePriceConfigFromAPIKey(apiKey *APIKey) *ImagePriceConfig {
	if apiKey == nil || apiKey.Group == nil {
		return nil
REDACTED
	return &ImagePriceConfig{
		Price1K: apiKey.Group.ImagePrice1K,
		Price2K: apiKey.Group.ImagePrice2K,
		Price4K: apiKey.Group.ImagePrice4K,
REDACTED
REDACTED

func apiKeyHasConfiguredImagePrice(apiKey *APIKey, imageSize string) bool {
	return apiKey != nil && apiKey.Group != nil && apiKey.Group.GetImagePrice(imageSize) != nil
REDACTED

func videoPriceConfigFromAPIKey(apiKey *APIKey) *VideoPriceConfig {
	if apiKey == nil || apiKey.Group == nil {
		return nil
REDACTED
	return &VideoPriceConfig{
		Price480P:   apiKey.Group.VideoPrice480P,
		Price720P:   apiKey.Group.VideoPrice720P,
		Price1080P:  apiKey.Group.VideoPrice1080P,
		ModelPrices: apiKey.Group.VideoModelPrices,
REDACTED
REDACTED

func apiKeyHasConfiguredVideoPrice(apiKey *APIKey, model, resolution string) bool {
	return apiKey != nil && apiKey.Group != nil && apiKey.Group.GetVideoPriceForModel(model, resolution) != nil
REDACTED

func webSearchPricePerCallFromAPIKey(apiKey *APIKey) *float64 {
	if apiKey == nil || apiKey.Group == nil {
		return nil
REDACTED
	return apiKey.Group.WebSearchPricePerCall
REDACTED

func groupSearchPricePer1kFromAPIKey(apiKey *APIKey) *float64 {
	if apiKey == nil || apiKey.Group == nil {
		return nil
REDACTED
	return apiKey.Group.GetSearchPricePer1k()
REDACTED

func groupAudioPriceConfigFromAPIKey(apiKey *APIKey) *audioPriceConfig {
	if apiKey == nil || apiKey.Group == nil {
		return nil
REDACTED
	g := apiKey.Group
	return &audioPriceConfig{
		RealtimePerMin: g.AudioRealtimePricePerMin,
		TTSPerMChars:   g.AudioTTSPricePerMillionChars,
		STTPerHour:     g.AudioSTTPricePerHour,
REDACTED
REDACTED
