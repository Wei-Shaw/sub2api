package service

import (
	"context"
	"strings"
)

const featureKeyResponsesDefaultImageQuality = "responses_default_image_quality"

func normalizeConfiguredImageQuality(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case ImageQualityLow:
		return ImageQualityLow
	case ImageQualityHigh:
		return ImageQualityHigh
	default:
		return ImageQualityMedium
	}
}

func validOpenAIResponsesImageQuality(value string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case ImageQualityLow:
		return ImageQualityLow, true
	case ImageQualityMedium:
		return ImageQualityMedium, true
	case ImageQualityHigh:
		return ImageQualityHigh, true
	case "auto":
		return "auto", true
	default:
		return "", false
	}
}

func resolveOpenAIResponsesImageQuality(responseQuality, requestQuality, defaultQuality string) string {
	for _, candidate := range []string{responseQuality, requestQuality} {
		if quality, ok := validOpenAIResponsesImageQuality(candidate); ok {
			return quality
		}
	}
	return normalizeConfiguredImageQuality(defaultQuality)
}

func (c *Channel) ResponsesDefaultImageQuality(platform string) string {
	if c == nil || c.FeaturesConfig == nil {
		return ImageQualityMedium
	}
	raw, ok := c.FeaturesConfig[featureKeyResponsesDefaultImageQuality]
	if !ok {
		return ImageQualityMedium
	}
	if value, ok := raw.(string); ok {
		return normalizeConfiguredImageQuality(value)
	}
	values, ok := raw.(map[string]any)
	if !ok {
		return ImageQualityMedium
	}
	value, _ := values[strings.TrimSpace(platform)].(string)
	return normalizeConfiguredImageQuality(value)
}

func (s *OpenAIGatewayService) resolveOpenAIResponsesDefaultImageQuality(ctx context.Context, apiKey *APIKey) string {
	if s == nil || s.channelService == nil || apiKey == nil || apiKey.GroupID == nil {
		return ImageQualityMedium
	}
	channel, err := s.channelService.GetChannelForGroup(ctx, *apiKey.GroupID)
	if err != nil || channel == nil {
		return ImageQualityMedium
	}
	return channel.ResponsesDefaultImageQuality(PlatformOpenAI)
}
