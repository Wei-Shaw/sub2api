/**
 * formatScaled formats a per-token (or per-request) USD price scaled by `scale`.
 *
 *   formatScaled(0.000003, 1_000_000)    → "$3"      // per 1M tokens
 *   formatScaled(0.5,        1)          → "$0.5"    // per request
 *   formatScaled(null,       1_000_000)  → "-"
 *   formatScaled(0.000003, 1_000_000, 2) → "$3.00"   // pad to ≥2 decimals
 *   formatScaled(1.25e-8,  1_000_000, 2) → "$0.0125" // longer decimals kept as-is
 *
 * Uses toPrecision(10) then strips trailing zeros to avoid IEEE 754 display noise.
 * `minFractionDigits` pads the result back up to a minimum number of decimals.
 */
export function formatScaled(value: number | null, scale: number, minFractionDigits = 0): string {
  if (value == null) return '-'
  let s = (value * scale).toPrecision(10).replace(/\.?0+$/, '')
  if (minFractionDigits > 0 && !s.includes('e')) {
    const dot = s.indexOf('.')
    const digits = dot === -1 ? 0 : s.length - dot - 1
    if (digits < minFractionDigits) {
      s = (dot === -1 ? `${s}.` : s) + '0'.repeat(minFractionDigits - digits)
    }
  }
  return `$${s}`
}

import type { UserPricingInterval } from '@/api/channels'

type TokenPrices = Pick<UserPricingInterval, 'input_price' | 'output_price' | 'cache_write_price' | 'cache_write_1h_price' | 'cache_read_price'>

export function resolveIntervalPrices(iv: UserPricingInterval, base: TokenPrices): UserPricingInterval {
  const price = (absolute: number | null | undefined, multiplier: number | null | undefined, fallback: number | null | undefined) =>
    absolute ?? (fallback == null ? null : fallback * (multiplier ?? 1))
  return {
    ...iv,
    input_price: price(iv.input_price, iv.input_multiplier, base.input_price),
    output_price: price(iv.output_price, iv.output_multiplier, base.output_price),
    cache_write_price: price(iv.cache_write_price, iv.cache_write_multiplier, base.cache_write_price),
    // Resolver uses an explicit cache-write price for both durations unless 1h is overridden.
    cache_write_1h_price: iv.cache_write_1h_price ?? iv.cache_write_price ?? price(null, iv.cache_write_multiplier, base.cache_write_1h_price),
    cache_read_price: price(iv.cache_read_price, iv.cache_read_multiplier, base.cache_read_price)
  }
}
