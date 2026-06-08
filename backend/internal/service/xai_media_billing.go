package service

import (
	"encoding/json"
	"math"
	"strconv"
	"strings"
)

const (
	xaiDefaultVideoDurationSeconds = 1
	xaiDefaultVideoResolution      = "480p"
	xaiUSDTicksPerDollar           = 10_000_000_000
)

func buildXAIMediaCostFromUsageOrEstimate(usage OpenAIUsage, estimated *CostBreakdown) *CostBreakdown {
	if usage.CostInUSDTicks > 0 {
		return &CostBreakdown{
			OutputCost:  float64(usage.CostInUSDTicks) / xaiUSDTicksPerDollar,
			BillingMode: string(BillingModeImage),
		}
	}
	return estimated
}

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

func buildXAIImageMediaCost(model string, imageCount int, imageSize string, inputImageCount int) *CostBreakdown {
	if imageCount <= 0 {
		return nil
	}
	normalizedModel := normalizeXAIMediaModel(model)
	inputPrice, output1K, output2K, ok := xaiImagePrices(normalizedModel)
	if !ok {
		return nil
	}
	if inputImageCount < 0 {
		inputImageCount = 0
	}
	outputUnitPrice := output1K
	if normalizeXAIImageResolution(imageSize) == ImageBillingSize2K {
		outputUnitPrice = output2K
	}
	inputCost := float64(inputImageCount) * inputPrice
	outputCost := float64(imageCount) * outputUnitPrice
	return &CostBreakdown{
		InputCost:       inputCost,
		ImageOutputCost: outputCost,
		BillingMode:     string(BillingModeImage),
	}
}

func buildXAIVideoMediaCost(model string, payload map[string]any) *CostBreakdown {
	normalizedModel := normalizeXAIMediaModel(model)
	imageInputPrice, videoInputSecondPrice, output480p, output720p, ok := xaiVideoPrices(normalizedModel)
	if !ok {
		return nil
	}
	durationSeconds := xaiVideoDurationSeconds(payload)
	resolution := xaiVideoResolution(payload)
	outputUnitPrice := output480p
	if resolution == "720p" {
		outputUnitPrice = output720p
	}

	imageInputCount := countXAIMediaImageInputs(payload)
	inputCost := float64(imageInputCount) * imageInputPrice
	if videoInputSecondPrice > 0 && hasXAIVideoInput(payload) {
		inputVideoSeconds := xaiInputVideoDurationSeconds(payload)
		inputCost += float64(inputVideoSeconds) * videoInputSecondPrice
	}
	outputCost := float64(durationSeconds) * outputUnitPrice
	return &CostBreakdown{
		InputCost:   inputCost,
		OutputCost:  outputCost,
		BillingMode: string(BillingModeImage),
	}
}

func xaiImagePrices(model string) (inputPerImage, output1K, output2K float64, ok bool) {
	switch {
	case strings.HasPrefix(model, "grok-imagine-image-quality") || strings.HasPrefix(model, "grok-imagine-image-pro"):
		return 0.01, 0.05, 0.07, true
	case strings.HasPrefix(model, "grok-imagine-image"):
		return 0.002, 0.02, 0.02, true
	default:
		return 0, 0, 0, false
	}
}

func xaiVideoPrices(model string) (imageInputPerImage, videoInputPerSecond, output480pPerSecond, output720pPerSecond float64, ok bool) {
	switch {
	case strings.HasPrefix(model, "grok-imagine-video-1.5"):
		return 0.01, 0, 0.08, 0.14, true
	case strings.HasPrefix(model, "grok-imagine-video"):
		return 0.002, 0.01, 0.05, 0.07, true
	default:
		return 0, 0, 0, 0, false
	}
}

func normalizeXAIMediaModel(model string) string {
	return strings.TrimSpace(strings.ToLower(model))
}

func normalizeXAIImageResolution(size string) string {
	if strings.TrimSpace(size) == "" {
		return ImageBillingSize1K
	}
	switch NormalizeImageBillingTierOrDefault(size) {
	case ImageBillingSize1K:
		return ImageBillingSize1K
	default:
		return ImageBillingSize2K
	}
}

func xaiVideoResolution(payload map[string]any) string {
	for _, key := range []string{"resolution", "output_resolution"} {
		if v := strings.TrimSpace(strings.ToLower(xaiStringFromAny(payload[key]))); v != "" {
			if strings.Contains(v, "720") {
				return "720p"
			}
			if strings.Contains(v, "480") {
				return "480p"
			}
		}
	}
	return xaiDefaultVideoResolution
}

func xaiVideoDurationSeconds(payload map[string]any) int {
	return positiveCeilSecondsFromAny(payload["duration"], xaiDefaultVideoDurationSeconds)
}

func xaiInputVideoDurationSeconds(payload map[string]any) int {
	for _, key := range []string{"input_video_duration", "video_duration", "source_video_duration"} {
		if seconds := positiveCeilSecondsFromAny(payload[key], 0); seconds > 0 {
			return seconds
		}
	}
	return xaiVideoDurationSeconds(payload)
}

func resolveXAIImageBillingResolution(parsed *OpenAIImagesRequest, body []byte) string {
	if len(body) > 0 {
		var payload map[string]any
		if err := json.Unmarshal(body, &payload); err == nil {
			for _, key := range []string{"resolution", "size"} {
				if resolution := strings.TrimSpace(xaiStringFromAny(payload[key])); resolution != "" {
					return normalizeXAIImageResolution(resolution)
				}
			}
		}
	}
	if parsed != nil && parsed.ExplicitSize {
		return normalizeXAIImageResolution(parsed.SizeTier)
	}
	return ImageBillingSize1K
}

func positiveCeilSecondsFromAny(value any, fallback int) int {
	if seconds, ok := floatFromAny(value); ok && seconds > 0 {
		return int(math.Ceil(seconds))
	}
	if fallback > 0 {
		return fallback
	}
	return 0
}

func floatFromAny(value any) (float64, bool) {
	switch v := value.(type) {
	case float64:
		return v, true
	case float32:
		return float64(v), true
	case int:
		return float64(v), true
	case int64:
		return float64(v), true
	case json.Number:
		f, err := v.Float64()
		return f, err == nil
	case string:
		f, err := strconv.ParseFloat(strings.TrimSpace(v), 64)
		return f, err == nil
	default:
		return 0, false
	}
}

func xaiStringFromAny(value any) string {
	switch v := value.(type) {
	case string:
		return v
	case json.Number:
		return v.String()
	default:
		return ""
	}
}

func countXAIImageInputsFromImagesRequest(parsed *OpenAIImagesRequest, body []byte) int {
	count := 0
	if parsed != nil {
		count += len(parsed.InputImageURLs)
		count += len(parsed.Uploads)
	}
	if len(body) == 0 {
		return count
	}
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return count
	}
	return max(count, countXAIMediaImageInputs(payload))
}

func countXAIMediaImageInputs(payload map[string]any) int {
	if payload == nil {
		return 0
	}
	count := countMediaValue(payload["image"]) + countMediaValue(payload["image_url"])
	count += countMediaValue(payload["images"])
	count += countMediaValue(payload["reference_images"])
	count += countMediaValue(payload["reference_image_urls"])
	return count
}

func hasXAIVideoInput(payload map[string]any) bool {
	if payload == nil {
		return false
	}
	return countMediaValue(payload["video"]) > 0 || countMediaValue(payload["video_url"]) > 0 || countMediaValue(payload["source_video_url"]) > 0
}

func countMediaValue(value any) int {
	switch v := value.(type) {
	case nil:
		return 0
	case string:
		if strings.TrimSpace(v) == "" {
			return 0
		}
		return 1
	case []any:
		count := 0
		for _, item := range v {
			count += countMediaValue(item)
		}
		return count
	case map[string]any:
		for _, key := range []string{"url", "image_url", "video_url", "b64_json", "data"} {
			if countMediaValue(v[key]) > 0 {
				return 1
			}
		}
		return 0
	default:
		return 0
	}
}
