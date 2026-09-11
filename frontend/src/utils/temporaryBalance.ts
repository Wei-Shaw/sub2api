/**
 * Temporary balance grants are usable only when they have a positive amount
 * and an expiry in the future.  Keep this rule in one place so that the
 * dashboard, profile, and admin table never accidentally display an expired
 * grant as spendable balance.
 */

export interface TemporaryBalanceFields {
  temporary_balance?: number | null
  temporary_balance_expires_at?: string | null
}

export type TemporaryBalanceStatus = 'active' | 'expired' | 'none'

export function isTemporaryBalanceExpired(
  value: TemporaryBalanceFields,
  now: Date = new Date()
): boolean {
  const amount = Number(value.temporary_balance ?? 0)
  const expiry = value.temporary_balance_expires_at
  if (!(amount > 0) || !expiry) return false
  const expiresAt = Date.parse(expiry)
  return Number.isFinite(expiresAt) && expiresAt <= now.getTime()
}

export function getActiveTemporaryBalance(
  value: TemporaryBalanceFields,
  now: Date = new Date()
): number {
  const amount = Number(value.temporary_balance ?? 0)
  if (!(amount > 0) || !value.temporary_balance_expires_at) return 0
  const expiresAt = Date.parse(value.temporary_balance_expires_at)
  if (!Number.isFinite(expiresAt) || expiresAt <= now.getTime()) return 0
  return amount
}

export function getTemporaryBalanceStatus(
  value: TemporaryBalanceFields,
  now: Date = new Date()
): TemporaryBalanceStatus {
  if (getActiveTemporaryBalance(value, now) > 0) return 'active'
  return isTemporaryBalanceExpired(value, now) ? 'expired' : 'none'
}
