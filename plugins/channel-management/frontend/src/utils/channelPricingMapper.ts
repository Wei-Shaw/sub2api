/**
 * Pricing entry form ↔ API 单点映射 (T9 拆出).
 *
 * mTok ↔ per-token + interval mapping 公式集中在这里, 让 useChannelForm
 * Serializer 只关心更高层的 form-state 组装. 这两个函数在 channel pricing
 * 与 account_stats rule pricing 中复用 (单一公式, 复用原则).
 */
import type { ChannelModelPricing } from '../api/channels'
import type { PricingFormEntry } from '../components/types'
import {
  apiIntervalsToForm,
  formIntervalsToAPI,
  mTokToPerToken,
  perTokenToMTok,
} from '../components/types'

function numericOrNull(val: number | string | null | undefined): number | null {
  if (val == null || val === '') return null
  const n = Number(val)
  return Number.isFinite(n) ? n : null
}

/** Form 显示态 → API 存储态 (按 platform tag). */
export function pricingFormToAPI(
  platform: string,
  entry: PricingFormEntry,
): ChannelModelPricing {
  return {
    platform,
    models: entry.models,
    billing_mode: entry.billing_mode,
    input_price: mTokToPerToken(entry.input_price),
    output_price: mTokToPerToken(entry.output_price),
    cache_write_price: mTokToPerToken(entry.cache_write_price),
    cache_read_price: mTokToPerToken(entry.cache_read_price),
    image_output_price: mTokToPerToken(entry.image_output_price),
    per_request_price: numericOrNull(entry.per_request_price),
    intervals: formIntervalsToAPI(entry.intervals || []),
  }
}

/** API 存储态 → Form 显示态. */
export function pricingAPIToForm(p: ChannelModelPricing): PricingFormEntry {
  return {
    models: p.models || [],
    billing_mode: p.billing_mode,
    input_price: perTokenToMTok(p.input_price),
    output_price: perTokenToMTok(p.output_price),
    cache_write_price: perTokenToMTok(p.cache_write_price),
    cache_read_price: perTokenToMTok(p.cache_read_price),
    image_output_price: perTokenToMTok(p.image_output_price),
    per_request_price: p.per_request_price,
    intervals: apiIntervalsToForm(p.intervals || []),
  } as PricingFormEntry
}
