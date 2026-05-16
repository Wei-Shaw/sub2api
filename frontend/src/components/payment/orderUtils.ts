/**
 * Shared utility functions for payment order display.
 * Used by AdminOrderDetail, AdminOrderTable, AdminRefundDialog, AdminOrdersView, etc.
 */

const STATUS_BADGE_MAP: Record<string, string> = {
  PENDING: 'badge-warning',
  PAID: 'badge-info',
  RECHARGING: 'badge-info',
  COMPLETED: 'badge-success',
  EXPIRED: 'badge-secondary',
  CANCELLED: 'badge-secondary',
  FAILED: 'badge-danger',
  REFUND_REQUESTED: 'badge-warning',
  REFUNDING: 'badge-warning',
  REFUND_PENDING: 'badge-warning',
  PARTIALLY_REFUNDED: 'badge-warning',
  REFUNDED: 'badge-info',
  REFUND_FAILED: 'badge-danger',
}

const REFUNDABLE_STATUSES = ['COMPLETED', 'PARTIALLY_REFUNDED', 'REFUND_REQUESTED', 'REFUND_FAILED']

export function statusBadgeClass(status: string): string {
  return STATUS_BADGE_MAP[status] || 'badge-secondary'
}

export function canRefund(status: string): boolean {
  return REFUNDABLE_STATUSES.includes(status)
}

export function formatOrderDateTime(dateStr: string): string {
  if (!dateStr) return '-'
  return new Date(dateStr).toLocaleString()
}

/**
 * Map a currency code to its display symbol.
 * Falls back to the code itself if unknown.
 */
const CURRENCY_SYMBOLS: Record<string, string> = {
  CNY: '¥',
  USD: '$',
  KRW: '₩',
  VND: '₫',
  EUR: '€',
  GBP: '£',
  JPY: '¥',
}

export function currencySymbol(currency?: string): string {
  if (!currency) return '¥'
  return CURRENCY_SYMBOLS[currency.toUpperCase()] || currency
}

/**
 * Resolve the payment currency from an order object.
 * Admin API returns `payment_currency` directly on the entity.
 * User API returns `currency` (derived from PaymentOrderCurrency).
 */
export function orderPaymentCurrency(order: { payment_currency?: string; currency?: string }): string {
  return order.payment_currency || order.currency || 'CNY'
}

/**
 * Resolve the ledger currency from an order object.
 */
export function orderLedgerCurrency(order: { ledger_currency?: string }): string {
  return order.ledger_currency || 'USD'
}

/**
 * Format an amount with the correct currency symbol.
 * For zero-decimal currencies (KRW, VND, JPY), show no decimals.
 */
const ZERO_DECIMAL_CURRENCIES = ['KRW', 'VND', 'JPY']

export function formatCurrencyAmount(amount: number, currency?: string): string {
  const cur = (currency || 'CNY').toUpperCase()
  const symbol = currencySymbol(cur)
  if (ZERO_DECIMAL_CURRENCIES.includes(cur)) {
    return `${symbol}${Math.round(amount).toLocaleString()}`
  }
  return `${symbol}${amount.toFixed(2)}`
}
