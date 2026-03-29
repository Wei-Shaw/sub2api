import type { BillingMode, PricingInterval } from '@/api/admin/channels'

export interface IntervalFormEntry {
  min_tokens: number
  max_tokens: number | null
  tier_label: string
  input_price: number | string | null
  output_price: number | string | null
  cache_write_price: number | string | null
  cache_read_price: number | string | null
  per_request_price: number | string | null
  sort_order: number
}

export interface PricingFormEntry {
  modelsInput: string
  billing_mode: BillingMode
  input_price: number | string | null
  output_price: number | string | null
  cache_write_price: number | string | null
  cache_read_price: number | string | null
  per_request_price: number | string | null
  image_output_price: number | string | null
  intervals: IntervalFormEntry[]
}

export function toNullableNumber(val: number | string | null | undefined): number | null {
  if (val === null || val === undefined || val === '') return null
  const num = Number(val)
  return isNaN(num) ? null : num
}

export function apiIntervalsToForm(intervals: PricingInterval[]): IntervalFormEntry[] {
  return (intervals || []).map(iv => ({
    min_tokens: iv.min_tokens,
    max_tokens: iv.max_tokens,
    tier_label: iv.tier_label || '',
    input_price: iv.input_price,
    output_price: iv.output_price,
    cache_write_price: iv.cache_write_price,
    cache_read_price: iv.cache_read_price,
    per_request_price: iv.per_request_price,
    sort_order: iv.sort_order
  }))
}

export function formIntervalsToAPI(intervals: IntervalFormEntry[]): PricingInterval[] {
  return (intervals || []).map(iv => ({
    min_tokens: iv.min_tokens,
    max_tokens: iv.max_tokens,
    tier_label: iv.tier_label,
    input_price: toNullableNumber(iv.input_price),
    output_price: toNullableNumber(iv.output_price),
    cache_write_price: toNullableNumber(iv.cache_write_price),
    cache_read_price: toNullableNumber(iv.cache_read_price),
    per_request_price: toNullableNumber(iv.per_request_price),
    sort_order: iv.sort_order
  }))
}
