/**
 * AnnouncementBell 的 Tab 选择策略（纯函数，方便单测）。
 *
 * 决策规则（对齐 openspec tasks §8.4）：
 *
 *  1. 工单功能关闭时（supportTicketEnabled=false）：无条件返回 'announcement'。
 *     铃铛面板此时也不显示工单 Tab bar，是纯公告 UI。
 *
 *  2. 两个 Tab 都没有未读：默认 'announcement'（用户对系统级公告有天然预期）。
 *
 *  3. 只有工单侧有未读：默认 'ticket'（把用户注意力直接引到有内容的一侧）。
 *
 *  4. 两个 Tab 都有未读：默认 'announcement'——公告更常见"系统必读"性质，
 *     工单未读在 badge/红点已经显式提示，用户可以再手动切过去。
 *
 *  5. 只有公告有未读：默认 'announcement'（等同规则 4 的降级情况）。
 *
 * 该函数不读 store，也不 side-effect，方便在 vitest 里直接断言组合。
 */
export type BellTab = 'announcement' | 'ticket'

export interface BellTabInputs {
  /** 后端 support_ticket_enabled 开关。 */
  supportTicketEnabled: boolean
  /** 公告未读数。 */
  announcementUnread: number
  /**
   * 工单未读总和（notifications 里未读条目 + 未读工单数聚合，任意一者 > 0
   * 都视为"工单侧有内容需要关注"）。调用方传入前做好求和/max。
   */
  ticketUnread: number
}

export function pickDefaultBellTab(input: BellTabInputs): BellTab {
  if (!input.supportTicketEnabled) return 'announcement'
  if (input.announcementUnread === 0 && input.ticketUnread > 0) return 'ticket'
  return 'announcement'
}
