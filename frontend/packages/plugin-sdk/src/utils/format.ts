/**
 * Shared formatting helpers for plugin frontends.
 *
 * Single source of truth for the canonical "full date + 24h time, host-locale
 * aware" formatter that previously lived as a copy inside each plugin
 * (content-moderation/src/utils/format.ts, etc.).
 *
 * Locale resolution: read from the host SDK exposed on the global
 * `window[HOST_SDK_GLOBAL_KEY]` (set by the host during plugin bootstrap), so
 * the helper follows the host's active language without requiring the plugin to
 * thread its per-plugin SDK accessor through every call site. Falls back to the
 * browser locale before the host SDK is available.
 *
 * NOTE (money / currency): currency formatting that depends on decimal.js
 * (shopspring/decimal JSON parity) intentionally stays in the payment plugin —
 * pulling decimal.js into the shared SDK would force the dependency onto every
 * plugin (gateway / content-moderation) that never handles money.
 */
import { HOST_SDK_GLOBAL_KEY } from '../host-sdk'

/**
 * Current host locale, reactive value read at call time.
 *
 * Falls back to the browser locale (then `'en'`) when the host SDK has not yet
 * been injected on the global.
 */
export function getHostLocale(): string {
  try {
    const sdk =
      typeof window !== 'undefined' ? window[HOST_SDK_GLOBAL_KEY] : undefined
    const locale = sdk?.i18n.currentLocale.value
    if (locale) return locale
  } catch {
    // fall through to browser locale
  }
  return typeof navigator !== 'undefined' ? navigator.language : 'en'
}

const DATETIME_OPTIONS: Intl.DateTimeFormatOptions = {
  year: 'numeric',
  month: '2-digit',
  day: '2-digit',
  hour: '2-digit',
  minute: '2-digit',
  second: '2-digit',
  hour12: false,
}

/**
 * Format a date/time as full date + 24-hour time in the host locale.
 *
 * Returns an empty string for nullish / invalid input (matching the prior
 * content-moderation behaviour).
 */
export function formatDateTime(
  date: string | Date | null | undefined,
): string {
  if (!date) return ''
  const d = new Date(date)
  if (isNaN(d.getTime())) return ''
  return new Intl.DateTimeFormat(getHostLocale(), DATETIME_OPTIONS).format(d)
}
