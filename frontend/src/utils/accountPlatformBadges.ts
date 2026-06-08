/**
 * Platform-specific badge helpers for the account table.
 *
 * TODO: These functions contain platform-specific display logic that ideally
 * should be provided by gateway plugins via PlatformDeclaration metadata.
 * For now they read from account.extra fields directly.
 *
 * TODO: Migrate to PlatformDeclaration.frontend_meta.badge_extractors when
 * that metadata is available. This is complex because badge rendering requires
 * reading nested extra fields (e.g. load_code_assist.paidTier.id) and mapping
 * them to i18n labels + CSS classes. A follow-up should define a declarative
 * badge_extractors schema in frontend_meta that describes these field paths,
 * value-to-label mappings, and value-to-class mappings.
 */

// ---------------------------------------------------------------------------
// Antigravity tier badge
// ---------------------------------------------------------------------------

interface AccountLike {
  platform: string
  type: string
  extra?: Record<string, unknown>
}

function getAntigravityTierFromRow(row: AccountLike): string | null {
  if (row.platform !== 'antigravity') return null
  const extra = row.extra
  if (!extra) return null
  const lca = extra.load_code_assist as Record<string, unknown> | undefined
  if (!lca) return null
  const paid = lca.paidTier as Record<string, unknown> | undefined
  if (paid && typeof paid.id === 'string') return paid.id
  const current = lca.currentTier as Record<string, unknown> | undefined
  if (current && typeof current.id === 'string') return current.id
  return null
}

export function getAntigravityTierLabel(
  row: AccountLike,
  t: (key: string) => string,
): string | null {
  const tier = getAntigravityTierFromRow(row)
  switch (tier) {
    case 'free-tier': return t('admin.accounts.tier.free')
    case 'g1-pro-tier': return t('admin.accounts.tier.pro')
    case 'g1-ultra-tier': return t('admin.accounts.tier.ultra')
    default: return null
  }
}

export function getAntigravityTierClass(row: AccountLike): string {
  const tier = getAntigravityTierFromRow(row)
  switch (tier) {
    case 'free-tier': return 'bg-gray-100 text-gray-600 dark:bg-gray-700 dark:text-gray-300'
    case 'g1-pro-tier': return 'bg-blue-100 text-blue-600 dark:bg-blue-900/40 dark:text-blue-300'
    case 'g1-ultra-tier': return 'bg-purple-100 text-purple-600 dark:bg-purple-900/40 dark:text-purple-300'
    default: return ''
  }
}

// ---------------------------------------------------------------------------
// OpenAI compact mode badge
// ---------------------------------------------------------------------------

type OpenAICompactBadgeState = 'active' | 'blocked' | 'auto'

function getOpenAICompactState(row: AccountLike): OpenAICompactBadgeState | null {
  if (row.platform !== 'openai' || (row.type !== 'oauth' && row.type !== 'apikey')) return null
  const extra = row.extra
  const mode = typeof extra?.openai_compact_mode === 'string' ? extra.openai_compact_mode : 'auto'
  if (mode === 'force_on') return 'active'
  if (mode === 'force_off') return 'blocked'
  if (typeof extra?.openai_compact_supported === 'boolean') {
    return extra.openai_compact_supported ? 'active' : 'blocked'
  }
  return 'auto'
}

interface CompactBadgeMeta {
  label: string
  className: string
  dotClass: string
}

export function getOpenAICompactMeta(
  row: AccountLike,
  t: (key: string) => string,
): CompactBadgeMeta | null {
  const state = getOpenAICompactState(row)
  if (!state) return null
  switch (state) {
    case 'active':
      return {
        label: t('admin.accounts.openai.compactSupported'),
        className: 'text-emerald-600 dark:text-emerald-300',
        dotClass: 'bg-emerald-500 shadow-[0_0_0_2px_rgba(16,185,129,0.14)]',
      }
    case 'blocked':
      return {
        label: t('admin.accounts.openai.compactUnsupported'),
        className: 'text-rose-600 dark:text-rose-300',
        dotClass: 'bg-rose-500 shadow-[0_0_0_2px_rgba(244,63,94,0.14)]',
      }
    case 'auto':
      return {
        label: t('admin.accounts.openai.compactAuto'),
        className: 'text-slate-500 dark:text-slate-400',
        dotClass: 'bg-slate-300 dark:bg-slate-500',
      }
  }
}

export function getOpenAICompactTitle(
  row: AccountLike,
  t: (key: string) => string,
  formatDateTime: (date: Date, opts?: Intl.DateTimeFormatOptions) => string,
): string {
  const extra = row.extra
  const checkedAt = typeof extra?.openai_compact_checked_at === 'string' ? extra.openai_compact_checked_at : ''
  const label = getOpenAICompactMeta(row, t)?.label || ''
  if (!checkedAt) return label
  return `${label} | ${t('admin.accounts.openai.compactLastChecked')}: ${formatDateTime(new Date(checkedAt))}`
}
