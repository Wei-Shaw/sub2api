import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { defineComponent } from 'vue'

import SubscriptionsView from '../SubscriptionsView.vue'

const { listSubscriptions, getAllGroups } = vi.hoisted(() => ({
  listSubscriptions: vi.fn(),
  getAllGroups: vi.fn()
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    subscriptions: { list: listSubscriptions },
    groups: { getAll: getAllGroups }
  }
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({
    showError: vi.fn(),
    showSuccess: vi.fn()
  })
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string, params?: { id?: number }) =>
        key === 'admin.redeem.userPrefix' ? `User #${params?.id}` : key
    })
  }
})

const DataTableStub = {
  props: ['data'],
  template: `
    <div>
      <div v-for="row in data" :key="row.id">
        <slot name="cell-user" :row="row" />
      </div>
    </div>
  `
}

const RouterLinkStub = defineComponent({
  name: 'RouterLink',
  props: { to: { type: Object, required: true } },
  template: '<a :href="`${to.path}?user_id=${to.query.user_id}`"><slot /></a>'
})

describe('admin subscription user usage link', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    localStorage.clear()
    listSubscriptions.mockResolvedValue({
      items: [{
        id: 9,
        user_id: 42,
        group_id: 3,
        status: 'active',
        starts_at: '2026-01-01T00:00:00Z',
        expires_at: null,
        daily_usage_usd: 0,
        weekly_usage_usd: 0,
        monthly_usage_usd: 0,
        daily_window_start: null,
        weekly_window_start: null,
        monthly_window_start: null,
        created_at: '2026-01-01T00:00:00Z',
        updated_at: '2026-01-01T00:00:00Z',
        user: { email: 'reader@example.com', username: 'Reader' }
      }],
      total: 1,
      pages: 1
    })
    getAllGroups.mockResolvedValue([])
  })

  const mountView = () => mount(SubscriptionsView, {
    global: {
      stubs: {
        AppLayout: { template: '<div><slot /></div>' },
        TablePageLayout: { template: '<div><slot name="table" /></div>' },
        DataTable: DataTableStub,
        RouterLink: RouterLinkStub,
        Pagination: true,
        BaseDialog: true,
        ConfirmDialog: true,
        EmptyState: true,
        Select: true,
        GroupBadge: true,
        GroupOptionItem: true,
        Icon: true,
        Teleport: true
      }
    }
  })

  it('renders the user email as a link to that user filtered usage records', async () => {
    const wrapper = mountView()

    await flushPromises()

    const link = wrapper.getComponent(RouterLinkStub)
    expect(link.text()).toBe('reader@example.com')
    expect(link.props('to')).toEqual({ path: '/admin/usage', query: { user_id: 42 } })
  })

  it('uses the user ID label for the usage link when username mode has no username', async () => {
    localStorage.setItem('subscription-user-column-mode', 'username')
    listSubscriptions.mockResolvedValue({
      items: [{
        id: 9,
        user_id: 42,
        group_id: 3,
        status: 'active',
        starts_at: '2026-01-01T00:00:00Z',
        expires_at: null,
        daily_usage_usd: 0,
        weekly_usage_usd: 0,
        monthly_usage_usd: 0,
        daily_window_start: null,
        weekly_window_start: null,
        monthly_window_start: null,
        created_at: '2026-01-01T00:00:00Z',
        updated_at: '2026-01-01T00:00:00Z',
        user: { email: 'reader@example.com' }
      }],
      total: 1,
      pages: 1
    })

    const wrapper = mountView()
    await flushPromises()

    const link = wrapper.getComponent(RouterLinkStub)
    expect(link.text()).toBe('User #42')
    expect(link.props('to')).toEqual({ path: '/admin/usage', query: { user_id: 42 } })
  })
})
