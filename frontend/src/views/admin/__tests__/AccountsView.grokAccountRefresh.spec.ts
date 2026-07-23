import { defineComponent, nextTick } from 'vue'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'

import AccountsView from '../AccountsView.vue'

const {
  listAccounts,
  listWithEtag,
  getAccountById,
  getBatchTodayStats,
  getAllProxies,
  getAllGroups,
  startProbeAll,
  getProbeAllStatus,
  showSuccess,
  showError
} = vi.hoisted(() => ({
  listAccounts: vi.fn(),
  listWithEtag: vi.fn(),
  getAccountById: vi.fn(),
  getBatchTodayStats: vi.fn(),
  getAllProxies: vi.fn(),
  getAllGroups: vi.fn(),
  startProbeAll: vi.fn(),
  getProbeAllStatus: vi.fn(),
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
    grok: {
      startProbeAll,
      getProbeAllStatus
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
    useI18n: () => ({
      t: (key: string, params?: Record<string, unknown>) => (
        params ? `${key}:${Object.values(params).join('/')}` : key
      )
    })
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
  afterEach(() => {
    vi.useRealTimers()
  })

  beforeEach(() => {
    localStorage.clear()
    for (const mock of [
      listAccounts,
      listWithEtag,
      getAccountById,
      getBatchTodayStats,
      getAllProxies,
      getAllGroups,
      startProbeAll,
      getProbeAllStatus,
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
    getProbeAllStatus.mockResolvedValue({
      run_id: '',
      running: false,
      total: 0,
      completed: 0,
      succeeded: 0,
      failed: 0,
      started_at: null,
      finished_at: null,
      status_counts: {}
    })
    startProbeAll.mockResolvedValue({
      run_id: 'probe-1',
      running: true,
      total: 2,
      completed: 0,
      succeeded: 0,
      failed: 0,
      started_at: '2026-07-22T12:00:00Z',
      finished_at: null,
      status_counts: {}
    })
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

  it('shows and refreshes probe-all only for the Grok platform filter', async () => {
    const wrapper = mountView()
    await flushPromises()
    const setupState = wrapper.vm.$.setupState as {
      params: { platform: string }
    }

    expect(wrapper.find('[data-test="grok-probe-all"]').exists()).toBe(false)
    expect(getProbeAllStatus).not.toHaveBeenCalled()

    setupState.params.platform = 'openai'
    await nextTick()
    expect(wrapper.find('[data-test="grok-probe-all"]').exists()).toBe(false)
    expect(getProbeAllStatus).not.toHaveBeenCalled()

    setupState.params.platform = 'grok'
    await nextTick()
    await flushPromises()
    expect(wrapper.find('[data-test="grok-probe-all"]').exists()).toBe(true)
    expect(getProbeAllStatus).toHaveBeenCalledTimes(1)

    setupState.params.platform = ''
    await nextTick()
    expect(wrapper.find('[data-test="grok-probe-all"]').exists()).toBe(false)

    setupState.params.platform = 'grok'
    await nextTick()
    await flushPromises()
    expect(wrapper.find('[data-test="grok-probe-all"]').exists()).toBe(true)
    expect(getProbeAllStatus).toHaveBeenCalledTimes(2)
    wrapper.unmount()
  })

  it('starts one shared probe job, displays progress, and refreshes on completion', async () => {
    const wrapper = mountView()
    await flushPromises()
    const setupState = wrapper.vm.$.setupState as {
      params: { platform: string }
      refreshGrokProbeAllStatus: () => Promise<void>
    }
    setupState.params.platform = 'grok'
    await nextTick()
    await flushPromises()

    const probeAllButton = wrapper.get('[data-test="grok-probe-all"]')
    await probeAllButton.trigger('click')
    await probeAllButton.trigger('click')
    await flushPromises()

    expect(startProbeAll).toHaveBeenCalledTimes(1)
    expect(wrapper.get('[data-test="grok-probe-all"]').text()).toContain(
      'admin.accounts.grokProbeAllRunning:0/2'
    )

    getProbeAllStatus.mockResolvedValueOnce({
      run_id: 'probe-1',
      running: false,
      total: 2,
      completed: 2,
      succeeded: 1,
      failed: 1,
      started_at: '2026-07-22T12:00:00Z',
      finished_at: '2026-07-22T12:00:03Z',
      status_counts: { '200': 1, '402': 1 }
    })
    await setupState.refreshGrokProbeAllStatus()
    await flushPromises()

    expect(listWithEtag).toHaveBeenCalledTimes(1)
    expect(showSuccess).toHaveBeenCalledWith('admin.accounts.grokProbeAllCompleted:1/1')
    expect(wrapper.get('[data-test="grok-probe-all"]').attributes('title')).toContain('402: 1')
    wrapper.unmount()
  })

  it('stops probe status polling after leaving the Grok platform filter', async () => {
    vi.useFakeTimers()
    getProbeAllStatus.mockResolvedValue({
      run_id: 'probe-running',
      running: true,
      total: 2,
      completed: 0,
      succeeded: 0,
      failed: 0,
      started_at: '2026-07-22T12:00:00Z',
      finished_at: null,
      status_counts: {}
    })
    const wrapper = mountView()
    await flushPromises()
    const setupState = wrapper.vm.$.setupState as {
      params: { platform: string }
    }

    setupState.params.platform = 'grok'
    await nextTick()
    await flushPromises()
    expect(getProbeAllStatus).toHaveBeenCalledTimes(1)

    setupState.params.platform = 'openai'
    await nextTick()
    await vi.advanceTimersByTimeAsync(2500)

    expect(wrapper.find('[data-test="grok-probe-all"]').exists()).toBe(false)
    expect(getProbeAllStatus).toHaveBeenCalledTimes(1)
    wrapper.unmount()
  })
})
