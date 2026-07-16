/**
 * pickDefaultBellTab 纯函数单测：覆盖 tasks §8.8 的 4 条默认打开策略。
 */
import { describe, expect, it } from 'vitest'
import { pickDefaultBellTab } from '../announcementBellTab'

describe('pickDefaultBellTab', () => {
  it('工单功能关闭时无条件返回 announcement（哪怕工单侧有未读也不会命中）', () => {
    expect(
      pickDefaultBellTab({
        supportTicketEnabled: false,
        announcementUnread: 0,
        ticketUnread: 10,
      })
    ).toBe('announcement')
  })

  it('两个 Tab 都空时默认 announcement', () => {
    expect(
      pickDefaultBellTab({
        supportTicketEnabled: true,
        announcementUnread: 0,
        ticketUnread: 0,
      })
    ).toBe('announcement')
  })

  it('只有工单侧有未读时默认 ticket', () => {
    expect(
      pickDefaultBellTab({
        supportTicketEnabled: true,
        announcementUnread: 0,
        ticketUnread: 3,
      })
    ).toBe('ticket')
  })

  it('两个 Tab 都有未读时默认 announcement（公告优先级高）', () => {
    expect(
      pickDefaultBellTab({
        supportTicketEnabled: true,
        announcementUnread: 2,
        ticketUnread: 5,
      })
    ).toBe('announcement')
  })

  it('只有公告有未读时默认 announcement', () => {
    expect(
      pickDefaultBellTab({
        supportTicketEnabled: true,
        announcementUnread: 4,
        ticketUnread: 0,
      })
    ).toBe('announcement')
  })
})
