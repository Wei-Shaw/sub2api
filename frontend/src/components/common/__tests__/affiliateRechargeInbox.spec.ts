/**
 * affiliateRechargeInbox 纯函数单测：被邀请人充值通知 → 未读 invitee 集合 / 最新 seq / 置顶排序。
 */
import { describe, expect, it } from 'vitest'
import type { InboxMessage } from '@/api/inbox'
import {
  unreadRechargeInviteeIDs,
  latestRechargeSeq,
  sortInviteesByRecharge,
  AFFILIATE_RECHARGE_NAMESPACE,
} from '../affiliateRechargeInbox'

function msg(seq: number, namespace: string, payload: Record<string, unknown> = {}): InboxMessage {
  return { seq, scope: 'direct', namespace, payload, created_at: '2026-07-19T00:00:00Z' }
}

describe('unreadRechargeInviteeIDs', () => {
  it('仅收集 affiliate_recharge 且 seq > 水位的 invitee_id', () => {
    const messages = [
      msg(10, AFFILIATE_RECHARGE_NAMESPACE, { invitee_id: 101 }),
      msg(11, AFFILIATE_RECHARGE_NAMESPACE, { invitee_id: 102 }),
      msg(5, AFFILIATE_RECHARGE_NAMESPACE, { invitee_id: 999 }), // 已读(<=水位)
      msg(12, 'support_ticket', { invitee_id: 103 }), // 其它 namespace 不计
    ]
    const ids = unreadRechargeInviteeIDs(messages, 5)
    expect([...ids].sort()).toEqual([101, 102])
  })

  it('无效 invitee_id 忽略', () => {
    const messages = [
      msg(10, AFFILIATE_RECHARGE_NAMESPACE, {}),
      msg(11, AFFILIATE_RECHARGE_NAMESPACE, { invitee_id: 0 }),
      msg(12, AFFILIATE_RECHARGE_NAMESPACE, { invitee_id: 'x' }),
    ]
    expect(unreadRechargeInviteeIDs(messages, 0).size).toBe(0)
  })
})

describe('latestRechargeSeq', () => {
  it('返回该 namespace 最高 seq', () => {
    const messages = [
      msg(10, AFFILIATE_RECHARGE_NAMESPACE, { invitee_id: 1 }),
      msg(20, AFFILIATE_RECHARGE_NAMESPACE, { invitee_id: 2 }),
      msg(99, 'support_ticket', {}), // 不计入
    ]
    expect(latestRechargeSeq(messages)).toBe(20)
  })
  it('无消息返回 0', () => {
    expect(latestRechargeSeq([])).toBe(0)
  })
})

describe('sortInviteesByRecharge', () => {
  it('有新充值的稳定置顶，其余保持原顺序', () => {
    const invitees = [
      { user_id: 1 },
      { user_id: 2 },
      { user_id: 3 },
      { user_id: 4 },
    ]
    const out = sortInviteesByRecharge(invitees, new Set([3, 1]))
    // 置顶组保持原相对顺序 [1,3]，其余保持 [2,4]
    expect(out.map((x) => x.user_id)).toEqual([1, 3, 2, 4])
  })
  it('空集合返回原顺序副本', () => {
    const invitees = [{ user_id: 1 }, { user_id: 2 }]
    const out = sortInviteesByRecharge(invitees, new Set())
    expect(out.map((x) => x.user_id)).toEqual([1, 2])
    expect(out).not.toBe(invitees) // 新数组
  })
})
