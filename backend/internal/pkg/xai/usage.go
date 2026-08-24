package xai

// IncludeIndependentReasoningTokens adds reasoning tokens to billed output
// only when total_tokens proves they are not already folded into output.
//
// Official xAI Chat Completions example: prompt=32, completion=9,
// reasoning=94, total=135. Responses example: input=32, output=9,
// reasoning=110, total=151. OpenAI's canonical completion_tokens already
// includes reasoning and total equals input+output.
func IncludeIndependentReasoningTokens(input, output, total, reasoning int64) int64 {
	if input < 0 || output < 0 || reasoning <= 0 || total <= 0 {
		return output
	}
	if total == input+output {
		return output
	}
	gap := total - input - output
	if gap <= 0 {
		return output
	}
	if reasoning < gap {
		gap = reasoning
	}
	return output + gap
}
