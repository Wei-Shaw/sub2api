/**
 * Model Plaza pricing display currency. Billing remains in USD; this is only a
 * fixed display conversion so public users can understand the listed prices.
 */
export const MODEL_PLAZA_USD_TO_VND = 1_000

/**
 * formatScaled formats a scaled price in the requested display currency.
 *
 * `currency` defaults to USD for existing channel screens. Model Plaza passes
 * `{ currency: 'VND' }`, using the fixed 1 USD = 1,000 VND display rate.
 */
export function formatScaled(
  value: number | null,
  scale: number,
  minFractionDigits = 0,
  options: { currency?: 'USD' | 'VND' } = {}
): string {
  if (value == null) return '-'
  const currency = options.currency ?? 'USD'
  const displayValue = value * scale * (currency === 'VND' ? MODEL_PLAZA_USD_TO_VND : 1)
  let s = displayValue.toPrecision(10).replace(/\.?0+$/, '')
  if (minFractionDigits > 0 && currency === 'USD' && !s.includes('e')) {
    const dot = s.indexOf('.')
    const digits = dot === -1 ? 0 : s.length - dot - 1
    if (digits < minFractionDigits) {
      s = (dot === -1 ? `${s}.` : s) + '0'.repeat(minFractionDigits - digits)
    }
  }
  if (currency === 'VND') {
    s = Number(s).toLocaleString('vi-VN', { maximumFractionDigits: 6 })
  }
  return currency === 'VND' ? `${s}₫` : `$${s}`
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
