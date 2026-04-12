export interface UsageTokenLike {
  input_tokens: number
  output_tokens: number
  cache_read_tokens: number
  cache_creation_tokens: number
}

export function buildUsageTokenDisplay(row: UsageTokenLike) {
  const netInputTokens = row.input_tokens || 0
  const cacheReadTokens = row.cache_read_tokens || 0
  const cacheCreationTokens = row.cache_creation_tokens || 0
  const outputTokens = row.output_tokens || 0

  const displayInputTokens = netInputTokens + cacheReadTokens + cacheCreationTokens
  const displayTotalTokens = displayInputTokens + outputTokens

  return {
    netInputTokens,
    displayInputTokens,
    displayTotalTokens,
  }
}
