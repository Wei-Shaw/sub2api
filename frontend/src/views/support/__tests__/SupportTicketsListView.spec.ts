/**
 * SupportTicketsListView 单测
 *
 * 覆盖 §13.1 验收点：
 *   - 空态文案展示
 *   - 列表渲染（行数 / 状态徽章）
 *   - 分页交互（changePage / changePageSize 都会触发新的 listMyTickets 请求）
 *   - 点击新建工单跳路由
 *
 * 设计要点：
 *   - 沿用 RedeemView.batchUpdate / RiskControlView 的 vi.hoisted + vi.mock 模式，
 *     不引入 pinia / 完整 vue-i18n / vue-router 实例，避免 mount 复杂度。
 *   - extractI18nErrorMessage 直接返回 fallback，测试关注组件本身行为而非 i18n 翻译。
 *   - "feature_disabled 时入口不渲染" 是 sidebar 层面的逻辑（已在 §10 覆盖于
 *     `components/layout/__tests__/AppSidebar.spec.ts` 的 feature flag 体系），
 *     本组件无需重复测试，只验证 props=disabled 场景下后端 404 走 toast 路径。
 */
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'

import SupportTicketsListView from '../SupportTicketsListView.vue'

const { listMyTickets, push, showError } = vi.hoisted(() => ({
  listMyTickets: vi.fn(),
  push: vi.fn(),
  showError: vi.fn(),
}))

vi.mock('@/api/support', () => ({
  listMyTickets,
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({
    showError,
  }),
}))

vi.mock('vue-router', () => ({
  useRouter: () => ({ push }),
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
    useI18n: () => ({
      t: (key: string) => key,
    }),
  }
})

const stubs = {
  AppLayout: { template: '<div><slot /></div>' },
  Icon: true,
  Pagination: {
    props: ['total', 'page', 'pageSize'],
    emits: ['update:page', 'update:pageSize'],
    template: `<div class="pagination-stub">
      <button data-test="page-next" @click="$emit('update:page', 2)">next</button>
      <button data-test="page-size" @click="$emit('update:pageSize', 50)">50</button>
    </div>`,
  },
  SupportStatusBadge: { props: ['status'], template: '<span class="badge-status">{{ status }}</span>' },
  SupportPriorityBadge: { props: ['priority'], template: '<span class="badge-priority">{{ priority }}</span>' },
}

describe('SupportTicketsListView', () => {
  beforeEach(() => {
    listMyTickets.mockReset()
    push.mockReset()
    showError.mockReset()
  })

  it('renders empty state when API returns no items', async () => {
    listMyTickets.mockResolvedValue({ items: [], total: 0, page: 1, page_size: 20, pages: 0 })

    const wrapper = mount(SupportTicketsListView, { global: { stubs } })
    await flushPromises()

    expect(wrapper.text()).toContain('support.list.empty')
    // 空态时不展示分页器
    expect(wrapper.find('.pagination-stub').exists()).toBe(false)
  })

  it('renders ticket rows with status / priority badges', async () => {
    listMyTickets.mockResolvedValue({
      items: [
        {
          id: 1, user_id: 7, title: 'Hello', content: '', category: 'API',
          status: 'open', priority: 'normal', created_at: '2026-06-01', updated_at: '2026-06-01',
        },
        {
          id: 2, user_id: 7, title: 'World', content: '', category: '账号',
          status: 'in_progress', priority: 'high', created_at: '2026-06-02', updated_at: '2026-06-02',
        },
      ],
      total: 2, page: 1, page_size: 20, pages: 1,
    })

    const wrapper = mount(SupportTicketsListView, { global: { stubs } })
    await flushPromises()

    const rows = wrapper.findAll('tbody tr')
    expect(rows).toHaveLength(2)
    expect(rows[0].text()).toContain('#1')
    expect(rows[0].text()).toContain('Hello')
    expect(rows[0].find('.badge-status').text()).toBe('open')
    expect(rows[1].find('.badge-priority').text()).toBe('high')
  })

  it('triggers refetch with new page on pagination change', async () => {
    listMyTickets.mockResolvedValue({
      items: [
        { id: 1, user_id: 7, title: 'A', content: '', category: '', status: 'open', priority: 'normal', created_at: '', updated_at: '' },
      ],
      total: 100, page: 1, page_size: 20, pages: 5,
    })

    const wrapper = mount(SupportTicketsListView, { global: { stubs } })
    await flushPromises()
    expect(listMyTickets).toHaveBeenCalledWith(1, 20)

    await wrapper.get('[data-test="page-next"]').trigger('click')
    await flushPromises()
    expect(listMyTickets).toHaveBeenLastCalledWith(2, 20)

    await wrapper.get('[data-test="page-size"]').trigger('click')
    await flushPromises()
    // pageSize 变化后应回到第 1 页
    expect(listMyTickets).toHaveBeenLastCalledWith(1, 50)
  })

  it('routes to /support/tickets/new when clicking primary button', async () => {
    listMyTickets.mockResolvedValue({ items: [], total: 0, page: 1, page_size: 20, pages: 0 })

    const wrapper = mount(SupportTicketsListView, { global: { stubs } })
    await flushPromises()

    const primary = wrapper.findAll('button').find((b) => b.text().includes('support.list.newButton'))
    expect(primary).toBeTruthy()
    await primary!.trigger('click')

    expect(push).toHaveBeenCalledWith('/support/tickets/new')
  })

  it('shows error toast when API throws (e.g. feature_disabled)', async () => {
    listMyTickets.mockRejectedValue(new Error('feature disabled'))

    const wrapper = mount(SupportTicketsListView, { global: { stubs } })
    await flushPromises()

    expect(showError).toHaveBeenCalledTimes(1)
    expect(wrapper.text()).toContain('support.list.empty')
  })
})
