export interface OutputTokenSpeedUsage {
  output_tokens?: number | null
  duration_ms?: number | null
  image_count?: number | null
  image_output_tokens?: number | null
}

export function calculateOutputTokensPerSecond(usage: OutputTokenSpeedUsage): number | null {
  if ((usage.image_count ?? 0) > 0 || (usage.image_output_tokens ?? 0) > 0) {
    return null
  }

  const outputTokens = usage.output_tokens
  const durationMs = usage.duration_ms
  if (
    typeof outputTokens !== 'number'
    || !Number.isFinite(outputTokens)
    || outputTokens <= 0
    || typeof durationMs !== 'number'
    || !Number.isFinite(durationMs)
    || durationMs <= 0
  ) {
    return null
  }

  return outputTokens * 1000 / durationMs
}
