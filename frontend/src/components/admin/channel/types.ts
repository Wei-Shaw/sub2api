import type { BillingMode, PricingInterval } from '@/api/admin/channels'

type TranslateFn = (key: string, params?: Record<string, unknown>) => string

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
  models: string[]
  billing_mode: BillingMode
  input_price: number | string | null
  output_price: number | string | null
  cache_write_price: number | string | null
  cache_read_price: number | string | null
  image_input_price: number | string | null
  image_output_price: number | string | null
  per_request_price: number | string | null
  intervals: IntervalFormEntry[]
}

// 价格转换：后端存 per-token，前端显示 per-MTok ($/1M tokens)
const MTOK = 1_000_000

export function toNullableNumber(val: number | string | null | undefined): number | null {
  if (val === null || val === undefined || val === '') return null
  const num = Number(val)
  return isNaN(num) ? null : num
}

/** 前端显示值($/MTok) → 后端存储值(per-token) */
export function mTokToPerToken(val: number | string | null | undefined): number | null {
  const num = toNullableNumber(val)
  return num === null ? null : parseFloat((num / MTOK).toPrecision(10))
}

/** 后端存储值(per-token) → 前端显示值($/MTok) */
export function perTokenToMTok(val: number | null | undefined): number | null {
  if (val === null || val === undefined) return null
  // toPrecision(10) 消除 IEEE 754 浮点乘法精度误差，如 5e-8 * 1e6 = 0.04999...96 → 0.05
  return parseFloat((val * MTOK).toPrecision(10))
}

export function apiIntervalsToForm(intervals: PricingInterval[]): IntervalFormEntry[] {
  return (intervals || []).map(iv => ({
    min_tokens: iv.min_tokens,
    max_tokens: iv.max_tokens,
    tier_label: iv.tier_label || '',
    input_price: perTokenToMTok(iv.input_price),
    output_price: perTokenToMTok(iv.output_price),
    cache_write_price: perTokenToMTok(iv.cache_write_price),
    cache_read_price: perTokenToMTok(iv.cache_read_price),
    per_request_price: iv.per_request_price,
    sort_order: iv.sort_order
  }))
}

export function formIntervalsToAPI(intervals: IntervalFormEntry[]): PricingInterval[] {
  return (intervals || []).map(iv => ({
    min_tokens: iv.min_tokens,
    max_tokens: iv.max_tokens,
    tier_label: iv.tier_label,
    input_price: mTokToPerToken(iv.input_price),
    output_price: mTokToPerToken(iv.output_price),
    cache_write_price: mTokToPerToken(iv.cache_write_price),
    cache_read_price: mTokToPerToken(iv.cache_read_price),
    per_request_price: toNullableNumber(iv.per_request_price),
    sort_order: iv.sort_order
  }))
}

// ── 模型模式冲突检测 ──────────────────────────────────────

interface ModelPattern {
  pattern: string
  prefix: string  // lowercase, 通配符去掉尾部 *
  wildcard: boolean
}

function toModelPattern(model: string): ModelPattern {
  const lower = model.toLowerCase()
  const wildcard = lower.endsWith('*')
  return {
    pattern: model,
    prefix: wildcard ? lower.slice(0, -1) : lower,
    wildcard,
  }
}

function patternsConflict(a: ModelPattern, b: ModelPattern): boolean {
  if (!a.wildcard && !b.wildcard) return a.prefix === b.prefix
  if (a.wildcard && !b.wildcard) return b.prefix.startsWith(a.prefix)
  if (!a.wildcard && b.wildcard) return a.prefix.startsWith(b.prefix)
  // 双通配符：任一前缀是另一前缀的前缀即冲突
  return a.prefix.startsWith(b.prefix) || b.prefix.startsWith(a.prefix)
}

/** 检测模型模式列表中的冲突，返回冲突的两个模式名；无冲突返回 null */
export function findModelConflict(models: string[]): [string, string] | null {
  const patterns = models.map(toModelPattern)
  for (let i = 0; i < patterns.length; i++) {
    for (let j = i + 1; j < patterns.length; j++) {
      if (patternsConflict(patterns[i], patterns[j])) {
        return [patterns[i].pattern, patterns[j].pattern]
      }
    }
  }
  return null
}

// ── 区间校验 ──────────────────────────────────────────────

type Translate = TranslateFn

const defaultTranslate: Translate = (key, params = {}) => {
  const messages: Record<string, string> = {
    'admin.channels.intervalValidation.minTokensNonNegative':
      'Interval #{index}: minimum token count ({value}) cannot be negative',
    'admin.channels.intervalValidation.maxTokensPositive':
      'Interval #{index}: maximum token count ({value}) must be greater than 0',
    'admin.channels.intervalValidation.maxTokensGreaterThanMin':
      'Interval #{index}: maximum token count ({max}) must be greater than minimum token count ({min})',
    'admin.channels.intervalValidation.priceNonNegative':
      'Interval #{index}: {name} cannot be negative',
    'admin.channels.intervalValidation.unboundedLast':
      'Interval #{index}: unbounded interval (empty max tokens) can only be the last interval',
    'admin.channels.intervalValidation.overlap':
      'Intervals #{prevIndex} and #{index} overlap: previous upper bound ({prevMax}) is greater than current lower bound ({min})',
    'admin.channels.intervalValidation.priceNames.input': 'Input price',
    'admin.channels.intervalValidation.priceNames.output': 'Output price',
    'admin.channels.intervalValidation.priceNames.cacheWrite': 'Cache write price',
    'admin.channels.intervalValidation.priceNames.cacheRead': 'Cache read price',
    'admin.channels.intervalValidation.priceNames.perRequest': 'Per-request price',
  }

  const template = messages[key] ?? key
  return template.replace(/\{(\w+)\}/g, (_, param: string) => String(params[param] ?? ''))
}

/** Validate interval rules and return a localized error message; returns null when valid.
 *
 * mode determines interval semantics:
 * - token: intervals are context token count segments (min, max], cannot overlap, unbounded must be last
 * - per_request / image: intervals are tier_label based (1K/2K/4K etc), backend matches by label,
 *   so overlap / last-unlimited checks are skipped
 */
export function validateIntervals(
  intervals: IntervalFormEntry[],
  translateOrMode: Translate | BillingMode = 'token',
  modeOrTranslate: BillingMode | Translate = defaultTranslate,
): string | null {
  const translate = typeof translateOrMode === 'function'
    ? translateOrMode
    : typeof modeOrTranslate === 'function'
      ? modeOrTranslate
      : defaultTranslate
  const validationMode = typeof translateOrMode === 'function'
    ? typeof modeOrTranslate === 'string' ? modeOrTranslate : 'token'
    : translateOrMode
  if (!intervals || intervals.length === 0) return null

  const sorted = [...intervals].sort((a, b) => a.min_tokens - b.min_tokens)

  for (let i = 0; i < sorted.length; i++) {
    const err = validateSingleInterval(sorted[i], i, translate)
    if (err) return err
  }

  // per_request / image 模式按 tier_label 匹配，不做 token 区间重叠校验
  if (validationMode !== 'token') return null
  return checkIntervalOverlap(sorted, translate)
}

function validateSingleInterval(
  iv: IntervalFormEntry,
  idx: number,
  translate: Translate,
): string | null {
  if (iv.min_tokens < 0) {
    return translate('admin.channels.intervalValidation.minTokensNonNegative', {
      index: idx + 1,
      value: iv.min_tokens,
    })
  }
  if (iv.max_tokens != null) {
    if (iv.max_tokens <= 0) {
      return translate('admin.channels.intervalValidation.maxTokensPositive', {
        index: idx + 1,
        value: iv.max_tokens,
      })
    }
    if (iv.max_tokens <= iv.min_tokens) {
      return translate('admin.channels.intervalValidation.maxTokensGreaterThanMin', {
        index: idx + 1,
        max: iv.max_tokens,
        min: iv.min_tokens,
      })
    }
  }
  return validateIntervalPrices(iv, idx, translate)
}

function validateIntervalPrices(
  iv: IntervalFormEntry,
  idx: number,
  translate: Translate,
): string | null {
  const prices: [string, number | string | null][] = [
    [translate('admin.channels.intervalValidation.priceNames.input'), iv.input_price],
    [translate('admin.channels.intervalValidation.priceNames.output'), iv.output_price],
    [translate('admin.channels.intervalValidation.priceNames.cacheWrite'), iv.cache_write_price],
    [translate('admin.channels.intervalValidation.priceNames.cacheRead'), iv.cache_read_price],
    [translate('admin.channels.intervalValidation.priceNames.perRequest'), iv.per_request_price],
  ]
  for (const [name, val] of prices) {
    if (val != null && val !== '' && Number(val) < 0) {
      return translate('admin.channels.intervalValidation.priceNonNegative', {
        index: idx + 1,
        name,
      })
    }
  }
  return null
}

function checkIntervalOverlap(
  sorted: IntervalFormEntry[],
  translate: Translate,
): string | null {
  for (let i = 0; i < sorted.length; i++) {
    if (sorted[i].max_tokens == null && i < sorted.length - 1) {
      return translate('admin.channels.intervalValidation.unboundedLast', { index: i + 1 })
    }
    if (i === 0) continue
    const prev = sorted[i - 1]
    if (prev.max_tokens == null || prev.max_tokens > sorted[i].min_tokens) {
      const prevMax = prev.max_tokens == null ? '∞' : String(prev.max_tokens)
      return translate('admin.channels.intervalValidation.overlap', {
        prevIndex: i,
        index: i + 1,
        prevMax,
        min: sorted[i].min_tokens,
      })
    }
  }
  return null
}

/** 平台对应的模型 tag 样式（背景+文字） */
export function getPlatformTagClass(platform: string): string {
  switch (platform) {
    case 'anthropic': return 'bg-orange-100 text-orange-700 dark:bg-orange-900/30 dark:text-orange-400'
    case 'openai': return 'bg-emerald-100 text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-400'
    case 'gemini': return 'bg-blue-100 text-blue-700 dark:bg-blue-900/30 dark:text-blue-400'
    case 'antigravity': return 'bg-purple-100 text-purple-700 dark:bg-purple-900/30 dark:text-purple-400'
    case 'grok': return 'bg-slate-100 text-slate-700 dark:bg-slate-800 dark:text-slate-300'
    default: return 'bg-gray-100 text-gray-700 dark:bg-gray-900/30 dark:text-gray-400'
  }
}

/** 平台对应的模型文字色（仅 text-*，用于 input/text 场景）— 与 getPlatformTagClass 同色系 */
export function getPlatformTextClass(platform: string): string {
  switch (platform) {
    case 'anthropic': return 'text-orange-700 dark:text-orange-400'
    case 'openai': return 'text-emerald-700 dark:text-emerald-400'
    case 'gemini': return 'text-blue-700 dark:text-blue-400'
    case 'antigravity': return 'text-purple-700 dark:text-purple-400'
    case 'grok': return 'text-slate-700 dark:text-slate-300'
    default: return ''
  }
}
