package service

import "strings"

// DeepSeek-compatible Responses endpoints require visible reasoning replay;
// OpenAI's encrypted-only history cleanup must not remove that protocol data.
func isDeepSeekReasoningReplayModel(model string) bool {
	model = strings.ToLower(strings.TrimSpace(model))
	if index := strings.LastIndex(model, "/"); index >= 0 {
		model = model[index+1:]
	}
	return strings.HasPrefix(model, "deepseek-")
}
