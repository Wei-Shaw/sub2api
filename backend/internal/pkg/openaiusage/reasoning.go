package openaiusage

// OutputTokensWithMissingReasoning adds reasoning tokens only when totalTokens
// proves that the reported output does not already include them. This supports
// additive Chat Completions providers without double-counting OpenAI/Responses
// usage, where reasoning tokens are already part of output tokens.
func OutputTokensWithMissingReasoning(inputTokens, outputTokens, totalTokens, reasoningTokens int64) int64 {
	if inputTokens < 0 || outputTokens < 0 || totalTokens < 0 || reasoningTokens <= 0 {
		return outputTokens
	}
	if totalTokens <= inputTokens {
		return outputTokens
	}
	missing := totalTokens - inputTokens
	if missing <= outputTokens {
		return outputTokens
	}
	missing -= outputTokens
	if reasoningTokens < missing {
		missing = reasoningTokens
	}
	return outputTokens + missing
}
