/**
 * Shared utility functions for payment order display.
 * Used by AdminOrderDetail, AdminOrderTable, AdminOrdersView, etc.
 */

const STATUS_BADGE_MAP: Record<string, string> = {
  PENDING: 'badge-warning',
  PAID: 'badge-info',
  RECHARGING: 'badge-info',
  COMPLETED: 'badge-success',
  EXPIRED: 'badge-secondary',
  CANCELLED: 'badge-secondary',
  FAILED: 'badge-danger',
  // Legacy refund statuses: nothing produces them any more, but historical
  // orders still carry them and must not render as an unknown status.
  REFUND_REQUESTED: 'badge-warning',
  REFUNDING: 'badge-warning',
  REFUND_PENDING: 'badge-warning',
  PARTIALLY_REFUNDED: 'badge-warning',
  REFUNDED: 'badge-info',
  REFUND_FAILED: 'badge-danger',
}

export function statusBadgeClass(status: string): string {
  return STATUS_BADGE_MAP[status] || 'badge-secondary'
}

export function formatOrderDateTime(dateStr: string): string {
  if (!dateStr) return '-'
  return new Date(dateStr).toLocaleString()
}
