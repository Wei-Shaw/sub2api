/**
 * 待续费看板的纯函数：分组、剩余时间人话化、告警口径计数。
 * 不依赖 Vue / i18n，方便单测。
 */
import type { ExpiringAccount } from '@/api/admin/opsTrace'

/**
 * 分组（按紧急程度排序）：
 * - expiredActive：已过期但仍是 active + 可调度 —— 会继续接流量但凭据/订阅已失效，最紧急
 * - expiredPaused：已过期且已暂停/禁用/出错 —— 不再接流量，仅提示
 * - within24h：未过期，剩余 ≤ 24 小时
 * - within3d：剩余 ≤ 72 小时
 * - within7d：剩余 ≤ 7 天
 * - later：剩余 > 7 天（仅在 30 天窗口下出现）
 */
export type RenewalGroupKey = 'expiredActive' | 'expiredPaused' | 'within24h' | 'within3d' | 'within7d' | 'later'

export const RENEWAL_GROUP_ORDER: RenewalGroupKey[] = [
  'expiredActive',
  'expiredPaused',
  'within24h',
  'within3d',
  'within7d',
  'later'
]

export const HOURS_24 = 24
export const HOURS_3D = 72
export const HOURS_7D = 168

/** 与后端 account_expiring_soon_count 告警窗口一致（accountExpiringSoonWindow = 72h） */
export const ALERT_WINDOW_HOURS = HOURS_3D

/** 与后端 Expired = hours_left <= 0 一致；两者任一为真即视为已过期 */
export function isExpired(a: Pick<ExpiringAccount, 'expired' | 'hours_left'>): boolean {
  return a.expired === true || !(a.hours_left > 0)
}

/** 仍在调度池里：status=active 且 schedulable */
export function isInScheduling(a: Pick<ExpiringAccount, 'status' | 'schedulable'>): boolean {
  return a.status === 'active' && a.schedulable === true
}

export function classifyRenewal(a: ExpiringAccount): RenewalGroupKey {
  if (isExpired(a)) {
    return isInScheduling(a) ? 'expiredActive' : 'expiredPaused'
  }
  const h = a.hours_left
  if (h <= HOURS_24) return 'within24h'
  if (h <= HOURS_3D) return 'within3d'
  if (h <= HOURS_7D) return 'within7d'
  return 'later'
}

export interface RenewalGroup {
  key: RenewalGroupKey
  items: ExpiringAccount[]
}

/** 分组；组内保持后端返回顺序（到期时间升序）。只返回非空组，按 RENEWAL_GROUP_ORDER 排序。 */
export function groupRenewals(items: ExpiringAccount[]): RenewalGroup[] {
  const buckets = new Map<RenewalGroupKey, ExpiringAccount[]>()
  for (const it of items) {
    if (!it) continue
    const key = classifyRenewal(it)
    const list = buckets.get(key)
    if (list) list.push(it)
    else buckets.set(key, [it])
  }
  return RENEWAL_GROUP_ORDER.filter((k) => buckets.has(k)).map((k) => ({ key: k, items: buckets.get(k)! }))
}

/** 各组数量（包含 0） */
export function countRenewalGroups(items: ExpiringAccount[]): Record<RenewalGroupKey, number> {
  const out = Object.fromEntries(RENEWAL_GROUP_ORDER.map((k) => [k, 0])) as Record<RenewalGroupKey, number>
  for (const it of items) {
    if (!it) continue
    out[classifyRenewal(it)] += 1
  }
  return out
}

/**
 * 与后端 countAccountsNeedingRenewal 同口径：72 小时内到期（未过期） + 已过期仍在调度。
 */
export function countAlertScope(items: ExpiringAccount[]): number {
  let n = 0
  for (const it of items) {
    if (!it) continue
    if (!isExpired(it)) {
      if (it.hours_left <= ALERT_WINDOW_HOURS) n++
      continue
    }
    if (isInScheduling(it)) n++
  }
  return n
}

export type RemainingUnit = 'minutes' | 'hours' | 'days'

export interface RemainingParts {
  expired: boolean
  unit: RemainingUnit
  value: number
}

/**
 * 剩余/已过期时长拆成「数值 + 单位」，由调用方套 i18n 文案。
 * < 1 小时用分钟（最少 1），< 48 小时用小时，其余用天；均向下取整。
 */
export function describeRemaining(hoursLeft: number, expired?: boolean): RemainingParts {
  const isExp = expired === true || !(hoursLeft > 0)
  const abs = Math.abs(Number.isFinite(hoursLeft) ? hoursLeft : 0)
  if (abs < 1) {
    return { expired: isExp, unit: 'minutes', value: Math.max(1, Math.floor(abs * 60)) }
  }
  if (abs < 48) {
    return { expired: isExp, unit: 'hours', value: Math.floor(abs) }
  }
  return { expired: isExp, unit: 'days', value: Math.floor(abs / 24) }
}
