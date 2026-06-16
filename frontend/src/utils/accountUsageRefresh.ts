import type { Account } from '@/types'
import { usePlatforms } from '@/composables/usePlatforms'
import { getAccountTypeMeta } from '@/utils/platformFrontendMeta'

const normalizeUsageRefreshValue = (value: unknown): string => {
  if (value == null) return ''
  return String(value)
}

/**
 * Build a usage refresh key from account extra fields declared in metadata.
 * Checks AccountTypeDeclaration.frontend_meta.usage_refresh_extra_fields first,
 * falls back to hardcoded OpenAI OAuth codex fields.
 */
export const buildOpenAIUsageRefreshKey = (account: Pick<Account, 'id' | 'platform' | 'type' | 'updated_at' | 'last_used_at' | 'rate_limit_reset_at' | 'extra'>): string => {
  const { getAccountTypeDecl } = usePlatforms()
  const typeDecl = getAccountTypeDecl(account.platform, account.type)
  const meta = getAccountTypeMeta(typeDecl)

  // If metadata declares extra fields, use those
  if (meta.usage_refresh_extra_fields && meta.usage_refresh_extra_fields.length > 0) {
    const extra = account.extra ?? {}
    return [
      account.id,
      account.updated_at,
      account.last_used_at,
      account.rate_limit_reset_at,
      ...meta.usage_refresh_extra_fields.map(field => extra[field]),
    ].map(normalizeUsageRefreshValue).join('|')
  }

  // Fallback: hardcoded OpenAI OAuth check
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
    extra.codex_7d_window_minutes
  ].map(normalizeUsageRefreshValue).join('|')
}
