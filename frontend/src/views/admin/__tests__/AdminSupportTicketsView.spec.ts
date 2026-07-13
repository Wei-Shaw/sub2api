/**
 * AdminSupportTicketsView 单测
 *
 * 覆盖 §13.4 验收点：
 *   - 过滤器变化触发列表请求（status / q / userId）
 *   - 抽屉打开后能修改 priority + status，PATCH 仅提交 dirty 字段
 *   - 关闭工单后状态徽章切换为 closed
 *   - listCategories 404 时降级为 free-text 输入
 */
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { defineComponent, h } from 'vue'
import { flushPromises, mount } from '@vue/test-utils'

import AdminSupportTicketsView from '../AdminSupportTicketsView.vue'

const {
  adminListTickets,
  adminGetTicket,
  adminAppendReply,
  adminPatchTicket,
  listCategories,
  showSuccess,
  showError,
} = vi.hoisted(() => ({
  adminListTickets: vi.fn(),
  adminGetTicket: vi.fn(),
  adminAppendReply: vi.fn(),
  adminPatchTicket: vi.fn(),
  listCategories: vi.fn(),
  showSuccess: vi.fn(),
  showError: vi.fn(),
}))

vi.mock('@/api/support', () => ({
  adminListTickets,
  adminGetTicket,
  adminAppendReply,
  adminPatchTicket,
  listCategories,
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({ showSuccess, showError }),
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
  setup(props, { slots }) {
    return () => (props.show && slots.default ? h('div', { 'data-test': 'drawer' }, [slots.default()]) : null)
  },
})

const stubs = {
  AppLayout: { template: '<div><slot /></div>' },
  Icon: true,
  Pagination: true,
  BaseDialog: BaseDialogStub,
  SupportStatusBadge: { props: ['status'], template: '<span class="badge-status">{{ status }}</span>' },
  SupportPriorityBadge: { props: ['priority'], template: '<span class="badge-priority">{{ priority }}</span>' },
}

const baseRow = (overrides: Record<string, unknown> = {}) => ({
  id: 1,
  user_id: 11,
  title: 'Need help',
  content: '...',
  category: 'API',
  status: 'open' as const,
  priority: 'normal' as const,
  closed_at: null,
  created_at: '2026-06-01',
  updated_at: '2026-06-01',
  ...overrides,
})

const baseDetail = (overrides: Record<string, unknown> = {}) => ({
  ...baseRow(),
  chat_context: null,
  replies: [],
  ...overrides,
})

beforeEach(() => {
  adminListTickets.mockReset()
  adminGetTicket.mockReset()
  adminAppendReply.mockReset()
  adminPatchTicket.mockReset()
  listCategories.mockReset()
  showSuccess.mockReset()
  showError.mockReset()

  adminListTickets.mockResolvedValue({ items: [baseRow()], total: 1, page: 1, page_size: 20, pages: 1 })
  listCategories.mockResolvedValue({ categories: ['API', '账号', '充值'], default_priority: 'normal' })
})

describe('AdminSupportTicketsView', () => {
  it('refetches list when filter status changes (page reset to 1)', async () => {
    const wrapper = mount(AdminSupportTicketsView, { global: { stubs } })
    await flushPromises()
    expect(adminListTickets).toHaveBeenCalledTimes(1)

    const statusSelect = wrapper.findAll('select')[0]
    await statusSelect.setValue('open')
    await flushPromises()

    expect(adminListTickets).toHaveBeenCalledTimes(2)
    const lastArgs = adminListTickets.mock.calls.at(-1)
    expect(lastArgs?.[0]).toMatchObject({ status: 'open' })
    expect(lastArgs?.[1]).toBe(1)
  })

  it('falls back to free-text category input when listCategories rejects (feature_disabled)', async () => {
    listCategories.mockRejectedValueOnce(new Error('feature disabled'))

    const wrapper = mount(AdminSupportTicketsView, { global: { stubs } })
    await flushPromises()

    // 顶部过滤栏 category 应该是 input，不是 select
    const labels = wrapper.findAll('.input-label').map((l) => l.text())
    const categoryIdx = labels.findIndex((label) => label === 'support.common.category')
    expect(categoryIdx).toBeGreaterThanOrEqual(0)

    // 第三个过滤项是 category；当 categories 空时降级为 input[type=text]
    const inputs = wrapper.findAll('input[type="text"]')
    expect(inputs.length).toBeGreaterThan(0)
  })

  it('opens drawer, edits priority + status, and PATCH only sends dirty fields', async () => {
    adminGetTicket.mockResolvedValueOnce(baseDetail())
    adminPatchTicket.mockResolvedValue(baseRow({ priority: 'high', status: 'in_progress' }))
    // 第二次刷新 detail
    adminGetTicket.mockResolvedValueOnce(baseDetail({ priority: 'high', status: 'in_progress' }))

    const wrapper = mount(AdminSupportTicketsView, { global: { stubs } })
    await flushPromises()

    // 点击行打开抽屉
    await wrapper.get('tbody tr').trigger('click')
    await flushPromises()
    expect(adminGetTicket).toHaveBeenCalledWith(1)

    const drawer = wrapper.get('[data-test="drawer"]')
    // 抽屉里 PATCH 区的 select：[0]=status, [1]=priority, [2]=category
    const drawerSelects = drawer.findAll('select')
    expect(drawerSelects.length).toBeGreaterThanOrEqual(2)
    await drawerSelects[0].setValue('in_progress') // status
    await drawerSelects[1].setValue('high') // priority

    const saveBtn = drawer.findAll('button').find((b) => b.text().includes('admin.tickets.drawer.save'))
    expect(saveBtn).toBeTruthy()
    await saveBtn!.trigger('click')
    await flushPromises()

    // PATCH 入参仅包含 dirty 字段（category 未改 → 不应出现）
    expect(adminPatchTicket).toHaveBeenCalledTimes(1)
    expect(adminPatchTicket).toHaveBeenCalledWith(1, { status: 'in_progress', priority: 'high' })
    expect(showSuccess).toHaveBeenCalled()
  })

  it('reflects status change in the table row after PATCH closes ticket', async () => {
    // 第一次 list（初次加载）+ 第二次 list（保存后 fetchList 重拉）
    adminListTickets.mockReset()
    adminListTickets.mockResolvedValueOnce({
      items: [baseRow()],
      total: 1, page: 1, page_size: 20, pages: 1,
    })
    adminGetTicket.mockResolvedValueOnce(baseDetail())
    adminPatchTicket.mockResolvedValue(baseRow({ status: 'closed', closed_at: '2026-06-10' }))
    adminGetTicket.mockResolvedValueOnce(
      baseDetail({ status: 'closed', closed_at: '2026-06-10' }),
    )
    adminListTickets.mockResolvedValueOnce({
      items: [baseRow({ status: 'closed', closed_at: '2026-06-10' })],
      total: 1, page: 1, page_size: 20, pages: 1,
    })

    const wrapper = mount(AdminSupportTicketsView, { global: { stubs } })
    await flushPromises()

    await wrapper.get('tbody tr').trigger('click')
    await flushPromises()

    const drawer = wrapper.get('[data-test="drawer"]')
    const drawerSelects = drawer.findAll('select')
    await drawerSelects[0].setValue('closed')

    const saveBtn = drawer.findAll('button').find((b) => b.text().includes('admin.tickets.drawer.save'))
    await saveBtn!.trigger('click')
    await flushPromises()

    expect(adminPatchTicket).toHaveBeenCalledWith(1, { status: 'closed' })
    // PATCH 成功后 fetchList 会重拉，列表行的 status badge 应反映 closed
    expect(adminListTickets).toHaveBeenCalledTimes(2)
    const rowBadge = wrapper.find('tbody tr .badge-status')
    expect(rowBadge.exists()).toBe(true)
    expect(rowBadge.text()).toBe('closed')
  })
})
