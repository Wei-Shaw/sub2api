/**
 * affiliateRechargeInbox.ts —— "邀请返利列表"消费通用信箱(inbox)里"被邀请人充值"通知的
 * 纯数据逻辑。
 *
 * 后端在被邀请人充值并产生返利后，向邀请人单播一条 namespace=affiliate_recharge 的消息，
 * payload 含 { invitee_id, amount, order_id }。前端据此：
 *   - 把"有新充值(未读)"的被邀请人在列表里置顶；
 *   - 在对应列表项显示红色向上箭头；
 *   - 邀请人查看（离开返利页）后把"已查看水位"推进到最新 seq，标记恢复。
 *
 * 未读判定：seq > sinceSeq 即未读。sinceSeq 由调用方传入"页面级独立已查看水位"
 * （持久化在 localStorage），而非 inbox 的 localAckSeq——后者是投递水位，消息到达后
 * 会被自动 ack 推进，用它判红点会瞬间清空。抽成纯函数便于单测。
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
  sinceSeq: number
): Set<number> {
  const ids = new Set<number>()
  for (const m of messages) {
    if (m.namespace !== AFFILIATE_RECHARGE_NAMESPACE) continue
    if (m.seq <= sinceSeq) continue
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
 * sumUnreadRebate 汇总"未读充值通知"对应的返利额，用于红字展示"本次新增返利"。
 *
 * 直接来自消息 payload（不依赖页面 reload 前后差值）——这样即使在**新开网页**、
 * 返利早已入账的场景下（catchup 拉到未读消息时），也能正确展示本次新增。
 * 优先用 payload.rebate（后端下发的实际返利额）；旧消息无该字段时按 amount × 比例兜底估算。
 */
export function sumUnreadRebate(
  messages: InboxMessage[],
  sinceSeq: number,
  ratePercent: number
): number {
  let total = 0
  for (const m of messages) {
    if (m.namespace !== AFFILIATE_RECHARGE_NAMESPACE) continue
    if (m.seq <= sinceSeq) continue
    const p = (m.payload as Record<string, unknown> | undefined) ?? {}
    const rebate = Number(p.rebate)
    if (Number.isFinite(rebate) && rebate > 0) {
      total += rebate
      continue
    }
    const amount = Number(p.amount)
    if (Number.isFinite(amount) && amount > 0 && ratePercent > 0) {
      total += (amount * ratePercent) / 100
    }
  }
  return total
}

/**
 * unreadRebateByInvitee 按被邀请人拆分未读充值通知对应的返利额，返回 Map<inviteeID, 返利额>。
 *
 * 用于在列表里为每个"有新增返利"的被邀请人展示其单人本次新增返利（可能多笔累加）。
 * 与 sumUnreadRebate 同源：优先 payload.rebate，旧消息按 amount × 比例兜底。
 */
export function unreadRebateByInvitee(
  messages: InboxMessage[],
  sinceSeq: number,
  ratePercent: number
): Map<number, number> {
  const map = new Map<number, number>()
  for (const m of messages) {
    if (m.namespace !== AFFILIATE_RECHARGE_NAMESPACE) continue
    if (m.seq <= sinceSeq) continue
    const p = (m.payload as Record<string, unknown> | undefined) ?? {}
    const inviteeID = Number(p.invitee_id)
    if (!Number.isFinite(inviteeID) || inviteeID <= 0) continue
    let value = 0
    const rebate = Number(p.rebate)
    if (Number.isFinite(rebate) && rebate > 0) {
      value = rebate
    } else {
      const amount = Number(p.amount)
      if (Number.isFinite(amount) && amount > 0 && ratePercent > 0) {
        value = (amount * ratePercent) / 100
      }
    }
    if (value > 0) {
      map.set(inviteeID, (map.get(inviteeID) ?? 0) + value)
    }
  }
  return map
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
