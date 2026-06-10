/**
 * SupportTicketNewView 单测
 *
 * 覆盖 §13.2 验收点：
 *   - 表单初始 disabled（标题/内容/分类未填写）
 *   - 提交成功后跳详情页
 *   - URL query `from=chat&session=<key>` 命中且 localStorage 有内容时自动填充
 *   - localStorage 缺失时 silent skip + warning toast
 */
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'

import SupportTicketNewView from '../SupportTicketNewView.vue'

const {
  createTicket,
  listCategories,
  push,
  replace,
  showSuccess,
  showError,
  showInfo,
  showWarning,
  routeQuery,
} = vi.hoisted(() => ({
  createTicket: vi.fn(),
  listCategories: vi.fn(),
  push: vi.fn(),
  replace: vi.fn(),
  showSuccess: vi.fn(),
  showError: vi.fn(),
  showInfo: vi.fn(),
  showWarning: vi.fn(),
  routeQuery: { value: {} as Record<string, string> },
}))

vi.mock('@/api/support', () => ({
  createTicket,
  listCategories,
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({
    showSuccess,
    showError,
    showInfo,
    showWarning,
  }),
}))

vi.mock('vue-router', () => ({
  useRouter: () => ({ push, replace }),
  useRoute: () => ({
    get query() {
      return routeQuery.value
    },
  }),
}))

vi.mock('@/utils/apiError', () => ({
  extractI18nErrorMessage: (_err: unknown, _t: unknown, _ns: string, fallback: string) =>
    fallback,
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string) => key,
    }),
  }
})

const stubs = {
  AppLayout: { template: '<div><slot /></div>' },
  Icon: true,
}

beforeEach(() => {
  createTicket.mockReset()
  listCategories.mockReset()
  push.mockReset()
  replace.mockReset()
  showSuccess.mockReset()
  showError.mockReset()
  showInfo.mockReset()
  showWarning.mockReset()
  routeQuery.value = {}
  localStorage.clear()
  listCategories.mockResolvedValue({
    categories: ['账号', '充值', 'API'],
    default_priority: 'normal',
  })
})

describe('SupportTicketNewView', () => {
  it('disables submit button until title / category / content are filled', async () => {
    const wrapper = mount(SupportTicketNewView, { global: { stubs } })
    await flushPromises()

    const submit = wrapper.find('button[type="submit"]')
    expect(submit.exists()).toBe(true)
    expect(submit.attributes('disabled')).toBeDefined()

    await wrapper.get('#ticket-title').setValue('Need help')
    await wrapper.get('#ticket-content').setValue('Please assist with my issue.')
    await wrapper.get('#ticket-category').setValue('API')

    expect(wrapper.find('button[type="submit"]').attributes('disabled')).toBeUndefined()
  })

  it('submits ticket and routes to detail on success', async () => {
    createTicket.mockResolvedValue({ id: 42, replies: [] })

    const wrapper = mount(SupportTicketNewView, { global: { stubs } })
    await flushPromises()

    await wrapper.get('#ticket-title').setValue('Reset password')
    await wrapper.get('#ticket-category').setValue('账号')
    await wrapper.get('#ticket-content').setValue('Cannot login.')

    await wrapper.find('form').trigger('submit.prevent')
    await flushPromises()

    expect(createTicket).toHaveBeenCalledWith({
      title: 'Reset password',
      category: '账号',
      content: 'Cannot login.',
    })
    expect(replace).toHaveBeenCalledWith('/support/tickets/42')
    expect(showSuccess).toHaveBeenCalled()
  })

  it('hydrates content + chat_context from localStorage when query=from=chat', async () => {
    routeQuery.value = { from: 'chat', session: 'sess-xyz' }
    localStorage.setItem(
      'sess-xyz',
      JSON.stringify([
        { role: 'user', content: 'Why does X fail?' },
        { role: 'assistant', content: 'Try Y.' },
      ]),
    )

    const wrapper = mount(SupportTicketNewView, { global: { stubs } })
    await flushPromises()

    const textarea = wrapper.get<HTMLTextAreaElement>('#ticket-content').element
    expect(textarea.value).toContain('Why does X fail?')
    expect(textarea.value).toContain('Try Y.')
    expect(textarea.value).toContain('对话上下文')
    // chat_context badge 提示出现
    expect(wrapper.text()).toContain('support.common.contextHint')
  })

  it('silent-skips and warns when query session key is missing in localStorage', async () => {
    routeQuery.value = { from: 'chat', session: 'missing-key' }

    const wrapper = mount(SupportTicketNewView, { global: { stubs } })
    await flushPromises()

    const textarea = wrapper.get<HTMLTextAreaElement>('#ticket-content').element
    expect(textarea.value).toBe('')
    expect(showWarning).toHaveBeenCalledTimes(1)
    expect(wrapper.text()).not.toContain('support.common.contextHint')
  })
})
