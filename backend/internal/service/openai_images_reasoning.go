package service

import "github.com/Wei-Shaw/sub2api/internal/config"

const (
	OpenAIImagesResponsesReasoningEffortLow     = config.OpenAIImagesResponsesReasoningEffortLow
	OpenAIImagesResponsesReasoningEffortMedium  = config.OpenAIImagesResponsesReasoningEffortMedium
	OpenAIImagesResponsesReasoningEffortHigh    = config.OpenAIImagesResponsesReasoningEffortHigh
	OpenAIImagesResponsesReasoningEffortXHigh   = config.OpenAIImagesResponsesReasoningEffortXHigh
	OpenAIImagesResponsesReasoningEffortDefault = config.OpenAIImagesResponsesReasoningEffortDefault
)

func IsValidOpenAIImagesResponsesReasoningEffort(raw string) bool {
	return config.IsValidOpenAIImagesResponsesReasoningEffort(raw)
}

func NormalizeOpenAIImagesResponsesReasoningEffort(raw string) string {
	return config.NormalizeOpenAIImagesResponsesReasoningEffort(raw)
}
