import { computed, unref, type MaybeRef } from 'vue'

export const DEFAULT_USD_CNY_RATE = 7.2

export function normalizeUsdCnyRate(rate?: number | null): number {
  const value = Number(rate ?? DEFAULT_USD_CNY_RATE)
  if (!Number.isFinite(value) || value <= 0) return DEFAULT_USD_CNY_RATE
  return value
}

function formatCurrencyAmount(value?: number | null, symbol = '¥'): string {
  const amount = Number(value ?? 0)
  if (!Number.isFinite(amount)) return `${symbol}0.00`
  const sign = amount < 0 ? '-' : ''
  const abs = Math.abs(amount)
  const fixed = abs.toFixed(4)
  if (abs > 0 && abs < 0.01) {
    return `${sign}${symbol}${fixed}`
  }
  const [whole, decimal = ''] = fixed.split('.')
  const trimmed = decimal.replace(/0+$/, '')
  const decimals = trimmed.length < 2 ? decimal.slice(0, 2) : trimmed
  return `${sign}${symbol}${whole}.${decimals}`
}

export function formatCny(usdAmount?: number | null, rate?: number | null): string {
  return formatCurrencyAmount(Number(usdAmount ?? 0) * normalizeUsdCnyRate(rate), '¥')
}

export function formatByCurrency(
  amount?: number | null,
  currency?: string | null,
  rate?: number | null
): string {
  if (String(currency ?? 'USD').trim().toUpperCase() === 'CNY') {
    return formatCurrencyAmount(amount, '¥')
  }
  return formatCny(amount, rate)
}

export function formatUsdAccountAmount(amount?: number | null): string {
  return formatCurrencyAmount(amount, '$')
}

export function useDisplayCurrency(rate?: MaybeRef<number | null | undefined>) {
  const usdCnyRate = computed(() => normalizeUsdCnyRate(unref(rate)))
  return {
    usdCnyRate,
    formatCny: (usdAmount?: number | null) => formatCny(usdAmount, usdCnyRate.value),
    formatByCurrency: (amount?: number | null, currency?: string | null) =>
      formatByCurrency(amount, currency, usdCnyRate.value),
    formatUsdAccountAmount
  }
}
