import type { UserPricingInterval, UserSupportedModelPricing } from '@/api/channels'
import {
  BILLING_MODE_IMAGE,
  BILLING_MODE_PER_REQUEST,
  BILLING_MODE_TOKEN,
  BILLING_MODE_VIDEO
} from '@/constants/channel'

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
  let s = Number((value * scale).toPrecision(10)).toString()
  if (minFractionDigits > 0 && !s.includes('e')) {
    const dot = s.indexOf('.')
    const digits = dot === -1 ? 0 : s.length - dot - 1
    if (digits < minFractionDigits) {
      s = (dot === -1 ? `${s}.` : s) + '0'.repeat(minFractionDigits - digits)
    }
  }
  return `$${s}`
}

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

const PER_MILLION = 1_000_000

/**
 * Compact one-line price for model chips (always-visible summary).
 * Returns empty string when pricing is missing / all fields empty.
 *
 *   token        → "$3 / $15"           (input / output per 1M tokens)
 *   per_request  → "$0.05"              (flat per request)
 *   image        → "$0.04"              (image output or per-request)
 *   video        → "$0.05/s"            (per second)
 */
export function summarizeModelPricing(pricing: UserSupportedModelPricing | null | undefined): string {
  if (!pricing) return ''

  switch (pricing.billing_mode) {
    case BILLING_MODE_PER_REQUEST: {
      if (pricing.per_request_price == null) return firstIntervalPerRequest(pricing) ?? ''
      return formatScaled(pricing.per_request_price, 1)
    }
    case BILLING_MODE_IMAGE: {
      const value = pricing.image_output_price ?? pricing.per_request_price
      if (value != null) return formatScaled(value, 1)
      return firstIntervalPerRequest(pricing) ?? ''
    }
    case BILLING_MODE_VIDEO: {
      if (pricing.per_request_price != null) {
        return `${formatScaled(pricing.per_request_price, 1)}/s`
      }
      const tier = firstIntervalPerRequest(pricing)
      return tier ? `${tier}/s` : ''
    }
    case BILLING_MODE_TOKEN:
    default: {
      if (pricing.input_price != null || pricing.output_price != null) {
        return `${formatScaled(pricing.input_price, PER_MILLION)} / ${formatScaled(pricing.output_price, PER_MILLION)}`
      }
      // Fall back to first interval when flat prices are absent.
      const iv = pricing.intervals?.[0]
      if (!iv) return ''
      const resolved = resolveIntervalPrices(iv, pricing)
      if (resolved.input_price == null && resolved.output_price == null) {
        if (resolved.per_request_price != null) return formatScaled(resolved.per_request_price, 1)
        return ''
      }
      return `${formatScaled(resolved.input_price, PER_MILLION)} / ${formatScaled(resolved.output_price, PER_MILLION)}`
    }
  }
}

function firstIntervalPerRequest(pricing: UserSupportedModelPricing): string | null {
  for (const iv of pricing.intervals ?? []) {
    if (iv.per_request_price != null) return formatScaled(iv.per_request_price, 1)
  }
  return null
}
