import { defineComponent } from 'vue'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'

import AccountsView from '../AccountsView.vue'

const {
  listAccounts,
  listWithEtag,
  getAccountById,
  getBatchTodayStats,
  getAllProxies,
  getAllGroups,
  showSuccess,
  showError
} = vi.hoisted(() => ({
  listAccounts: vi.fn(),
  listWithEtag: vi.fn(),
  getAccountById: vi.fn(),
  getBatchTodayStats: vi.fn(),
  getAllProxies: vi.fn(),
  getAllGroups: vi.fn(),
  showSuccess: vi.fn(),
  showError: vi.fn()
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    accounts: {
      list: listAccounts,
      listWithEtag,
      getById: getAccountById,
      getBatchTodayStats,
      getUpstreamBillingProbeSettings: vi.fn().mockResolvedValue({ enabled: true, interval_minutes: 30 })
    },
    proxies: { getAll: getAllProxies },
    groups: { getAll: getAllGroups }
  }
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({
    showError,
    showSuccess,
    showInfo: vi.fn()
  })
}))

vi.mock('@/stores/auth', () => ({
  useAuthStore: () => ({ token: 'test-token' })
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({ t: (key: string) => key })
  }
})

const DataTableStub = {
  props: ['data'],
  template: `
    <div>
      <div v-for="row in data" :key="row.id">
        <slot name="cell-usage" :row="row" />
      </div>
    </div>
  `
}

const AccountUsageCellStub = defineComponent({
  props: ['account'],
  emits: ['account-refresh-requested'],
  template: `
    <button
      data-test="grok-probe-state"
      @click="$emit('account-refresh-requested', {
        accountId: account.id,
        statusCode: 402,
        observedAt: '2026-07-22T12:00:00Z'
      })"
    >
      {{ account.status }}|{{ account.schedulable }}
    </button>
  `
})

const activeAccount = {
  id: 73,
  name: 'grok-account',
  platform: 'grok',
  type: 'oauth',
  proxy_id: null,
  concurrency: 1,
  priority: 0,
  status: 'active',
  schedulable: true,
  error_message: null,
  last_used_at: null,
  expires_at: null,
  auto_pause_on_expired: false,
  created_at: '2026-07-22T00:00:00Z',
  updated_at: '2026-07-22T00:00:00Z',
  rate_limited_at: null,
  rate_limit_reset_at: null,
  overload_until: null,
  temp_unschedulable_until: null,
  temp_unschedulable_reason: null,
  session_window_start: null,
  session_window_end: null,
  session_window_status: null
}

const mountView = () => mount(AccountsView, {
  global: {
    stubs: {
      AppLayout: { template: '<div><slot /></div>' },
      TablePageLayout: {
        template: '<div><slot name="filters" /><slot name="table" /><slot name="pagination" /></div>'
      },
      DataTable: DataTableStub,
      AccountUsageCell: AccountUsageCellStub,
      AccountTableActions: { template: '<div><slot name="beforeCreate" /><slot name="after" /></div>' },
      AccountTableFilters: true,
      AccountBulkActionsBar: true,
      AccountActionMenu: true,
      ImportDataModal: true,
      ReAuthAccountModal: true,
      AccountTestModal: true,
      AccountStatsModal: true,
      ScheduledTestsPanel: true,
      SyncFromCrsModal: true,
      TempUnschedStatusModal: true,
      ErrorPassthroughRulesModal: true,
      TLSFingerprintProfilesModal: true,
      CreateAccountModal: true,
      EditAccountModal: true,
      BulkEditAccountModal: true,
      PlatformTypeBadge: true,
      AccountCapacityCell: true,
      AccountStatusIndicator: true,
      AccountTodayStatsCell: true,
      AccountGroupsCell: true,
      HelpTooltip: true,
      Pagination: true,
      ConfirmDialog: true,
      Icon: true
    }
  }
})

describe('admin AccountsView Grok account refresh', () => {
  beforeEach(() => {
    localStorage.clear()
    for (const mock of [
      listAccounts,
      listWithEtag,
      getAccountById,
      getBatchTodayStats,
      getAllProxies,
      getAllGroups,
      showSuccess,
      showError
    ]) {
      mock.mockReset()
    }

    listAccounts.mockResolvedValue({
      items: [activeAccount],
      total: 1,
      page: 1,
      page_size: 20,
      pages: 1
    })
    listWithEtag.mockResolvedValue({ notModified: true, etag: null, data: null })
    getBatchTodayStats.mockResolvedValue({ stats: {} })
    getAllProxies.mockResolvedValue([])
    getAllGroups.mockResolvedValue([])
    getAccountById.mockResolvedValue({
      ...activeAccount,
      status: 'error',
      schedulable: false,
      error_message: 'Grok upstream returned 402; account scheduling paused',
      updated_at: '2026-07-22T12:00:01Z'
    })
  })

  it('patches one account while coalescing duplicate refresh events', async () => {
    const wrapper = mountView()
    await flushPromises()

    const stateButton = wrapper.get('[data-test="grok-probe-state"]')
    expect(stateButton.text()).toBe('active|true')

    await stateButton.trigger('click')
    await stateButton.trigger('click')
    await flushPromises()

    expect(getAccountById).toHaveBeenCalledTimes(1)
    expect(getAccountById).toHaveBeenCalledWith(73)
    expect(wrapper.get('[data-test="grok-probe-state"]').text()).toBe('error|false')
    wrapper.unmount()
  })

  it('runs one trailing refresh when a newer state arrives in flight', async () => {
    let resolveFirst: ((account: typeof activeAccount) => void) | undefined
    getAccountById
      .mockImplementationOnce(() => new Promise((resolve) => { resolveFirst = resolve }))
      .mockResolvedValueOnce({
        ...activeAccount,
        status: 'error',
        schedulable: false,
        error_message: 'Grok upstream returned 402; account scheduling paused',
        updated_at: '2026-07-22T12:00:02Z'
      })
    const wrapper = mountView()
    await flushPromises()
    const setupState = wrapper.vm.$.setupState as {
      handleGrokAccountRefreshRequested: (request: {
        accountId: number
        statusCode: number
        observedAt: string | null
      }) => void
    }

    setupState.handleGrokAccountRefreshRequested({
      accountId: 73,
      statusCode: 200,
      observedAt: '2026-07-22T12:00:01Z'
    })
    setupState.handleGrokAccountRefreshRequested({
      accountId: 73,
      statusCode: 402,
      observedAt: '2026-07-22T12:00:02Z'
    })
    expect(getAccountById).toHaveBeenCalledTimes(1)

    resolveFirst?.({ ...activeAccount, updated_at: '2026-07-22T12:00:01Z' })
    await flushPromises()

    expect(getAccountById).toHaveBeenCalledTimes(2)
    expect(wrapper.get('[data-test="grok-probe-state"]').text()).toBe('error|false')
    wrapper.unmount()
  })
})
