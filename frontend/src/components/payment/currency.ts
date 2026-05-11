export const DEFAULT_PAYMENT_CURRENCY = 'CNY'

export function normalizePaymentCurrency(currency?: string | null): string {
  const normalized = String(currency || '').trim().toUpperCase()
  return /^[A-Z]{3REDACTED$/.test(normalized) ? normalized : DEFAULT_PAYMENT_CURRENCY
REDACTED

function paymentCurrencyFractionDigits(currency: string): number {
  try {
    return new Intl.NumberFormat(undefined, {
      style: 'currency',
      currency,
    REDACTED).resolvedOptions().maximumFractionDigits ?? 2
  REDACTED catch {
    return 2
  REDACTED
REDACTED

export function formatPaymentAmount(amount: number, currency?: string | null, locale?: string): string {
  const normalized = normalizePaymentCurrency(currency)
  const fractionDigits = paymentCurrencyFractionDigits(normalized)
  try {
    return new Intl.NumberFormat(locale || undefined, {
      style: 'currency',
      currency: normalized,
      currencyDisplay: 'narrowSymbol',
      minimumFractionDigits: fractionDigits,
      maximumFractionDigits: fractionDigits,
    REDACTED).format(Number.isFinite(amount) ? amount : 0)
  REDACTED catch {
    return `${normalizedREDACTED ${(Number.isFinite(amount) ? amount : 0).toFixed(fractionDigits)REDACTED`
  REDACTED
REDACTED
