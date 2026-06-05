/**
 * Currency / number formatting helpers used by the payment plugin frontend.
 *
 * Trimmed-down copy of host frontend/src/utils/format.ts — we only re-export
 * formatCurrency since that's all the migrated payment views consume. Keeping
 * this slim avoids dragging the host's i18n singleton into the plugin bundle.
 */

import { getSdk } from '../api/sdk'
import { money } from './decimal'

/** Read the current locale from the host SDK; fall back to en-US if not set. */
function currentLocale(): string {
  try {
    return getSdk().i18n.currentLocale.value || 'en-US'
  } catch {
    return 'en-US'
  }
}

/**
 * Localised currency formatter.
 *
 * Accepts string (decimal — backend's shopspring/decimal JSON form) or
 * number (legacy callers). Internally we coerce through `money()` so the
 * value is precision-safe before handing it to Intl.NumberFormat (which
 * itself takes a JS number — same precision floor as before).
 *
 * @param amount - amount in dollars (already converted from minor units)
 * @param currency - ISO 4217 currency code (default USD)
 */
export function formatCurrency(amount: string | number | null | undefined, currency: string = 'USD'): string {
  if (amount === null || amount === undefined || amount === '') return '$0.00'
  let num: number
  try {
    num = money(amount).toNumber()
  } catch {
    return '$0.00'
  }
  if (!Number.isFinite(num)) return '$0.00'

  // IEEE 754 round-trip: 5e-8 * 1e6 = 0.04999...96 → 0.05
  const normalized = Number(num.toPrecision(10))
  const fractionDigits = normalized > 0 && normalized < 0.01 ? 6 : 2

  return new Intl.NumberFormat(currentLocale(), {
    style: 'currency',
    currency,
    minimumFractionDigits: fractionDigits,
    maximumFractionDigits: fractionDigits,
  }).format(normalized)
}
