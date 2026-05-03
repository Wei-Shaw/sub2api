/**
 * Currency / number formatting helpers used by the payment plugin frontend.
 *
 * Trimmed-down copy of host frontend/src/utils/format.ts — we only re-export
 * formatCurrency since that's all the migrated payment views consume. Keeping
 * this slim avoids dragging the host's i18n singleton into the plugin bundle.
 */

import { getSdk } from '../api/sdk'

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
 * @param amount - amount in dollars (already converted from minor units)
 * @param currency - ISO 4217 currency code (default USD)
 */
export function formatCurrency(amount: number | null | undefined, currency: string = 'USD'): string {
  if (amount === null || amount === undefined) return '$0.00'
  const num = Number(amount)
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
