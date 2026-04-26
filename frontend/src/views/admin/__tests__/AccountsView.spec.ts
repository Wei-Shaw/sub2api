import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'

import type { Account, AdminGroup, PaginatedResponse, WindowStats } from '@/types'
import AccountsView from '../AccountsView.vue'

const {
  listAccounts,
  listWithEtag,
  getFilterModels,
  getBatchTodayStats,
  listProxies,
  getAllProxies,
  getAllGroups
} = vi.hoisted(() => ({
  listAccounts: vi.fn(),
  listWithEtag: vi.fn(),
  getFilterModels: vi.fn(),
  getBatchTodayStats: vi.fn(),
  listProxies: vi.fn(),
  getAllProxies: vi.fn(),
  getAllGroups: vi.fn()
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    accounts: {
      list: listAccounts,
      listWithEtag,
      getFilterModels,
      getBatchTodayStats,
      delete: vi.fn(),
      batchClearError: vi.fn(),
      batchRefresh: vi.fn(),
      bulkUpdate: vi.fn(),
      exportData: vi.fn(),
      getAvailableModels: vi.fn(),
      refreshCredentials: vi.fn(),
      recoverState: vi.fn(),
      resetAccountQuota: vi.fn(),
      setPrivacy: vi.fn(),
      setSchedulable: vi.fn()
    },
    proxies: {
      list: listProxies,
      getAll: getAllProxies
    },
    groups: {
      getAll: getAllGroups
    }
  }
}))

const showError = vi.fn()
const showSuccess = vi.fn()

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({
    showError,
    showSuccess
  })
}))

vi.mock('@/stores/auth', () => ({
  useAuthStore: () => ({
    isSimpleMode: false
  })
}))

vi.mock('@/composables/useSwipeSelect', () => ({
  useSwipeSelect: vi.fn()
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string, params?: Record<string, unknown>) => {
        if (key === 'admin.accounts.autoRefreshCountdown' && params?.seconds != null) {
          return `${key}:${params.seconds}`
        }
        return key
      }
    })
  }
})

const createAccount = (): Account => ({
  id: 1,
  name: 'Claude Account',
  platform: 'anthropic',
  type: 'oauth',
  credentials: {},
  extra: {},
  proxy_id: null,
  concurrency: 1,
  priority: 1,
  status: 'active',
  error_message: null,
  last_used_at: null,
  expires_at: null,
  auto_pause_on_expired: false,
  created_at: '2026-04-25T00:00:00Z',
  updated_at: '2026-04-25T00:00:00Z',
  groups: [],
  group_ids: [],
  schedulable: true,
  rate_limited_at: null,
  rate_limit_reset_at: null,
  overload_until: null,
  temp_unschedulable_until: null,
  temp_unschedulable_reason: null,
  session_window_start: null,
  session_window_end: null,
  session_window_status: null
})

const createListResponse = (): PaginatedResponse<Account> => ({
  items: [createAccount()],
  total: 1,
  page: 1,
  page_size: 20,
  pages: 1
})

const anthropicGroups = [
  {
    platform: 'anthropic',
    label: 'Anthropic',
    models: [{ value: 'claude-3-7-sonnet-20250219', label: 'claude-3-7-sonnet-20250219' }]
  }
]

const openaiGroups = [
  {
    platform: 'openai',
    label: 'OpenAI',
    models: [{ value: 'gpt-4o', label: 'gpt-4o' }]
  }
]

const AccountTableFiltersStub = {
  props: ['filters', 'groups', 'modelGroups', 'searchQuery'],
  emits: ['update:filters', 'change', 'update:searchQuery'],
  template: `
    <div>
      <div data-test="current-model">{{ filters.model || '' }}</div>
      <div data-test="current-quota-strategy">{{ filters.quota_strategy || '' }}</div>
      <div data-test="current-proxy-filter">{{ filters.proxy_filter || '' }}</div>
      <div data-test="model-groups">{{ (modelGroups || []).map(group => group.platform).join(',') }}</div>
      <button
        data-test="set-model"
        @click="$emit('update:filters', { ...filters, model: 'claude-3-7-sonnet-20250219' }); $emit('change')"
      >set-model</button>
      <button
        data-test="set-openai-platform"
        @click="$emit('update:filters', { ...filters, platform: 'openai' }); $emit('change')"
      >set-openai-platform</button>
      <button
        data-test="set-quota-strategy"
        @click="$emit('update:filters', { ...filters, quota_strategy: 'enabled' }); $emit('change')"
      >set-quota-strategy</button>
      <button
        data-test="set-proxy-filter"
        @click="$emit('update:filters', { ...filters, proxy_filter: 'configured' }); $emit('change')"
      >set-proxy-filter</button>
      <button
        data-test="set-status-filter"
        @click="$emit('update:filters', { ...filters, status: 'active_excluding_quota_stopped' }); $emit('change')"
      >set-status-filter</button>
    </div>
  `
}

const TablePageLayoutStub = {
  template: '<div><slot name="filters" /><slot name="table" /><slot name="pagination" /></div>'
}

const AccountTableActionsStub = {
  template: '<div><slot name="beforeCreate" /><slot /><slot name="after" /></div>'
}

const createWrapper = () => mount(AccountsView, {
  global: {
    stubs: {
      AppLayout: { template: '<div><slot /></div>' },
      TablePageLayout: TablePageLayoutStub,
      AccountTableFilters: AccountTableFiltersStub,
      AccountTableActions: AccountTableActionsStub,
      AccountBulkActionsBar: true,
      DataTable: { template: '<div />' },
      Pagination: true,
      ConfirmDialog: { template: '<div><slot /></div>' },
      CreateAccountModal: true,
      EditAccountModal: true,
      BulkEditAccountModal: true,
      SyncFromCrsModal: true,
      TempUnschedStatusModal: true,
      ImportDataModal: true,
      ReAuthAccountModal: true,
      AccountTestModal: true,
      AccountStatsModal: true,
      ScheduledTestsPanel: true,
      AccountActionMenu: true,
      ErrorPassthroughRulesModal: true,
      TLSFingerprintProfilesModal: true,
      AccountStatusIndicator: true,
      AccountUsageCell: true,
      AccountTodayStatsCell: true,
      AccountGroupsCell: true,
      AccountCapacityCell: true,
      PlatformTypeBadge: true,
      Icon: true,
      Teleport: true
    }
  }
})

describe('admin AccountsView', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    localStorage.clear()

    listAccounts.mockReset()
    listWithEtag.mockReset()
    getFilterModels.mockReset()
    getBatchTodayStats.mockReset()
    listProxies.mockReset()
    getAllProxies.mockReset()
    getAllGroups.mockReset()
    showError.mockReset()
    showSuccess.mockReset()

    listAccounts.mockResolvedValue(createListResponse())
    listWithEtag.mockResolvedValue({ notModified: true, etag: 'etag', data: null })
    getBatchTodayStats.mockResolvedValue({ stats: { '1': { requests: 0, tokens: 0, cost: 0, standard_cost: 0, user_cost: 0 } satisfies WindowStats } })
    listProxies.mockResolvedValue({ items: [], total: 0, page: 1, page_size: 500, pages: 1 })
    getAllProxies.mockResolvedValue([])
    getAllGroups.mockResolvedValue([] as AdminGroup[])
    getFilterModels.mockImplementation(async (platform?: string) => {
      if (platform === 'openai') return openaiGroups
      if (platform === 'anthropic') return anthropicGroups
      return [...anthropicGroups, ...openaiGroups]
    })
  })

  it('首屏加载模型筛选项，并透传 model 参数请求列表', async () => {
    const wrapper = createWrapper()

    await flushPromises()

    expect(getFilterModels).toHaveBeenCalledWith(undefined)
    expect(wrapper.get('[data-test="model-groups"]').text()).toContain('anthropic')

    await wrapper.get('[data-test="set-model"]').trigger('click')
    await vi.advanceTimersByTimeAsync(350)
    await flushPromises()

    expect(listAccounts).toHaveBeenLastCalledWith(
      1,
      20,
      expect.objectContaining({
        model: 'claude-3-7-sonnet-20250219'
      }),
      expect.any(Object)
    )
  })

  it('切换平台时会清空无效模型值并重新拉取平台模型', async () => {
    const wrapper = createWrapper()

    await flushPromises()

    await wrapper.get('[data-test="set-model"]').trigger('click')
    await vi.advanceTimersByTimeAsync(350)
    await flushPromises()
    expect(wrapper.get('[data-test="current-model"]').text()).toBe('claude-3-7-sonnet-20250219')

    await wrapper.get('[data-test="set-openai-platform"]').trigger('click')
    await flushPromises()
    await vi.advanceTimersByTimeAsync(350)
    await flushPromises()

    expect(getFilterModels).toHaveBeenLastCalledWith('openai')
    expect(wrapper.get('[data-test="current-model"]').text()).toBe('')
    expect(listAccounts).toHaveBeenLastCalledWith(
      1,
      20,
      expect.objectContaining({
        platform: 'openai',
        model: ''
      }),
      expect.any(Object)
    )
  })

  it('额度策略筛选会透传到列表请求', async () => {
    const wrapper = createWrapper()

    await flushPromises()

    await wrapper.get('[data-test="set-quota-strategy"]').trigger('click')
    await vi.advanceTimersByTimeAsync(350)
    await flushPromises()

    expect(wrapper.get('[data-test="current-quota-strategy"]').text()).toBe('enabled')
    expect(listAccounts).toHaveBeenLastCalledWith(
      1,
      20,
      expect.objectContaining({
        quota_strategy: 'enabled'
      }),
      expect.any(Object)
    )
  })

  it('代理筛选会透传到列表请求', async () => {
    const wrapper = createWrapper()

    await flushPromises()

    await wrapper.get('[data-test="set-proxy-filter"]').trigger('click')
    await vi.advanceTimersByTimeAsync(350)
    await flushPromises()

    expect(wrapper.get('[data-test="current-proxy-filter"]').text()).toBe('configured')
    expect(listAccounts).toHaveBeenLastCalledWith(
      1,
      20,
      expect.objectContaining({
        proxy_filter: 'configured'
      }),
      expect.any(Object)
    )
  })

  it('新状态筛选会透传到列表请求', async () => {
    const wrapper = createWrapper()

    await flushPromises()

    await wrapper.get('[data-test="set-status-filter"]').trigger('click')
    await vi.advanceTimersByTimeAsync(350)
    await flushPromises()

    expect(listAccounts).toHaveBeenLastCalledWith(
      1,
      20,
      expect.objectContaining({
        status: 'active_excluding_quota_stopped'
      }),
      expect.any(Object)
    )
  })
})
