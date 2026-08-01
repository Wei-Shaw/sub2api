export interface CurrencyMeta {
  minor_units: number
  symbol: string
}

const DEFAULT_CURRENCY_META: Record<string, CurrencyMeta> = {
  USD: { minor_units: 2, symbol: '$' },
  CNY: { minor_units: 2, symbol: '¥' },
  VND: { minor_units: 0, symbol: '₫' },
  KRW: { minor_units: 0, symbol: '₩' },
  JPY: { minor_units: 0, symbol: '¥' },
  EUR: { minor_units: 2, symbol: '€' },
}

export function normalizeCurrency(currency: string | null | undefined, fallback = 'USD'): string {
  const normalized = String(currency || '').trim().toUpperCase()
  return normalized || fallback
}

export function currencyMeta(
  currency: string | null | undefined,
  meta: Record<string, CurrencyMeta> = {},
): CurrencyMeta {
  const normalized = normalizeCurrency(currency)
  return meta[normalized] ?? DEFAULT_CURRENCY_META[normalized] ?? { minor_units: 2, symbol: normalized }
}

export function currencyMinorUnits(currency: string | null | undefined, meta: Record<string, CurrencyMeta> = {}): number {
  const units = currencyMeta(currency, meta).minor_units
  return Number.isInteger(units) && units >= 0 ? Math.min(units, 8) : 2
}

export function roundMoney(amount: number, currency: string | null | undefined, meta: Record<string, CurrencyMeta> = {}): number {
  const units = currencyMinorUnits(currency, meta)
  const factor = 10 ** units
  return Math.round((Number(amount) || 0) * factor) / factor
}

export function ceilMoney(amount: number, currency: string | null | undefined, meta: Record<string, CurrencyMeta> = {}): number {
  const units = currencyMinorUnits(currency, meta)
  const factor = 10 ** units
  return Math.ceil(((Number(amount) || 0) * factor) - 1e-9) / factor
}

export function formatMoney(
  amount: number | null | undefined,
  currency: string | null | undefined,
  meta: Record<string, CurrencyMeta> = {},
): string {
  const normalized = normalizeCurrency(currency)
  const { symbol } = currencyMeta(normalized, meta)
  const units = currencyMinorUnits(normalized, meta)
  const value = Number(amount) || 0
  return `${symbol}${value.toLocaleString(undefined, {
    minimumFractionDigits: units,
    maximumFractionDigits: units,
  })}`
}

export function ledgerAmountFromPayment(
  paymentAmount: number,
  paymentCurrency: string,
  ledgerCurrency: string,
  fxRates: Record<string, number>,
  meta: Record<string, CurrencyMeta> = {},
): number {
  const normalizedPayment = normalizeCurrency(paymentCurrency, ledgerCurrency)
  const normalizedLedger = normalizeCurrency(ledgerCurrency)
  if (normalizedPayment === normalizedLedger) {
    return roundMoney(paymentAmount, normalizedLedger, meta)
  }
  const rate = fxRates[normalizedPayment]
  if (!Number.isFinite(rate) || rate <= 0) {
    return 0
  }
  return roundMoney(paymentAmount * rate, normalizedLedger, meta)
}

export function paymentAmountFromLedger(
  ledgerAmount: number,
  paymentCurrency: string,
  ledgerCurrency: string,
  fxRates: Record<string, number>,
  meta: Record<string, CurrencyMeta> = {},
): number {
  const normalizedPayment = normalizeCurrency(paymentCurrency, ledgerCurrency)
  const normalizedLedger = normalizeCurrency(ledgerCurrency)
  if (normalizedPayment === normalizedLedger) {
    return roundMoney(ledgerAmount, normalizedPayment, meta)
  }
  const rate = fxRates[normalizedPayment]
  if (!Number.isFinite(rate) || rate <= 0) {
    return 0
  }
  return ceilMoney(ledgerAmount / rate, normalizedPayment, meta)
}
