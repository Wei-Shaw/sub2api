import type { RechargeBonusTier } from '@/types/payment'

export type { RechargeBonusTier }

export type RechargeBonusMode = 'bonus' | 'discount'

export const MAX_RECHARGE_BONUS_TIERS = 20
export const MAX_RECHARGE_BONUS_PERCENT = 1000

const AMOUNT_EPSILON = 1e-9

// 后台编辑态：两个值都允许留空（未完成的行在提交时丢弃）。
export interface RechargeBonusTierDraft {
  min_amount: number | null
  bonus_percent: number | null
}

export interface RechargeBonusInterval {
  from: number
  /** null 表示开区间（≥ from） */
  to: number | null
  percent: number
}

export interface RechargeBonusQuote {
  mode: RechargeBonusMode
  /** 命中档位的百分比；未命中或未产生优惠时为 0 */
  percent: number
  /** 网关收款基数（支付币种，不含手续费）；赠金模式 = 输入金额，折扣模式 = 折后金额 */
  payBase: number
  /** 到账基数（输入金额 × 倍率，USD），不含赠送 */
  base: number
  /** 免费额度（USD）：赠金模式为额外赠送，折扣模式为未付费却到账的部分 */
  bonus: number
  /** 到账总额（USD） */
  credited: number
  tier: RechargeBonusTier | null
}

export function roundRechargeAmount(value: number, digits = 2): number {
  if (!Number.isFinite(value)) return 0
  const factor = 10 ** digits
  return Math.round((value + Number.EPSILON) * factor) / factor
}

function hasAtMostTwoDecimals(value: number): boolean {
  return Math.abs(roundRechargeAmount(value) - value) < AMOUNT_EPSILON
}

export function normalizeRechargeBonusMode(raw: unknown): RechargeBonusMode {
  return String(raw ?? '').trim().toLowerCase() === 'discount' ? 'discount' : 'bonus'
}

export function isRechargeBonusMinAmountValid(value: unknown): value is number {
  return typeof value === 'number' && Number.isFinite(value) && value >= 0 && hasAtMostTwoDecimals(value)
}

export function isRechargeBonusPercentValid(value: unknown): value is number {
  return (
    typeof value === 'number' &&
    Number.isFinite(value) &&
    value >= 0 &&
    value <= MAX_RECHARGE_BONUS_PERCENT &&
    hasAtMostTwoDecimals(value)
  )
}

// 折扣模式下百分比必须 < 100，否则实付为 0 或负数；与后端 ValidateRechargeBonusTiersForMode 一致。
export function isRechargeBonusPercentValidForMode(value: unknown, mode: RechargeBonusMode): value is number {
  if (!isRechargeBonusPercentValid(value)) return false
  return mode !== 'discount' || value < 100
}

function minAmountKey(value: number): string {
  return roundRechargeAmount(value).toFixed(2)
}

function collectTiers(candidates: { min_amount: unknown; bonus_percent: unknown }[]): RechargeBonusTier[] {
  const seen = new Set<string>()
  const out: RechargeBonusTier[] = []
  for (const item of candidates) {
    const minAmount = item.min_amount
    const percent = item.bonus_percent
    if (!isRechargeBonusMinAmountValid(minAmount) || !isRechargeBonusPercentValid(percent)) continue
    const key = minAmountKey(minAmount)
    if (seen.has(key)) continue
    seen.add(key)
    out.push({ min_amount: minAmount, bonus_percent: percent })
  }
  out.sort((a, b) => a.min_amount - b.min_amount)
  return out
}

// 读路径宽松归一（checkout-info / 后台 GET 回填）：非法行丢弃，同阈值保留先出现，按阈值升序。
export function normalizeRechargeBonusTiers(raw: unknown): RechargeBonusTier[] {
  if (!Array.isArray(raw)) return []
  const candidates: { min_amount: unknown; bonus_percent: unknown }[] = []
  for (const item of raw) {
    if (!item || typeof item !== 'object') continue
    const record = item as { min_amount?: unknown; bonus_percent?: unknown }
    candidates.push({
      min_amount: record.min_amount === null || record.min_amount === undefined || record.min_amount === '' ? NaN : Number(record.min_amount),
      bonus_percent: record.bonus_percent === null || record.bonus_percent === undefined || record.bonus_percent === '' ? NaN : Number(record.bonus_percent),
    })
  }
  return collectTiers(candidates)
}

// 提交清洗：留空/非法的行整行丢弃，同阈值保留先出现，按阈值升序。
export function sanitizeRechargeBonusTiersForSubmit(
  tiers: RechargeBonusTierDraft[] | null | undefined,
): RechargeBonusTier[] {
  if (!Array.isArray(tiers)) return []
  return collectTiers(
    tiers.map((tier) => ({
      min_amount: tier?.min_amount === null || tier?.min_amount === undefined ? NaN : Number(tier.min_amount),
      bonus_percent: tier?.bonus_percent === null || tier?.bonus_percent === undefined ? NaN : Number(tier.bonus_percent),
    })),
  )
}

// 编辑器即时校验：同一阈值在其他行已出现时返回 true。
export function isDuplicateRechargeBonusMinAmount(tiers: RechargeBonusTierDraft[], index: number): boolean {
  const current = tiers[index]?.min_amount
  if (!isRechargeBonusMinAmountValid(current)) return false
  const key = minAmountKey(current)
  return tiers.some((tier, i) => i !== index && isRechargeBonusMinAmountValid(tier.min_amount) && minAmountKey(tier.min_amount) === key)
}

// 命中规则：取不超过支付金额的最大阈值档位；与后端 matchRechargeBonusTier 一致。
export function matchRechargeBonusTier(tiers: RechargeBonusTier[], paymentAmount: number): RechargeBonusTier | null {
  if (!Number.isFinite(paymentAmount) || paymentAmount <= 0) return null
  let matched: RechargeBonusTier | null = null
  for (const tier of tiers) {
    if (paymentAmount + AMOUNT_EPSILON < tier.min_amount) continue
    if (!matched || tier.min_amount > matched.min_amount) matched = tier
  }
  return matched
}

// 赠送额度 = 到账基数 × 百分比，保留两位小数；与后端 calculateRechargeBonus 一致。
export function calculateRechargeBonus(baseCredited: number, percent: number): number {
  if (!Number.isFinite(baseCredited) || !Number.isFinite(percent) || baseCredited <= 0 || percent <= 0) return 0
  return roundRechargeAmount((baseCredited * percent) / 100)
}

export interface RechargeBonusQuoteOptions {
  multiplier?: number
  mode?: RechargeBonusMode
  /** 支付币种小数位，折扣模式实付基数按此精度四舍五入 */
  currencyDigits?: number
}

// 充值页报价：阈值按支付金额比较；赠金模式按到账基数加赠送，折扣模式按百分比减实付。
// 与后端 quoteRechargeBonus 一致（含折扣 ≥ 100% 的 fail-safe）。
export function quoteRechargeBonus(
  tiers: RechargeBonusTier[],
  paymentAmount: number,
  options: RechargeBonusQuoteOptions | number = {},
): RechargeBonusQuote {
  const opts: RechargeBonusQuoteOptions = typeof options === 'number' ? { multiplier: options } : options
  const mode = opts.mode ?? 'bonus'
  const amount = Number.isFinite(paymentAmount) && paymentAmount > 0 ? paymentAmount : 0
  const rate = typeof opts.multiplier === 'number' && Number.isFinite(opts.multiplier) && opts.multiplier > 0 ? opts.multiplier : 1
  const digits = typeof opts.currencyDigits === 'number' && Number.isInteger(opts.currencyDigits) && opts.currencyDigits >= 0 ? opts.currencyDigits : 2
  const base = roundRechargeAmount(amount * rate)
  const quote: RechargeBonusQuote = { mode, percent: 0, payBase: amount, base, bonus: 0, credited: base, tier: null }
  const tier = matchRechargeBonusTier(tiers, amount)
  if (!tier || tier.bonus_percent <= 0) return quote
  quote.tier = tier
  if (mode === 'discount') {
    if (tier.bonus_percent >= 100) return quote
    const payBase = roundRechargeAmount((amount * (100 - tier.bonus_percent)) / 100, digits)
    if (payBase <= 0 || payBase >= amount) return quote
    const paidCredit = roundRechargeAmount(payBase * rate)
    quote.payBase = payBase
    quote.bonus = Math.max(0, roundRechargeAmount(base - paidCredit))
    quote.percent = tier.bonus_percent
    return quote
  }
  const bonus = calculateRechargeBonus(base, tier.bonus_percent)
  if (bonus <= 0) return quote
  quote.bonus = bonus
  quote.credited = roundRechargeAmount(base + bonus)
  quote.percent = tier.bonus_percent
  return quote
}

// 区间预览：把升序阈值列表展开为 [from, to) 区间；首档阈值 > 0 时补一段「无优惠」。
export function describeRechargeBonusIntervals(tiers: RechargeBonusTier[]): RechargeBonusInterval[] {
  const sorted = [...tiers].sort((a, b) => a.min_amount - b.min_amount)
  const out: RechargeBonusInterval[] = []
  if (sorted.length === 0) return out
  if (sorted[0]!.min_amount > 0) {
    out.push({ from: 0, to: sorted[0]!.min_amount, percent: 0 })
  }
  sorted.forEach((tier, index) => {
    out.push({ from: tier.min_amount, to: sorted[index + 1]?.min_amount ?? null, percent: tier.bonus_percent })
  })
  return out
}

// 展示用：去掉多余的小数 0（20 → "20"，12.5 → "12.5"）。
export function formatRechargeBonusNumber(value: number): string {
  if (!Number.isFinite(value)) return '0'
  return String(Number(value.toFixed(2)))
}
