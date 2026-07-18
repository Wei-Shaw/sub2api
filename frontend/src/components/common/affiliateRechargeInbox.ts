/**
 * affiliateRechargeInbox.ts —— "邀请返利列表"消费通用信箱(inbox)里"被邀请人充值"通知的
 * 纯数据逻辑。
 *
 * 后端在被邀请人充值并产生返利后，向邀请人单播一条 namespace=affiliate_recharge 的消息，
 * payload 含 { invitee_id, amount, order_id }。前端据此：
 *   - 把"有新充值(未读)"的被邀请人在列表里置顶；
 *   - 在对应列表项显示红色向上箭头；
 *   - 邀请人查看后 ack 到最新 seq，标记恢复。
 *
 * 未读判定沿用 inbox 累积 ack 语义：seq > localAckSeq 即未读。抽成纯函数便于单测。
 */
import type { InboxMessage } from '@/api/inbox'

/** 被邀请人充值事件在通用信箱中的命名空间（与后端 AffiliateRechargeInboxNamespace 对齐）。 */
export const AFFILIATE_RECHARGE_NAMESPACE = 'affiliate_recharge'

/**
 * unreadRechargeInviteeIDs 返回"有未读充值通知"的被邀请人 id 集合。
 * 用于在邀请返利列表里判断某行是否置顶 / 是否显示红色向上箭头。
 */
export function unreadRechargeInviteeIDs(
  messages: InboxMessage[],
  localAckSeq: number
): Set<number> {
  const ids = new Set<number>()
  for (const m of messages) {
    if (m.namespace !== AFFILIATE_RECHARGE_NAMESPACE) continue
    if (m.seq <= localAckSeq) continue
    const inviteeID = Number((m.payload as Record<string, unknown> | undefined)?.invitee_id)
    if (Number.isFinite(inviteeID) && inviteeID > 0) {
      ids.add(inviteeID)
    }
  }
  return ids
}

/**
 * latestRechargeSeq 返回 affiliate_recharge 消息里的最高 seq（无则 0）。
 * 邀请人查看列表后据此 ack（markReadUpTo），把所有充值通知标记为已读。
 */
export function latestRechargeSeq(messages: InboxMessage[]): number {
  let max = 0
  for (const m of messages) {
    if (m.namespace === AFFILIATE_RECHARGE_NAMESPACE && m.seq > max) {
      max = m.seq
    }
  }
  return max
}

/**
 * sortInviteesByRecharge 把"有新充值"的被邀请人稳定置顶（其余保持原有顺序）。
 * 泛型约束：列表项需含数值型 user_id。返回新数组，不改原数组。
 */
export function sortInviteesByRecharge<T extends { user_id: number }>(
  invitees: T[],
  unreadIDs: Set<number>
): T[] {
  if (unreadIDs.size === 0) return invitees.slice()
  // 稳定排序：有新充值的排前，组内保持原顺序。
  return invitees
    .map((item, index) => ({ item, index }))
    .sort((a, b) => {
      const ap = unreadIDs.has(a.item.user_id) ? 0 : 1
      const bp = unreadIDs.has(b.item.user_id) ? 0 : 1
      if (ap !== bp) return ap - bp
      return a.index - b.index
    })
    .map((x) => x.item)
}
