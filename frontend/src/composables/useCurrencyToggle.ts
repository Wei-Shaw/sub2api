/**
 * Plaza CNY ⇄ USD toggle, anchored on `currency_meta.balance_recharge_multiplier`.
 *
 * 设计要点：
 * - 模型价格的 native currency 是 USD（来自 LiteLLM）；订阅套餐 native 是 CNY（运营定价）。
 * - 充值倍率定义：1 CNY 充值 → multiplier USD 信用（典型 0.10–0.20）。换算时：
 *     CNY → USD: usd = cny * multiplier
 *     USD → CNY: cny = usd / multiplier   （multiplier 为 0 时不换算，避免除零）
 * - 切换显示币种时只改前端展示，不重新请求接口。
 *
 * 偏好持久化在 localStorage（键 `plaza_currency_pref`），默认 `"CNY"`。
 */

import { computed, ref, watch } from 'vue'

export type PlazaCurrency = 'CNY' | 'USD'
export type PlazaNativeCurrency = 'USD' | 'CNY'

const STORAGE_KEY = 'plaza_currency_pref'
const DEFAULT: PlazaCurrency = 'CNY'

function readPref(): PlazaCurrency {
  try {
    const v = localStorage.getItem(STORAGE_KEY)
    if (v === 'CNY' || v === 'USD') return v
  } catch {
    /* ignored: SSR / storage disabled */
  }
  return DEFAULT
}

function writePref(v: PlazaCurrency): void {
  try {
    localStorage.setItem(STORAGE_KEY, v)
  } catch {
    /* ignored */
  }
}

/**
 * Build a currency toggle bound to a recharge multiplier.
 *
 * The multiplier is provided as a getter (typically `() => meta.value?.balance_recharge_multiplier`)
 * so the composable always reads the latest payload value without re-instantiating.
 */
export function useCurrencyToggle(getMultiplier: () => number | null | undefined) {
  const display = ref<PlazaCurrency>(readPref())

  // `flush: 'sync'` 让持久化与显式切换同步发生，避免 `set('USD')` 后立刻读 localStorage
  // 拿到旧值的情况；同时不影响下游模板渲染（display 仍是普通响应式 ref）。
  watch(display, (v) => writePref(v), { flush: 'sync' })

  /**
   * Read the current effective multiplier (>0) or 0 if unavailable.
   *
   * NOTE: this is a plain function rather than a `computed`. The `getMultiplier` getter is
   * typically backed by a non-reactive variable (template ref read inside a closure) when
   * used in tests; a `computed` would cache the first evaluation forever. Calling a function
   * each time guarantees we always see the latest value.
   */
  function readMultiplier(): number {
    const m = getMultiplier()
    return typeof m === 'number' && Number.isFinite(m) && m > 0 ? m : 0
  }

  // Exposed as a computed for templates that want to render the multiplier reactively.
  // It still re-reads the getter on each `.value` access via `readMultiplier()`.
  const multiplier = computed(() => readMultiplier())

  /**
   * Convert `amount` (expressed in `native`) to the currently displayed currency.
   * If multiplier is unavailable or zero, returns the amount unchanged with the native currency
   * so the UI still renders sensibly.
   */
  function convert(amount: number, native: PlazaNativeCurrency): { value: number; currency: PlazaCurrency } {
    if (!Number.isFinite(amount)) return { value: 0, currency: display.value }
    const m = readMultiplier()
    if (m <= 0) return { value: amount, currency: native as PlazaCurrency }
    if (display.value === native) {
      return { value: amount, currency: display.value }
    }
    if (native === 'CNY' && display.value === 'USD') {
      return { value: amount * m, currency: 'USD' }
    }
    if (native === 'USD' && display.value === 'CNY') {
      return { value: amount / m, currency: 'CNY' }
    }
    return { value: amount, currency: display.value }
  }

  /**
   * Format `amount` (in `native`) to a human-readable string with the current display currency.
   *
   * `digits` controls fraction digits (default 4 for USD-native model prices, 2 for CNY-native plans).
   */
  function format(amount: number, native: PlazaNativeCurrency, digits?: number): string {
    const { value, currency } = convert(amount, native)
    const fractionDigits = digits ?? (currency === 'USD' ? 4 : 2)
    const formatted = value.toLocaleString(undefined, {
      minimumFractionDigits: 0,
      maximumFractionDigits: fractionDigits,
    })
    const symbol = currency === 'CNY' ? '¥' : '$'
    return `${symbol}${formatted}`
  }

  function set(v: PlazaCurrency) {
    display.value = v
  }

  function toggle() {
    display.value = display.value === 'CNY' ? 'USD' : 'CNY'
  }

  return {
    display,
    multiplier,
    convert,
    format,
    set,
    toggle,
  }
}

export default useCurrencyToggle
