/**
 * SupportTicketDetailView 单测
 *
 * 覆盖 §13.3 验收点：
 *   - 回复时间线渲染（admin / user 两种条目）
 *   - status === 'closed' 时输入框替换为只读提示卡片，"关闭工单" 按钮消失
 *   - admin 回复显示为 "客服"（即 i18n key `support.common.authorAdmin`）
 *   - 调用 appendReply 后会重新拉取详情
 */
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { defineComponent, h } from 'vue'
import { flushPromises, mount } from '@vue/test-utils'

import SupportTicketDetailView from '../SupportTicketDetailView.vue'
import type { SupportTicketWithReplies } from '@/api/support'

const {
  getMyTicket,
  appendReply,
  closeTicket,
  push,
  showSuccess,
  showError,
} = vi.hoisted(() => ({
  getMyTicket: vi.fn(),
  appendReply: vi.fn(),
  closeTicket: vi.fn(),
  push: vi.fn(),
  showSuccess: vi.fn(),
  showError: vi.fn(),
}))

vi.mock('@/api/support', () => ({
  getMyTicket,
  appendReply,
  closeTicket,
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({ showSuccess, showError }),
}))

vi.mock('vue-router', () => ({
  useRouter: () => ({ push }),
  useRoute: () => ({ params: { id: '7' } }),
}))

vi.mock('@/utils/apiError', () => ({
  extractI18nErrorMessage: (_err: unknown, _t: unknown, _ns: string, fallback: string) =>
    fallback,
}))

vi.mock('@/utils/format', () => ({
  formatDateTime: (s: string | null | undefined) => s ?? '',
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({ t: (key: string) => key }),
  }
})

const BaseDialogStub = defineComponent({
  props: { show: { type: Boolean, default: false } },
  setup(_, { slots }) {
    return () => (slots.default ? h('div', { 'data-test': 'dialog' }, [slots.default()]) : null)
  },
})

const stubs = {
  AppLayout: { template: '<div><slot /></div>' },
  Icon: true,
  BaseDialog: BaseDialogStub,
  SupportStatusBadge: { props: ['status'], template: '<span>{{ status }}</span>' },
  SupportPriorityBadge: { props: ['priority'], template: '<span>{{ priority }}</span>' },
}

const baseTicket = (overrides: Partial<SupportTicketWithReplies> = {}): SupportTicketWithReplies => ({
  id: 7,
  user_id: 1,
  title: 'Login fails',
  content: 'Cannot login',
  category: '账号',
  status: 'open',
  priority: 'normal',
  closed_at: null,
  created_at: '2026-06-01',
  updated_at: '2026-06-01',
  chat_context: null,
  replies: [],
  ...overrides,
})

beforeEach(() => {
  getMyTicket.mockReset()
  appendReply.mockReset()
  closeTicket.mockReset()
  push.mockReset()
  showSuccess.mockReset()
  showError.mockReset()
})

describe('SupportTicketDetailView', () => {
  it('renders reply timeline with admin and user authors distinguished', async () => {
    getMyTicket.mockResolvedValue(
      baseTicket({
        replies: [
          { id: 1, ticket_id: 7, author_id: 99, is_admin: true, content: 'Try clearing cookies.', created_at: '2026-06-02' },
          { id: 2, ticket_id: 7, author_id: 1, is_admin: false, content: 'Done, still fails.', created_at: '2026-06-03' },
        ],
      }),
    )

    const wrapper = mount(SupportTicketDetailView, { global: { stubs } })
    await flushPromises()

    const items = wrapper.findAll('ul li')
    expect(items).toHaveLength(2)
    // admin 回复展示为 "客服"
    expect(items[0].text()).toContain('support.common.authorAdmin')
    expect(items[0].classes()).toContain('flex-row')
    // user 回复展示为 "用户" 并右对齐
    expect(items[1].text()).toContain('support.common.authorUser')
    expect(items[1].classes()).toContain('flex-row-reverse')
  })

  it('hides reply textarea + close button when ticket status is closed', async () => {
    getMyTicket.mockResolvedValue(
      baseTicket({
        status: 'closed',
        closed_at: '2026-06-05',
        replies: [],
      }),
    )

    const wrapper = mount(SupportTicketDetailView, { global: { stubs } })
    await flushPromises()

    expect(wrapper.find('textarea').exists()).toBe(false)
    expect(wrapper.text()).toContain('support.common.closedHint')

    // "关闭工单" 按钮消失
    const buttons = wrapper.findAll('button').map((b) => b.text())
    expect(buttons.some((t) => t.includes('support.common.closeTicket'))).toBe(false)
  })

  it('appends reply and refetches detail on submit', async () => {
    getMyTicket.mockResolvedValueOnce(baseTicket({ replies: [] }))
    appendReply.mockResolvedValue({ id: 99, ticket_id: 7, author_id: 1, is_admin: false, content: 'thx', created_at: '2026-06-04' })
    getMyTicket.mockResolvedValueOnce(
      baseTicket({
        status: 'in_progress',
        replies: [
          { id: 99, ticket_id: 7, author_id: 1, is_admin: false, content: 'thx', created_at: '2026-06-04' },
        ],
      }),
    )

    const wrapper = mount(SupportTicketDetailView, { global: { stubs } })
    await flushPromises()

    await wrapper.get('textarea').setValue('thx')
    const sendBtn = wrapper
      .findAll('button')
      .find((b) => b.text().includes('support.common.sendReply'))
    expect(sendBtn).toBeTruthy()
    await sendBtn!.trigger('click')
    await flushPromises()

    expect(appendReply).toHaveBeenCalledWith(7, 'thx')
    expect(showSuccess).toHaveBeenCalledTimes(1)
    // 重新拉取详情
    expect(getMyTicket).toHaveBeenCalledTimes(2)
  })
})
