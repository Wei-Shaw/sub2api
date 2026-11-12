import { describe, expect, it } from 'vitest'
import type { ExpiringAccount } from '@/api/admin/opsTrace'
import {
  classifyRenewal,
  countAlertScope,
  countRenewalGroups,
  describeRemaining,
  groupRenewals
} from '../renewalGroups'

let seq = 0
function acc(partial: Partial<ExpiringAccount>): ExpiringAccount {
  seq += 1
  const hours = partial.hours_left ?? 10
  return {
    id: seq,
    name: `acc-${seq}`,
    platform: 'anthropic',
    type: 'apikey',
    status: 'active',
    schedulable: true,
    expires_at: '2026-11-12T00:00:00Z',
    auto_pause_on_expired: true,
    expired: hours <= 0,
    hours_left: hours,
    ...partial
  }
}

describe('classifyRenewal', () => {
  it('刚好 24 小时归入 24 小时内，略超归入 3 天内', () => {
    expect(classifyRenewal(acc({ hours_left: 24 }))).toBe('within24h')
    expect(classifyRenewal(acc({ hours_left: 24.1 }))).toBe('within3d')
    expect(classifyRenewal(acc({ hours_left: 0.1 }))).toBe('within24h')
  })

  it('刚好 72 小时 / 168 小时 的边界', () => {
    expect(classifyRenewal(acc({ hours_left: 72 }))).toBe('within3d')
    expect(classifyRenewal(acc({ hours_left: 72.1 }))).toBe('within7d')
    expect(classifyRenewal(acc({ hours_left: 168 }))).toBe('within7d')
    expect(classifyRenewal(acc({ hours_left: 168.1 }))).toBe('later')
  })

  it('刚好过期（hours_left=0）且仍在调度 → 已过期仍在调度', () => {
    expect(classifyRenewal(acc({ hours_left: 0, expired: true }))).toBe('expiredActive')
    // 即使 expired 标记缺失，hours_left<=0 也视为过期（与后端 left<=0 口径一致）
    expect(classifyRenewal(acc({ hours_left: 0, expired: false }))).toBe('expiredActive')
  })

  it('已过期但已暂停（schedulable=false）→ 已过期已暂停', () => {
    expect(classifyRenewal(acc({ hours_left: -30, schedulable: false }))).toBe('expiredPaused')
  })

  it('已过期且已禁用 / 出错 → 已过期已暂停（不在调度池）', () => {
    expect(classifyRenewal(acc({ hours_left: -2, status: 'disabled', schedulable: true }))).toBe('expiredPaused')
    expect(classifyRenewal(acc({ hours_left: -2, status: 'error', schedulable: true }))).toBe('expiredPaused')
  })

  it('未过期但已禁用的账号仍按剩余时间分组', () => {
    expect(classifyRenewal(acc({ hours_left: 5, status: 'disabled', schedulable: false }))).toBe('within24h')
  })
})

describe('groupRenewals / countRenewalGroups', () => {
  it('按紧急程度排序、只保留非空组、组内保持原顺序', () => {
    const a = acc({ hours_left: 50 })
    const b = acc({ hours_left: -1 })
    const c = acc({ hours_left: -5, schedulable: false })
    const d = acc({ hours_left: 60 })
    const groups = groupRenewals([c, b, a, d])
    expect(groups.map((g) => g.key)).toEqual(['expiredActive', 'expiredPaused', 'within3d'])
    expect(groups[2].items.map((x) => x.id)).toEqual([a.id, d.id])

    const counts = countRenewalGroups([c, b, a, d])
    expect(counts).toEqual({
      expiredActive: 1,
      expiredPaused: 1,
      within24h: 0,
      within3d: 2,
      within7d: 0,
      later: 0
    })
  })

  it('空列表', () => {
    expect(groupRenewals([])).toEqual([])
    expect(countAlertScope([])).toBe(0)
  })
})

describe('countAlertScope（与后端 account_expiring_soon_count 同口径）', () => {
  it('72h 内未过期 + 已过期仍在调度；排除已暂停过期与 72h 以外', () => {
    const items = [
      acc({ hours_left: 1 }), // ✓
      acc({ hours_left: 72 }), // ✓ 边界
      acc({ hours_left: 72.5 }), // ✗
      acc({ hours_left: 100 }), // ✗
      acc({ hours_left: -3 }), // ✓ 过期仍调度
      acc({ hours_left: -3, schedulable: false }), // ✗
      acc({ hours_left: -3, status: 'disabled' }), // ✗
      acc({ hours_left: 10, status: 'disabled', schedulable: false }) // ✓ 未过期均计入（后端 ExpiresAt.After(now) 即计数）
    ]
    expect(countAlertScope(items)).toBe(4)
  })
})

describe('describeRemaining', () => {
  it('小时 / 天 / 分钟', () => {
    expect(describeRemaining(5.9)).toEqual({ expired: false, unit: 'hours', value: 5 })
    expect(describeRemaining(47.9)).toEqual({ expired: false, unit: 'hours', value: 47 })
    expect(describeRemaining(48)).toEqual({ expired: false, unit: 'days', value: 2 })
    expect(describeRemaining(0.5)).toEqual({ expired: false, unit: 'minutes', value: 30 })
  })

  it('已过期', () => {
    expect(describeRemaining(-49)).toEqual({ expired: true, unit: 'days', value: 2 })
    expect(describeRemaining(-2.4)).toEqual({ expired: true, unit: 'hours', value: 2 })
    expect(describeRemaining(0)).toEqual({ expired: true, unit: 'minutes', value: 1 })
  })
})
