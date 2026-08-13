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

/**
 * Order status → semantic tone, for `StatusDot` / `Badge`.
 *
 * `accent` is absent on purpose: in this design system the accent means
 * INTERACTIVE OR SELECTED and must never signal state, or a selected row and a
 * paid order become indistinguishable.
 *
 * Everything that is simply *over* — expired, cancelled, refunded — stays
 * neutral. A list where every row carries a colour has no signal left for the
 * rows that actually need attention, and a completed refund needs none.
 */
const STATUS_TONE_MAP: Record<string, OrderDisplayTone> = {
  PENDING: 'warn',
  PAID: 'info',
  RECHARGING: 'info',
  COMPLETED: 'success',
  EXPIRED: 'neutral',
  CANCELLED: 'neutral',
  FAILED: 'danger',
  REFUND_REQUESTED: 'warn',
  REFUNDING: 'warn',
  REFUND_PENDING: 'warn',
  PARTIALLY_REFUNDED: 'neutral',
  REFUNDED: 'neutral',
  REFUND_FAILED: 'danger',
}

const STATUS_I18N_KEY_MAP: Record<string, string> = {
  PENDING: 'payment.status.pending',
  PAID: 'payment.status.paid',
  RECHARGING: 'payment.status.recharging',
  COMPLETED: 'payment.status.completed',
  EXPIRED: 'payment.status.expired',
  CANCELLED: 'payment.status.cancelled',
  FAILED: 'payment.status.failed',
  REFUND_REQUESTED: 'payment.status.refund_requested',
  REFUNDING: 'payment.status.refunding',
  REFUND_PENDING: 'payment.status.refund_pending',
  PARTIALLY_REFUNDED: 'payment.status.partially_refunded',
  REFUNDED: 'payment.status.refunded',
  REFUND_FAILED: 'payment.status.refund_failed',
}

/** Mirrors `Tone` in `components/common/primitives.ts`, minus `accent`. */
export type OrderDisplayTone = 'neutral' | 'success' | 'warn' | 'danger' | 'info'

export function orderStatusTone(status: string): OrderDisplayTone {
  return STATUS_TONE_MAP[String(status || '').trim().toUpperCase()] ?? 'neutral'
}

/**
 * Returns `''` for an unmapped status so the caller can print the raw value.
 * A literal `PARTIALLY_SETTLED` from a newer backend is more diagnostic than
 * the word "unknown".
 */
export function orderStatusI18nKey(status: string): string {
  return STATUS_I18N_KEY_MAP[String(status || '').trim().toUpperCase()] ?? ''
}

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
