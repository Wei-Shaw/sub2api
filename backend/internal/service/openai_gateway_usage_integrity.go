package service

const grokMissingUsageMessage = "Grok upstream returned a successful response without billable usage"

// hasBillableOpenAIUsage distinguishes a usable accounting result from the
// zero value produced when an upstream omits usage entirely. A successful Grok
// completion without any positive usage cannot be safely charged, so callers
// must fail over before committing the response to the client.
func hasBillableOpenAIUsage(usage OpenAIUsage) bool {
	return usage.InputTokens > 0 ||
		usage.ImageInputTokens > 0 ||
		usage.OutputTokens > 0 ||
		usage.CacheCreationInputTokens > 0 ||
		usage.CacheReadInputTokens > 0 ||
		usage.ImageOutputTokens > 0
REDACTED
