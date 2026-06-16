// Money helper: wraps decimal.js Decimal so the rest of the codebase has
// a single import surface and consistent formatting.
//
// Why decimal.js: payment amounts arrive from the backend as strings
// (Go shopspring/decimal MarshalJSON emits "10.00" not 10.00) so we must
// avoid Number() coercion which loses precision. decimal.js works on
// strings end-to-end and matches the backend's rounding semantics.

import Decimal from 'decimal.js'

export type Money = Decimal

/**
 * Wrap an arbitrary input as a Decimal. Accepts:
 *   - string ("10.00") — the API contract for money fields
 *   - number — legacy / inline literals
 *   - Decimal — passthrough
 *   - null / undefined / "" — returns Decimal(0)
 *
 * Throws is intentional for genuinely invalid input (e.g. "abc"); use the
 * caller-side default `?? '0'` to opt into a soft fallback.
 */
export function money(value: string | number | Decimal | null | undefined): Money {
  if (value === null || value === undefined || value === '') return new Decimal(0)
  if (value instanceof Decimal) return value
  return new Decimal(value)
}

/**
 * Format a Money for display with the conventional 2-decimal precision.
 * Equivalent to `value.toFixed(2)` on a number, but precision-preserving.
 */
export function formatMoney(value: string | number | Decimal | null | undefined): string {
  return money(value).toFixed(2)
}

export { Decimal }
