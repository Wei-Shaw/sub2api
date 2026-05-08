import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'

import type { Group, UserSubscription } from '@/types'
import SubscriptionsView from '../SubscriptionsView.vue'

const {
  listSubscriptions,
  getAllGroups,
  copyToClipboard,
  showError,
  showSuccess
} = vi.hoisted(() => ({
  listSubscriptions: vi.fn(),
  getAllGroups: vi.fn(),
  copyToClipboard: vi.fn(),
  showError: vi.fn(),
  showSuccess: vi.fn()
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    subscriptions: {
      list: listSubscriptions,
      assign: vi.fn(),
      extend: vi.fn(),
      revoke: vi.fn(),
      resetQuota: vi.fn()
    },
    groups: {
      getAll: getAllGroups
    },
    usage: {
      searchUsers: vi.fn()
    }
  }
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({
    showError,
    showSuccess
  })
}))

vi.mock('@/composables/useClipboard', () => ({
  useClipboard: () => ({
    copyToClipboard
  })
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  const labels: Record<string, string> = {
    'admin.subscriptions.daily': 'Ngày',
    'admin.subscriptions.weekly': 'Tuần',
    'admin.subscriptions.monthly': 'Tháng',
    'common.copy': 'Copy'
  }

  return {
    ...actual,
    useI18n: () => ({
      t: (key: string) => labels[key] ?? key
    })
  }
})

const createGroup = (overrides: Partial<Group> = {}): Group => ({
  id: 10,
  name: 'OpenAI-Subscription',
  description: null,
  platform: 'openai',
  rate_multiplier: 1,
  is_exclusive: false,
  status: 'active',
  subscription_type: 'subscription',
  daily_limit_usd: 5,
  weekly_limit_usd: 10,
  monthly_limit_usd: 20,
  allow_image_generation: false,
  image_rate_independent: false,
  image_rate_multiplier: 1,
  image_price_1k: null,
  image_price_2k: null,
  image_price_4k: null,
  claude_code_only: false,
  fallback_group_id: null,
  fallback_group_id_on_invalid_request: null,
  require_oauth_only: false,
  require_privacy_set: false,
  created_at: '2026-05-08T00:00:00Z',
  updated_at: '2026-05-08T00:00:00Z',
  ...overrides
})

const createSubscription = (overrides: Partial<UserSubscription> = {}): UserSubscription => ({
  id: 1,
  user_id: 134,
  group_id: 10,
  status: 'active',
  daily_usage_usd: 0.03,
  weekly_usage_usd: 0.03,
  monthly_usage_usd: 0,
  daily_window_start: '2026-05-08T00:00:00Z',
  weekly_window_start: '2026-05-04T00:00:00Z',
  monthly_window_start: '2026-05-01T00:00:00Z',
  created_at: '2026-05-08T00:00:00Z',
  updated_at: '2026-05-08T00:00:00Z',
  expires_at: '2026-05-11T00:00:00Z',
  user: {
    id: 134,
    username: 'invite-user',
    email: 'invite-f9f57fa29ae0ea07bc41a80cc87fbee9@invite-local.invalid',
    role: 'user',
    balance: 0,
    concurrency: 1,
    status: 'active',
    allowed_groups: [],
    balance_notify_enabled: false,
    balance_notify_threshold: null,
    balance_notify_extra_emails: [],
    created_at: '2026-05-08T00:00:00Z',
    updated_at: '2026-05-08T00:00:00Z'
  },
  group: createGroup(),
  device_identity_code: 'DLG-YPK2-AAAA-BBBB',
  device_identity_type: 'device_login',
  has_device_binding: true,
  ...overrides
})

const DataTableStub = {
  props: ['columns', 'data'],
  emits: ['sort'],
  template: `
    <div>
      <div v-for="row in data" :key="row.id" data-test="subscription-row">
        <slot name="cell-user" :row="row" />
        <slot name="cell-usage" :row="row" />
      </div>
    </div>
  `
}

const mountView = () => mount(SubscriptionsView, {
  global: {
    stubs: {
      AppLayout: { template: '<div><slot /></div>' },
      RouterLink: { template: '<a><slot /></a>' },
      TablePageLayout: { template: '<div><slot name="filters" /><slot name="table" /><slot name="pagination" /></div>' },
      DataTable: DataTableStub,
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

describe('admin SubscriptionsView', () => {
  beforeEach(() => {
    localStorage.clear()
    listSubscriptions.mockReset()
    getAllGroups.mockReset()
    copyToClipboard.mockReset()
    showError.mockReset()
    showSuccess.mockReset()

    const subscription = createSubscription()
    listSubscriptions.mockResolvedValue({
      items: [subscription],
      total: 1,
      page: 1,
      page_size: 20,
      pages: 1
    })
    getAllGroups.mockResolvedValue([createGroup()])
    copyToClipboard.mockResolvedValue(true)
  })

  it('shows only the raw device code with a copy button', async () => {
    const wrapper = mountView()

    await flushPromises()

    expect(wrapper.text()).toContain('DLG-YPK2-AAAA-BBBB')
    expect(wrapper.text()).not.toContain('admin.subscriptions.deviceCode')
    expect(wrapper.text()).not.toContain('admin.subscriptions.deviceBound')

    await wrapper.get('button[aria-label="Copy"]').trigger('click')
    await flushPromises()

    expect(copyToClipboard).toHaveBeenCalledWith('DLG-YPK2-AAAA-BBBB')
  })

  it('uses short Vietnamese usage interval labels', async () => {
    const wrapper = mountView()

    await flushPromises()

    expect(wrapper.text()).toContain('Ngày')
    expect(wrapper.text()).toContain('Tuần')
    expect(wrapper.text()).toContain('Tháng')
    expect(wrapper.text()).not.toContain('Hàng ngày')
    expect(wrapper.text()).not.toContain('Hàng tuần')
    expect(wrapper.text()).not.toContain('Hàng tháng')
  })
})
