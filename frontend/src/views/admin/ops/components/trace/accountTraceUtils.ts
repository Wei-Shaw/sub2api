/**
 * 账号追溯面板的纯函数：统计、台账变化解析、指纹配色。
 * 不依赖组件实例，便于单测。
 */
import type { AccountLifecycleEvent, TraceAttempt, TraceFailureKind } from '@/api/admin/opsTrace'

/** 面板展示的失败类型（固定顺序） */
export const FAILURE_KINDS: Exclude<TraceFailureKind, 'none'>[] = ['auth', 'rate_limit', 'overload', 'transient', 'client']

/** 固定配色：auth 红、rate_limit 橙、overload/transient 黄、client 灰 */
export const FAILURE_KIND_STYLES: Record<string, { bar: string; badge: string }> = {
  auth: {
    bar: 'bg-red-500',
    badge: 'bg-red-100 text-red-700 dark:bg-red-500/20 dark:text-red-300'
  },
  rate_limit: {
    bar: 'bg-orange-500',
    badge: 'bg-orange-100 text-orange-700 dark:bg-orange-500/20 dark:text-orange-300'
  },
  overload: {
    bar: 'bg-yellow-400',
    badge: 'bg-yellow-100 text-yellow-800 dark:bg-yellow-500/20 dark:text-yellow-300'
  },
  transient: {
    bar: 'bg-yellow-400',
    badge: 'bg-yellow-100 text-yellow-800 dark:bg-yellow-500/20 dark:text-yellow-300'
  },
  client: {
    bar: 'bg-gray-400',
    badge: 'bg-gray-100 text-gray-700 dark:bg-dark-600 dark:text-gray-300'
  }
}

export function failureKindStyle(kind: string) {
  return FAILURE_KIND_STYLES[kind] ?? FAILURE_KIND_STYLES.client
}

export function isFailure(a: TraceAttempt): boolean {
  return !!a.failure_kind && a.failure_kind !== 'none'
}

export interface FailureSummary {
  total: number
  byKind: Record<string, number>
  lastFailureAt: string | null
  fingerprints: string[]
}

/** 统计时间窗内的失败分布；指纹个数统计所有尝试（含成功采样）里出现过的凭据。 */
export function summarizeAttempts(attempts: TraceAttempt[]): FailureSummary {
  const byKind: Record<string, number> = Object.fromEntries(FAILURE_KINDS.map((k) => [k, 0]))
  let total = 0
  let lastMs = -Infinity
  let lastFailureAt: string | null = null
  const fps = new Set<string>()
  for (const a of attempts) {
    if (a.credential_fp) fps.add(a.credential_fp)
    if (!isFailure(a)) continue
    total++
    byKind[a.failure_kind] = (byKind[a.failure_kind] ?? 0) + 1
    const ms = new Date(a.created_at).getTime()
    if (!Number.isNaN(ms) && ms > lastMs) {
      lastMs = ms
      lastFailureAt = a.created_at
    }
  }
  return { total, byKind, lastFailureAt, fingerprints: [...fps] }
}

/** 每个指纹在窗口内的失败次数 */
export function failuresByFingerprint(attempts: TraceAttempt[]): Record<string, number> {
  const out: Record<string, number> = {}
  for (const a of attempts) {
    if (!isFailure(a) || !a.credential_fp) continue
    out[a.credential_fp] = (out[a.credential_fp] ?? 0) + 1
  }
  return out
}

// ---------------------------------------------------------------------------
// 指纹配色：同一指纹在概览与台账里是同一颜色，便于肉眼对上
// ---------------------------------------------------------------------------

const FP_PALETTE = [
  'bg-sky-100 text-sky-800 ring-sky-300 dark:bg-sky-500/20 dark:text-sky-200 dark:ring-sky-500/40',
  'bg-violet-100 text-violet-800 ring-violet-300 dark:bg-violet-500/20 dark:text-violet-200 dark:ring-violet-500/40',
  'bg-emerald-100 text-emerald-800 ring-emerald-300 dark:bg-emerald-500/20 dark:text-emerald-200 dark:ring-emerald-500/40',
  'bg-pink-100 text-pink-800 ring-pink-300 dark:bg-pink-500/20 dark:text-pink-200 dark:ring-pink-500/40',
  'bg-teal-100 text-teal-800 ring-teal-300 dark:bg-teal-500/20 dark:text-teal-200 dark:ring-teal-500/40',
  'bg-indigo-100 text-indigo-800 ring-indigo-300 dark:bg-indigo-500/20 dark:text-indigo-200 dark:ring-indigo-500/40'
]

export function fingerprintClass(fp: string): string {
  let h = 0
  for (let i = 0; i < fp.length; i++) h = (h * 31 + fp.charCodeAt(i)) >>> 0
  return FP_PALETTE[h % FP_PALETTE.length]
}

// ---------------------------------------------------------------------------
// 台账 changes 解析
// ---------------------------------------------------------------------------

export interface ChangeRow {
  field: string
  /** created 快照没有 from */
  isSnapshot: boolean
  from: unknown
  to: unknown
}

function isFromTo(v: unknown): v is { from?: unknown; to?: unknown } {
  return !!v && typeof v === 'object' && !Array.isArray(v) && ('from' in (v as object) || 'to' in (v as object))
}

/** 把 changes 拆成行。created 事件是快照（字段: 值），其它是 {from,to}。 */
export function parseChanges(ev: AccountLifecycleEvent): ChangeRow[] {
  const raw = ev.changes
  if (!raw || typeof raw !== 'object') return []
  const rows: ChangeRow[] = []
  for (const [field, v] of Object.entries(raw)) {
    if (ev.event_type !== 'created' && isFromTo(v)) {
      rows.push({ field, isSnapshot: false, from: v.from ?? null, to: v.to ?? null })
    } else if (field === 'plan' && v && typeof v === 'object' && !Array.isArray(v)) {
      // created 快照里的套餐对象拍平成 extra.* 行
      for (const [k, pv] of Object.entries(v as Record<string, unknown>)) {
        rows.push({ field: `extra.${k}`, isSnapshot: true, from: null, to: pv })
      }
    } else {
      rows.push({ field, isSnapshot: true, from: null, to: v })
    }
  }
  return rows
}

const DATE_FIELDS = new Set(['expires_at', 'deleted_at'])

export function isDateField(field: string): boolean {
  return DATE_FIELDS.has(field) || /(_at|_until|expires|reset_at)$/.test(field)
}

/** 两个时间的差（天，保留 1 位小数）；任一缺失返回 null */
export function daysBetween(from: unknown, to: unknown): number | null {
  if (typeof from !== 'string' || typeof to !== 'string' || !from || !to) return null
  const a = new Date(from).getTime()
  const b = new Date(to).getTime()
  if (Number.isNaN(a) || Number.isNaN(b)) return null
  return Math.round(((b - a) / 86400000) * 10) / 10
}

/** 到期时间变化的天数：优先用 changes.expires_at，退回 expires_at_before/after */
export function expiryDeltaDays(ev: AccountLifecycleEvent): number | null {
  const c = ev.changes?.expires_at
  if (isFromTo(c)) {
    const d = daysBetween(c.from, c.to)
    if (d !== null) return d
  }
  return daysBetween(ev.expires_at_before, ev.expires_at_after)
}

/** 本条事件是否改了凭据指纹 */
export function credentialChanged(ev: AccountLifecycleEvent): boolean {
  if (ev.event_type === 'created' || ev.event_type === 'deleted') return false
  const before = ev.credential_fp_before ?? null
  const after = ev.credential_fp_after ?? null
  if (before !== after) return true
  return isFromTo(ev.changes?.credential_fp)
}

export function credentialFpPair(ev: AccountLifecycleEvent): { from: string | null; to: string | null } {
  const c = ev.changes?.credential_fp
  if (isFromTo(c)) {
    return { from: (c.from as string | null) ?? null, to: (c.to as string | null) ?? null }
  }
  return { from: ev.credential_fp_before ?? null, to: ev.credential_fp_after ?? null }
}

export const KNOWN_EVENT_TYPES = [
  'created', 'deleted', 'restored', 'expired', 'renewed', 'expiry_shortened', 'expiry_set', 'expiry_cleared',
  'plan_changed', 'quota_changed', 'disabled', 'errored', 'recovered', 'enabled', 'credential_rotated',
  'token_refreshed', 'unschedulable', 'schedulable', 'updated'
] as const

/** 事件类型的语义色（时间线圆点） */
export function eventTone(type: string): string {
  switch (type) {
    case 'created':
    case 'renewed':
    case 'enabled':
    case 'recovered':
    case 'restored':
    case 'schedulable':
      return 'bg-emerald-500'
    case 'expired':
    case 'errored':
    case 'deleted':
      return 'bg-red-500'
    case 'disabled':
    case 'unschedulable':
    case 'expiry_shortened':
      return 'bg-orange-500'
    case 'credential_rotated':
      return 'bg-amber-500'
    case 'plan_changed':
    case 'quota_changed':
    case 'expiry_set':
    case 'expiry_cleared':
      return 'bg-blue-500'
    default:
      return 'bg-gray-400'
  }
}

/** 时长（ms → 「x 小时 y 分」的 i18n 参数用） */
export function durationMinutes(from: string, to?: string | null): number | null {
  if (!to) return null
  const a = new Date(from).getTime()
  const b = new Date(to).getTime()
  if (Number.isNaN(a) || Number.isNaN(b) || b < a) return null
  return Math.round((b - a) / 60000)
}
