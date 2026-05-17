/**
 * OpenAI OAuth usage refresh key builder.
 *
 * Builds a composite key from account fields and codex usage snapshot.
 * When this key changes, the host should re-fetch usage data.
 */

interface UsageRefreshAccount {
  id: number
  platform: string
  type: string
  updated_at: string
  last_used_at: string | null
  rate_limit_reset_at: string | null
  extra?: Record<string, unknown>
}

const normalizeUsageRefreshValue = (value: unknown): string => {
  if (value == null) return ''
  return String(value)
}

export const buildOpenAIUsageRefreshKey = (
  account: UsageRefreshAccount,
): string => {
  if (account.platform !== 'openai' || account.type !== 'oauth') {
    return ''
  }

  const extra = account.extra ?? {}
  return [
    account.id,
    account.updated_at,
    account.last_used_at,
    account.rate_limit_reset_at,
    extra.codex_usage_updated_at,
    extra.codex_5h_used_percent,
    extra.codex_5h_reset_at,
    extra.codex_5h_reset_after_seconds,
    extra.codex_5h_window_minutes,
    extra.codex_7d_used_percent,
    extra.codex_7d_reset_at,
    extra.codex_7d_reset_after_seconds,
    extra.codex_7d_window_minutes,
  ].map(normalizeUsageRefreshValue).join('|')
}
