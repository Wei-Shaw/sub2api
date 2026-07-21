import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'

import AccountsView from '../AccountsView.vue'
import type { UpstreamQuotaQueryResult } from '@/types'
import {
  getUpstreamBillingRateHistoryCache,
  invalidateUpstreamBillingRateHistoryCache,
  setUpstreamBillingRateHistoryCache
} from '@/components/account/upstreamBillingRateHistoryCache'

const {
  authUser,
  listAccounts,
  listWithEtag,
  getUpstreamBillingRatesWithEtag,
  getBatchTodayStats,
  getUpstreamBillingProbeSettings,
  getAllProxies,
  getAllGroups,
  getAccountById,
  getUpstreamSiteLogo,
  queryUpstreamQuota,
  probeUpstreamBilling,
  getUsage,
  deleteAccount,
  showToast,
  hideToast,
  showError,
  showSuccess,
  createObjectURL,
  revokeObjectURL
} = vi.hoisted(() => ({
  authUser: { id: 99 },
  listAccounts: vi.fn(),
  listWithEtag: vi.fn(),
  getUpstreamBillingRatesWithEtag: vi.fn(),
  getBatchTodayStats: vi.fn(),
  getUpstreamBillingProbeSettings: vi.fn(),
  getAllProxies: vi.fn(),
  getAllGroups: vi.fn(),
  getAccountById: vi.fn(),
  getUpstreamSiteLogo: vi.fn(),
  queryUpstreamQuota: vi.fn(),
  probeUpstreamBilling: vi.fn(),
  getUsage: vi.fn(),
  deleteAccount: vi.fn(),
  showToast: vi.fn(),
  hideToast: vi.fn(),
  showError: vi.fn(),
  showSuccess: vi.fn(),
  createObjectURL: vi.fn(() => 'blob:upstream-site-logo'),
  revokeObjectURL: vi.fn()
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    accounts: {
      list: listAccounts,
      listWithEtag,
      getUpstreamBillingRatesWithEtag,
      getBatchTodayStats,
      getUpstreamBillingProbeSettings,
      getById: getAccountById,
      getUpstreamSiteLogo,
      queryUpstreamQuota,
      probeUpstreamBilling,
      getUsage,
      delete: deleteAccount,
      batchClearError: vi.fn(),
      batchRefresh: vi.fn(),
      toggleSchedulable: vi.fn()
    },
    proxies: { getAll: getAllProxies },
    groups: { getAll: getAllGroups }
  }
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({
    showToast,
    hideToast,
    showError,
    showSuccess
  })
}))

vi.mock('@/stores/auth', () => ({
  useAuthStore: () => ({
    user: authUser,
    token: 'test-token',
    isSimpleMode: false
  })
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({ t: (key: string) => key })
  }
})

const DataTableStub = {
  props: ['data', 'loading'],
  data: () => ({ showRows: true, reversed: false }),
  template: `
    <div data-test="accounts-table" :data-loading="String(loading)">
      <button data-test="toggle-rows" @click="showRows = !showRows">toggle</button>
      <button data-test="reverse-rows" @click="reversed = !reversed">reverse</button>
      <div v-if="showRows">
        <div v-for="row in (reversed ? data.slice().reverse() : data)" :key="row.id" data-test="virtual-row" :data-account-id="row.id">
          <slot name="cell-platform_type" :row="row" />
          <slot name="cell-upstream_billing_rate" :row="row" />
          <slot name="cell-usage" :row="row" />
        </div>
      </div>
    </div>
  `
}

const UpstreamBillingRateCellStub = {
  props: ['account', 'quotaResult', 'quotaError', 'quotaLoading', 'rateError', 'rateErrorAt', 'rateFeedback', 'quotaFeedback'],
  emits: ['query-quota', 'probe', 'open-history'],
  template: `
    <div
      data-test="quota-cell"
      :data-account-id="account.id"
      :data-loading="String(quotaLoading)"
      :data-rate-error="String(rateError)"
      :data-rate-error-at="String(rateErrorAt ?? '')"
      :data-rate-feedback="String(rateFeedback)"
      :data-quota-feedback="String(quotaFeedback)"
    >
      <span data-test="quota-result">{{ quotaResult?.quota?.remaining ?? '' }}</span>
      <span data-test="quota-error">{{ quotaError ?? '' }}</span>
      <button data-test="query-quota" @click="$emit('query-quota')">query</button>
      <button data-test="probe-rate" @click="$emit('probe')">probe</button>
      <button data-test="open-rate-history" @click="$emit('open-history')">history</button>
    </div>
  `
}

const AccountUsageCellStub = {
  props: ['account', 'upstreamQuotaResult'],
  template: `
    <div data-test="usage-cell" :data-account-id="account.id">
      <span data-test="usage-window-count">{{ upstreamQuotaResult?.quota?.subscription?.windows?.length ?? 0 }}</span>
    </div>
  `
}

const EditAccountModalStub = {
  emits: ['updated'],
  template: `
    <button
      data-test="account-updated"
      @click="$emit('updated', { id: 7, name: 'updated', platform: 'openai', type: 'apikey', status: 'active', schedulable: true })"
    >updated</button>
  `
}

const BulkEditAccountModalStub = {
  emits: ['updated'],
  template: '<button data-test="bulk-updated" @click="$emit(\'updated\')">updated</button>'
}

const ImportDataModalStub = {
  emits: ['imported'],
  template: '<button data-test="data-imported" @click="$emit(\'imported\')">imported</button>'
}

const PaginationStub = {
  props: ['page'],
  emits: ['update:page'],
  template: '<button data-test="go-page-2" :data-page="page" @click="$emit(\'update:page\', 2)">page 2</button>'
}

const AccountBulkActionsBarStub = {
  props: ['selectedIds', 'queryingUpstreamQuota'],
  emits: ['query-upstream-quota'],
  template: `
    <button
      v-if="selectedIds.length"
      data-test="bulk-query-quota"
      :data-loading="String(queryingUpstreamQuota)"
      @click="$emit('query-upstream-quota')"
    >query balances</button>
  `
}

const account = (id: number, overrides: Record<string, unknown> = {}) => ({
  id,
  name: `account-${id}`,
  platform: 'openai',
  type: 'apikey',
  status: 'active',
  schedulable: true,
  created_at: '2026-07-17T00:00:00Z',
  updated_at: '2026-07-17T00:00:00Z',
  ...overrides
})

const quotaResultFor = (accountID: number, remaining: number): UpstreamQuotaQueryResult => ({
  account_id: accountID,
  observed_at: '2026-07-17T00:00:00Z',
  quota: {
    provider: 'sub2api',
    mode: 'balance',
    unit: 'USD',
    remaining
  }
})

const quotaResult = quotaResultFor(7, 80)
const quotaCacheKey = (userID: number, accountID: number) =>
  `sub2api:admin:upstream-quota:v2:${userID}:${accountID}`

const mountView = () => mount(AccountsView, {
  global: {
    stubs: {
      AppLayout: { template: '<div><slot /></div>' },
      TablePageLayout: { template: '<div><slot name="table" /><slot name="pagination" /></div>' },
      DataTable: DataTableStub,
      UpstreamBillingRateCell: UpstreamBillingRateCellStub,
      Pagination: PaginationStub,
      ConfirmDialog: true,
      AccountTableActions: true,
      AccountTableFilters: true,
      AccountBulkActionsBar: AccountBulkActionsBarStub,
      AccountActionMenu: true,
      ImportDataModal: ImportDataModalStub,
      ReAuthAccountModal: true,
      AccountTestModal: true,
      AccountStatsModal: true,
      UpstreamBillingRateHistoryDialog: {
        props: ['show', 'account'],
        template: '<div v-if="show" data-test="rate-history-dialog" :data-account-id="account?.id" />'
      },
      ScheduledTestsPanel: true,
      SyncFromCrsModal: true,
      TempUnschedStatusModal: true,
      ErrorPassthroughRulesModal: true,
      TLSFingerprintProfilesModal: true,
      CreateAccountModal: true,
      EditAccountModal: EditAccountModalStub,
      BulkEditAccountModal: BulkEditAccountModalStub,
      PlatformTypeBadge: true,
      AccountCapacityCell: true,
      AccountStatusIndicator: true,
      AccountTodayStatsCell: true,
      AccountGroupsCell: true,
      AccountUsageCell: AccountUsageCellStub,
      HelpTooltip: true,
      Icon: true
    }
  }
})

describe('admin AccountsView upstream quota state', () => {
  afterEach(() => {
    vi.restoreAllMocks()
    vi.useRealTimers()
  })

  beforeEach(() => {
    Object.defineProperty(URL, 'createObjectURL', { configurable: true, value: createObjectURL })
    Object.defineProperty(URL, 'revokeObjectURL', { configurable: true, value: revokeObjectURL })
    localStorage.clear()
    invalidateUpstreamBillingRateHistoryCache()
    authUser.id = 99
    for (const mock of [
      listAccounts,
      listWithEtag,
      getUpstreamBillingRatesWithEtag,
      getBatchTodayStats,
      getUpstreamBillingProbeSettings,
      getAllProxies,
      getAllGroups,
      getAccountById,
      getUpstreamSiteLogo,
      queryUpstreamQuota,
      probeUpstreamBilling,
      getUsage,
      deleteAccount,
      showToast,
      hideToast,
      showError,
      showSuccess,
      createObjectURL,
      revokeObjectURL
    ]) mock.mockReset()
    createObjectURL.mockReturnValue('blob:upstream-site-logo')

    listAccounts.mockResolvedValue({
      items: [account(7), account(11)],
      total: 2,
      page: 1,
      page_size: 20,
      pages: 1
    })
    listWithEtag.mockResolvedValue({ notModified: true, etag: null, data: null })
    getUpstreamBillingRatesWithEtag.mockResolvedValue({ notModified: true, etag: null, data: null })
    getBatchTodayStats.mockResolvedValue({ stats: {} })
    getUpstreamBillingProbeSettings.mockResolvedValue({ enabled: true, interval_minutes: 30 })
    getAllProxies.mockResolvedValue([])
    getAllGroups.mockResolvedValue([])
    getAccountById.mockImplementation((id: number) => Promise.resolve(account(id)))
	getUpstreamSiteLogo.mockResolvedValue(new Blob(['logo'], { type: 'image/png' }))
  })

  it('fetches selected balances in sequential groups of four with batch progress', async () => {
    const selectedAccounts = Array.from({ length: 5 }, (_, index) => account(index + 1))
    listAccounts.mockResolvedValueOnce({
      items: selectedAccounts.slice(0, 4),
      total: selectedAccounts.length,
      page: 1,
      page_size: 20,
      pages: 1
    })
    const firstBatchResolvers = new Map<number, (result: UpstreamQuotaQueryResult) => void>()
    queryUpstreamQuota.mockImplementation((id: number) => (
      id === 5
        ? Promise.resolve(quotaResultFor(id, 50))
        : new Promise(resolve => { firstBatchResolvers.set(id, resolve) })
    ))
    showToast
      .mockReturnValueOnce('quota-batch-start')
      .mockReturnValueOnce('quota-batch-progress')

    const wrapper = mountView()
    await flushPromises()
    const setupState = wrapper.vm.$.setupState as unknown as {
      setSelectedIds: (ids: number[]) => void
    }
    setupState.setSelectedIds(selectedAccounts.map(({ id }) => id))
    await wrapper.vm.$nextTick()
    await wrapper.get('[data-test="bulk-query-quota"]').trigger('click')
    await wrapper.vm.$nextTick()

    expect(queryUpstreamQuota).toHaveBeenCalledTimes(4)
    expect(queryUpstreamQuota.mock.calls.map(([id]) => id)).toEqual([1, 2, 3, 4])
    expect(wrapper.get('[data-test="bulk-query-quota"]').attributes('data-loading')).toBe('true')
    expect(showToast).toHaveBeenNthCalledWith(1, 'info', 'admin.accounts.upstreamBilling.quotaBatchStarted')

    for (const [id, resolve] of firstBatchResolvers) resolve(quotaResultFor(id, 100 - id))
    await flushPromises()

    expect(queryUpstreamQuota).toHaveBeenCalledTimes(5)
    expect(queryUpstreamQuota).toHaveBeenLastCalledWith(5)
    expect(getAccountById).toHaveBeenCalledOnce()
    expect(getAccountById).toHaveBeenCalledWith(5)
    expect(hideToast).toHaveBeenNthCalledWith(1, 'quota-batch-start')
    expect(showToast).toHaveBeenNthCalledWith(2, 'info', 'admin.accounts.upstreamBilling.quotaBatchProgress')
    expect(hideToast).toHaveBeenLastCalledWith('quota-batch-progress')
    expect(showSuccess).toHaveBeenCalledWith('admin.accounts.upstreamBilling.quotaBatchCompleted')
    expect(wrapper.get('[data-test="bulk-query-quota"]').attributes('data-loading')).toBe('false')
    wrapper.unmount()
  })

  it('queries only the selected account and retains keyed state across remount, reorder, and refresh failure', async () => {
    let resolveQuery!: (result: UpstreamQuotaQueryResult) => void
    queryUpstreamQuota.mockReturnValueOnce(new Promise(resolve => { resolveQuery = resolve }))

    const wrapper = mountView()
    await flushPromises()

    expect(queryUpstreamQuota).not.toHaveBeenCalled()
    const cell = (id: number) => wrapper.get(`[data-test="quota-cell"][data-account-id="${id}"]`)
    await cell(7).get('[data-test="query-quota"]').trigger('click')
    await wrapper.vm.$nextTick()

    expect(queryUpstreamQuota).toHaveBeenCalledOnce()
    expect(queryUpstreamQuota).toHaveBeenCalledWith(7)
    expect(cell(7).attributes('data-loading')).toBe('true')
    expect(cell(11).attributes('data-loading')).toBe('false')

    resolveQuery(quotaResult)
    await flushPromises()
    expect(cell(7).get('[data-test="quota-result"]').text()).toBe('80')
    expect(cell(11).get('[data-test="quota-result"]').text()).toBe('')
    expect(JSON.parse(localStorage.getItem(quotaCacheKey(99, 7)) ?? 'null').result).toEqual(quotaResult)

    await wrapper.get('[data-test="toggle-rows"]').trigger('click')
    expect(wrapper.findAll('[data-test="quota-cell"]')).toHaveLength(0)
    await wrapper.get('[data-test="toggle-rows"]').trigger('click')
    await wrapper.get('[data-test="reverse-rows"]').trigger('click')
    expect(wrapper.findAll('[data-test="quota-cell"]').map(node => node.attributes('data-account-id'))).toEqual(['11', '7'])
    expect(cell(7).get('[data-test="quota-result"]').text()).toBe('80')

    queryUpstreamQuota.mockRejectedValueOnce({ message: 'refresh failed' })
    await cell(7).get('[data-test="query-quota"]').trigger('click')
    await flushPromises()
    expect(cell(7).get('[data-test="quota-result"]').text()).toBe('80')
    expect(cell(7).get('[data-test="quota-error"]').text()).toBe('refresh failed')
    expect(JSON.parse(localStorage.getItem(quotaCacheKey(99, 7)) ?? 'null').result).toEqual(quotaResult)

    queryUpstreamQuota.mockResolvedValueOnce({ ...quotaResult, quota: null })
    await cell(7).get('[data-test="query-quota"]').trigger('click')
    await flushPromises()
    expect(cell(7).get('[data-test="quota-result"]').text()).toBe('80')
    expect(cell(7).get('[data-test="quota-error"]').text()).toBe('admin.accounts.upstreamBilling.noQuotaData')
    expect(JSON.parse(localStorage.getItem(quotaCacheKey(99, 7)) ?? 'null').result).toEqual(quotaResult)

    expect(probeUpstreamBilling).not.toHaveBeenCalled()
    expect(getUsage).not.toHaveBeenCalled()
    expect(listAccounts).toHaveBeenCalledOnce()

    await wrapper.get('[data-test="account-updated"]').trigger('click')
    await wrapper.vm.$nextTick()
    expect(cell(7).get('[data-test="quota-result"]').text()).toBe('')
    expect(cell(7).get('[data-test="quota-error"]').text()).toBe('')
    expect(localStorage.getItem(quotaCacheKey(99, 7))).toBeNull()

    queryUpstreamQuota.mockResolvedValueOnce(quotaResult)
    await cell(7).get('[data-test="query-quota"]').trigger('click')
    await flushPromises()
    expect(cell(7).get('[data-test="quota-result"]').text()).toBe('80')
    expect(localStorage.getItem(quotaCacheKey(99, 7))).not.toBeNull()
    await wrapper.get('[data-test="data-imported"]').trigger('click')
    await flushPromises()
    expect(cell(7).get('[data-test="quota-result"]').text()).toBe('')
    expect(localStorage.getItem(quotaCacheKey(99, 7))).toBeNull()

    let resolveAfterAccountUpdate!: (result: UpstreamQuotaQueryResult) => void
    queryUpstreamQuota.mockReturnValueOnce(new Promise(resolve => { resolveAfterAccountUpdate = resolve }))
    await cell(7).get('[data-test="query-quota"]').trigger('click')
    await wrapper.vm.$nextTick()
    await wrapper.get('[data-test="account-updated"]').trigger('click')
    resolveAfterAccountUpdate(quotaResult)
    await flushPromises()
    expect(cell(7).get('[data-test="quota-result"]').text()).toBe('')
    wrapper.unmount()
  })

  it('opens one account history and invalidates cache only when Base URL changes', async () => {
    listAccounts.mockResolvedValue({
      items: [account(7, { credentials: { base_url: 'https://upstream.example/v1' } }), account(11)],
      total: 2,
      page: 1,
      page_size: 20,
      pages: 1
    })
    const cachedHistory = { account_id: 7, range_days: 90 as const, truncated: false, events: [] }
    setUpstreamBillingRateHistoryCache(7, 90, { data: cachedHistory, etag: '"history-v1"' })
    const wrapper = mountView()
    await flushPromises()

    await wrapper.get('[data-test="quota-cell"][data-account-id="7"] [data-test="open-rate-history"]').trigger('click')
    expect(wrapper.get('[data-test="rate-history-dialog"]').attributes('data-account-id')).toBe('7')

    const setupState = wrapper.vm.$.setupState as unknown as {
      handleAccountUpdated: (updated: ReturnType<typeof account>) => void
    }
    setupState.handleAccountUpdated(account(7, {
      credentials: { base_url: 'https://upstream.example/v1' }
    }))
    expect(getUpstreamBillingRateHistoryCache(7, 90)).toBeDefined()

    setupState.handleAccountUpdated(account(7, {
      credentials: { base_url: 'https://other.example/v1' }
    }))
    expect(getUpstreamBillingRateHistoryCache(7, 90)).toBeUndefined()
    wrapper.unmount()
  })

  it('shares and restores one queried subscription result in the usage window', async () => {
    const subscriptionResult: UpstreamQuotaQueryResult = {
      account_id: 7,
      observed_at: '2026-07-19T00:00:00Z',
      quota: {
        provider: 'sub2api',
        mode: 'quota',
        unit: 'USD',
        remaining: 80,
        subscription: {
          plan_name: 'Pro Monthly',
          remaining: 8,
          expires_at: '2026-08-01T00:00:00Z',
          windows: [
            { name: 'daily', used: 2, limit: 10, remaining: 8, reset_at: '2026-07-20T00:00:00Z' }
          ]
        }
      }
    }
    queryUpstreamQuota.mockResolvedValueOnce(subscriptionResult)

    const first = mountView()
    await flushPromises()
    await first.get('[data-test="quota-cell"][data-account-id="7"] [data-test="query-quota"]').trigger('click')
    await flushPromises()

    expect(first.get('[data-test="usage-cell"][data-account-id="7"] [data-test="usage-window-count"]').text())
      .toBe('1')
    expect(JSON.parse(localStorage.getItem(quotaCacheKey(99, 7)) ?? 'null').result)
      .toEqual(subscriptionResult)
    first.unmount()

    const reloaded = mountView()
    await flushPromises()
    expect(reloaded.get('[data-test="usage-cell"][data-account-id="7"] [data-test="usage-window-count"]').text())
      .toBe('1')
    expect(queryUpstreamQuota).toHaveBeenCalledOnce()
    reloaded.unmount()
  })

  it('silently refreshes server ordering after a manual rate probe', async () => {
    localStorage.setItem('account-table-sort', JSON.stringify({ key: 'upstream_billing_rate', order: 'asc' }))
    listAccounts
      .mockResolvedValueOnce({ items: [account(7), account(11)], total: 40, page: 1, page_size: 20, pages: 2 })
      .mockResolvedValueOnce({ items: [account(7), account(11)], total: 40, page: 2, page_size: 20, pages: 2 })
      .mockResolvedValueOnce({
        items: [account(11), account(7, { updated_at: '2026-07-17T00:01:00Z' })],
        total: 40,
        page: 2,
        page_size: 20,
        pages: 2
      })
    getUpstreamBillingRatesWithEtag.mockResolvedValueOnce({
      notModified: false,
      etag: '"rate-v1"',
      data: {
        items: [
          { account_id: 11, snapshot: { status: 'unsupported', last_attempt_at: '2026-07-17T00:00:00Z', next_probe_at: '2026-07-17T00:30:00Z' } },
          { account_id: 7, snapshot: { status: 'ok', data: { effective_rate_multiplier: 0.5 }, last_attempt_at: '2026-07-17T00:00:00Z', next_probe_at: '2026-07-17T00:30:00Z' } }
        ],
        total: 40,
        page: 2,
        page_size: 20
      }
    })
    probeUpstreamBilling.mockResolvedValueOnce({
      account_id: 7,
      snapshot: {
        status: 'ok',
        data: { effective_rate_multiplier: 0.5 },
        last_attempt_at: '2026-07-17T00:00:00Z',
        next_probe_at: '2026-07-17T00:30:00Z'
      }
    })

    const wrapper = mountView()
    await flushPromises()
    await wrapper.get('[data-test="go-page-2"]').trigger('click')
    await flushPromises()
    await wrapper.get('[data-test="quota-cell"][data-account-id="7"] [data-test="probe-rate"]').trigger('click')
    await flushPromises()

    expect(listAccounts).toHaveBeenCalledTimes(3)
    expect(listAccounts.mock.calls[2]?.[0]).toBe(2)
    expect(listWithEtag).not.toHaveBeenCalled()
    expect(getBatchTodayStats).toHaveBeenCalledTimes(2)
    expect(wrapper.get('[data-test="accounts-table"]').attributes('data-loading')).toBe('false')
    expect(wrapper.findAll('[data-test="virtual-row"]')).toHaveLength(2)
    expect(wrapper.get('[data-test="go-page-2"]').attributes('data-page')).toBe('2')

    await flushPromises()
    expect(wrapper.findAll('[data-test="virtual-row"]').map(row => row.attributes('data-account-id'))).toEqual(['11', '7'])
    expect(wrapper.get('[data-test="go-page-2"]').attributes('data-page')).toBe('2')
    wrapper.unmount()
  })

  it('polls persisted rates every five minutes without list, quota, or stats requests', async () => {
    vi.useFakeTimers()
    getUpstreamBillingRatesWithEtag.mockResolvedValue({
      notModified: false,
      etag: '"rate-v2"',
      data: {
        items: [
          { account_id: 7, snapshot: { status: 'ok', data: { effective_rate_multiplier: 0.75 }, last_attempt_at: '2026-07-17T00:00:00Z', next_probe_at: '2026-07-17T00:30:00Z' } },
          { account_id: 11, snapshot: null }
        ],
        total: 2,
        page: 1,
        page_size: 20
      }
    })

    const wrapper = mountView()
    await flushPromises()
    expect(getUpstreamBillingRatesWithEtag).not.toHaveBeenCalled()
    const initialStatsCalls = getBatchTodayStats.mock.calls.length

    await vi.advanceTimersByTimeAsync(5 * 60_000)
    await flushPromises()

    expect(getUpstreamBillingRatesWithEtag).toHaveBeenCalledOnce()
    expect(listAccounts).toHaveBeenCalledOnce()
    expect(getBatchTodayStats).toHaveBeenCalledTimes(initialStatsCalls)
    expect(queryUpstreamQuota).not.toHaveBeenCalled()
    wrapper.unmount()
  })

  it('drops a rate response that belongs to a page changed while it was in flight', async () => {
    vi.useFakeTimers()
    listAccounts
      .mockResolvedValueOnce({ items: [account(7), account(11)], total: 40, page: 1, page_size: 20, pages: 2 })
      .mockResolvedValueOnce({ items: [account(7), account(11)], total: 40, page: 2, page_size: 20, pages: 2 })
    let resolveRate!: (value: unknown) => void
    getUpstreamBillingRatesWithEtag.mockReturnValueOnce(new Promise(resolve => { resolveRate = resolve }))

    const wrapper = mountView()
    await flushPromises()
    vi.advanceTimersByTime(5 * 60_000)
    await wrapper.get('[data-test="go-page-2"]').trigger('click')
    await flushPromises()

    resolveRate({
      notModified: false,
      etag: '"stale-rate"',
      data: {
        items: [{ account_id: 7, snapshot: null }, { account_id: 11, snapshot: null }],
        total: 2,
        page: 1,
        page_size: 20
      }
    })
    await flushPromises()

    expect(listAccounts).toHaveBeenCalledTimes(2)
    expect(listAccounts.mock.calls[1]?.[0]).toBe(2)
    wrapper.unmount()
  })

  it('invalidates cached and in-flight quota results when a bulk edit may change base URL or proxy', async () => {
    queryUpstreamQuota
      .mockResolvedValueOnce(quotaResultFor(7, 80))
      .mockResolvedValueOnce(quotaResultFor(11, 60))

    const wrapper = mountView()
    await flushPromises()
    const cell = (id: number) => wrapper.get(`[data-test="quota-cell"][data-account-id="${id}"]`)

    await cell(7).get('[data-test="query-quota"]').trigger('click')
    await cell(11).get('[data-test="query-quota"]').trigger('click')
    await flushPromises()
    expect(cell(7).get('[data-test="quota-result"]').text()).toBe('80')
    expect(cell(11).get('[data-test="quota-result"]').text()).toBe('60')
    expect(localStorage.getItem(quotaCacheKey(99, 7))).not.toBeNull()
    expect(localStorage.getItem(quotaCacheKey(99, 11))).not.toBeNull()

    await wrapper.get('[data-test="bulk-updated"]').trigger('click')
    await flushPromises()
    expect(cell(7).get('[data-test="quota-result"]').text()).toBe('')
    expect(cell(11).get('[data-test="quota-result"]').text()).toBe('')
    expect(localStorage.getItem(quotaCacheKey(99, 7))).toBeNull()
    expect(localStorage.getItem(quotaCacheKey(99, 11))).toBeNull()

    let resolveStaleQuery!: (result: UpstreamQuotaQueryResult) => void
    queryUpstreamQuota.mockReturnValueOnce(new Promise(resolve => { resolveStaleQuery = resolve }))
    await cell(7).get('[data-test="query-quota"]').trigger('click')
    await wrapper.vm.$nextTick()
    await wrapper.get('[data-test="bulk-updated"]').trigger('click')
    resolveStaleQuery(quotaResultFor(7, 40))
    await flushPromises()
    expect(cell(7).get('[data-test="quota-result"]').text()).toBe('')
    wrapper.unmount()
  })

  it('discards a pending result when refreshed account data changes upstream identity', async () => {
    let resolveQuery!: (result: UpstreamQuotaQueryResult) => void
    queryUpstreamQuota.mockReturnValueOnce(new Promise(resolve => { resolveQuery = resolve }))

    const wrapper = mountView()
    await flushPromises()
    await wrapper.get('[data-test="quota-cell"][data-account-id="7"] [data-test="query-quota"]').trigger('click')
    await wrapper.vm.$nextTick()

    const setupState = wrapper.vm.$.setupState as unknown as {
      accounts: ReturnType<typeof account>[]
    }
    setupState.accounts = [
      account(7, { credentials: { base_url: 'https://changed.example.com' } }),
      account(11)
    ]
    await wrapper.vm.$nextTick()

    resolveQuery(quotaResult)
    await flushPromises()
    expect(wrapper.get('[data-test="quota-cell"][data-account-id="7"] [data-test="quota-result"]').text()).toBe('')
    expect(localStorage.getItem(quotaCacheKey(99, 7))).toBeNull()
    wrapper.unmount()
  })

  it('clears cache for successful accounts when a bulk delete only partially succeeds', async () => {
    queryUpstreamQuota
      .mockResolvedValueOnce(quotaResultFor(7, 80))
      .mockResolvedValueOnce(quotaResultFor(11, 60))
    deleteAccount.mockImplementation((id: number) => (
      id === 7 ? Promise.resolve() : Promise.reject(new Error('delete failed'))
    ))
    vi.spyOn(window, 'confirm').mockReturnValue(true)
    vi.spyOn(console, 'error').mockImplementation(() => {})

    const wrapper = mountView()
    await flushPromises()
    for (const id of [7, 11]) {
      await wrapper.get(`[data-test="quota-cell"][data-account-id="${id}"] [data-test="query-quota"]`).trigger('click')
    }
    await flushPromises()

    const setupState = wrapper.vm.$.setupState as unknown as {
      setSelectedIds: (ids: number[]) => void
      handleBulkDelete: () => Promise<void>
    }
    setupState.setSelectedIds([7, 11])
    await setupState.handleBulkDelete()
    await flushPromises()

    expect(deleteAccount).toHaveBeenCalledTimes(2)
    expect(localStorage.getItem(quotaCacheKey(99, 7))).toBeNull()
    expect(localStorage.getItem(quotaCacheKey(99, 11))).not.toBeNull()
    wrapper.unmount()
  })

  it('hydrates the last successful quota from user-scoped local storage', async () => {
    queryUpstreamQuota.mockResolvedValueOnce(quotaResult)

    const first = mountView()
    await flushPromises()
    await first.get('[data-test="quota-cell"][data-account-id="7"] [data-test="query-quota"]').trigger('click')
    await flushPromises()
    expect(localStorage.getItem(quotaCacheKey(99, 7))).not.toBeNull()
    const validCacheEntry = JSON.parse(localStorage.getItem(quotaCacheKey(99, 7)) ?? 'null')
    first.unmount()

    listAccounts.mockResolvedValue({
      items: [account(7, { updated_at: '2026-07-17T01:00:00Z' }), account(11)],
      total: 2,
      page: 1,
      page_size: 20,
      pages: 1
    })
    const reloaded = mountView()
    await flushPromises()
    expect(reloaded.get('[data-test="quota-cell"][data-account-id="7"] [data-test="quota-result"]').text()).toBe('80')
    expect(queryUpstreamQuota).toHaveBeenCalledOnce()
    reloaded.unmount()

    authUser.id = 100
    const otherAdmin = mountView()
    await flushPromises()
    expect(otherAdmin.get('[data-test="quota-cell"][data-account-id="7"] [data-test="quota-result"]').text()).toBe('')
    otherAdmin.unmount()

    authUser.id = 99
    listAccounts.mockResolvedValue({
      items: [account(7, { credentials: { base_url: 'https://changed.example.com' } }), account(11)],
      total: 2,
      page: 1,
      page_size: 20,
      pages: 1
    })
    const identityChanged = mountView()
    await flushPromises()
    expect(identityChanged.get('[data-test="quota-cell"][data-account-id="7"] [data-test="quota-result"]').text()).toBe('')
    expect(localStorage.getItem(quotaCacheKey(99, 7))).toBeNull()
    identityChanged.unmount()

    listAccounts.mockResolvedValue({
      items: [account(7), account(11)],
      total: 2,
      page: 1,
      page_size: 20,
      pages: 1
    })
    localStorage.setItem(quotaCacheKey(99, 7), JSON.stringify({
      ...validCacheEntry,
      result: { ...validCacheEntry.result, account_id: 11 }
    }))
    const wrongAccount = mountView()
    await flushPromises()
    expect(wrongAccount.get('[data-test="quota-cell"][data-account-id="7"] [data-test="quota-result"]').text()).toBe('')
    expect(localStorage.getItem(quotaCacheKey(99, 7))).toBeNull()
    wrongAccount.unmount()

    localStorage.setItem(quotaCacheKey(99, 7), JSON.stringify({
      ...validCacheEntry,
      result: {
        ...validCacheEntry.result,
        quota: { ...validCacheEntry.result.quota, mode: ['balance'] }
      }
    }))
    const invalidShape = mountView()
    await flushPromises()
    expect(invalidShape.get('[data-test="quota-cell"][data-account-id="7"] [data-test="quota-result"]').text()).toBe('')
    expect(localStorage.getItem(quotaCacheKey(99, 7))).toBeNull()
    invalidShape.unmount()

    localStorage.setItem(quotaCacheKey(99, 7), '{broken')
    const malformed = mountView()
    await flushPromises()
    expect(malformed.get('[data-test="quota-cell"][data-account-id="7"] [data-test="quota-result"]').text()).toBe('')
    expect(localStorage.getItem(quotaCacheKey(99, 7))).toBeNull()
    malformed.unmount()
  })

  it('does not write a pending result into another administrator cache', async () => {
    let resolveQuery!: (result: UpstreamQuotaQueryResult) => void
    queryUpstreamQuota.mockReturnValueOnce(new Promise(resolve => { resolveQuery = resolve }))

    const wrapper = mountView()
    await flushPromises()
    await wrapper.get('[data-test="quota-cell"][data-account-id="7"] [data-test="query-quota"]').trigger('click')
    await wrapper.vm.$nextTick()

    authUser.id = 100
    resolveQuery(quotaResult)
    await flushPromises()

    expect(wrapper.get('[data-test="quota-cell"][data-account-id="7"] [data-test="quota-result"]').text()).toBe('')
    expect(localStorage.getItem(quotaCacheKey(99, 7))).toBeNull()
    expect(localStorage.getItem(quotaCacheKey(100, 7))).toBeNull()
    wrapper.unmount()
  })

  it('keeps a successful result in memory when local storage is unavailable', async () => {
    queryUpstreamQuota.mockResolvedValueOnce(quotaResult)
    const nativeSetItem = Storage.prototype.setItem
    vi.spyOn(Storage.prototype, 'setItem').mockImplementation(function (key, value) {
      if (key === quotaCacheKey(99, 7)) throw new DOMException('storage disabled', 'SecurityError')
      return nativeSetItem.call(this, key, value)
    })

    const wrapper = mountView()
    await flushPromises()
    await wrapper.get('[data-test="quota-cell"][data-account-id="7"] [data-test="query-quota"]').trigger('click')
    await flushPromises()

    expect(wrapper.get('[data-test="quota-cell"][data-account-id="7"] [data-test="quota-result"]').text()).toBe('80')
    expect(localStorage.getItem(quotaCacheKey(99, 7))).toBeNull()
    wrapper.unmount()
  })

  it('shows keyed rate and quota outcomes for one second without stale timers winning', async () => {
    vi.useFakeTimers()
    probeUpstreamBilling
      .mockResolvedValueOnce({
        account_id: 7,
        snapshot: {
          status: 'ok',
          last_attempt_at: '2026-07-17T00:00:00Z',
          next_probe_at: '2026-07-17T00:30:00Z'
        }
      })
      .mockRejectedValueOnce({ message: 'rate failed' })
      .mockResolvedValueOnce({
        account_id: 7,
        snapshot: {
          status: 'ok',
          last_attempt_at: '2026-07-17T00:00:00Z',
          next_probe_at: '2026-07-17T00:30:00Z'
        }
      })
    queryUpstreamQuota
      .mockResolvedValueOnce(quotaResult)
      .mockRejectedValueOnce({ message: 'refresh failed' })

    const wrapper = mountView()
    await flushPromises()
    vi.spyOn(console, 'error').mockImplementation(() => {})
    const cell = () => wrapper.get('[data-test="quota-cell"][data-account-id="7"]')

    await cell().get('[data-test="probe-rate"]').trigger('click')
    await flushPromises()
    expect(cell().attributes('data-rate-feedback')).toBe('success')
    vi.advanceTimersByTime(999)
    await wrapper.vm.$nextTick()
    expect(cell().attributes('data-rate-feedback')).toBe('success')
    vi.advanceTimersByTime(1)
    await wrapper.vm.$nextTick()
    expect(cell().attributes('data-rate-feedback')).toBe('undefined')

    await cell().get('[data-test="probe-rate"]').trigger('click')
    await flushPromises()
    expect(cell().attributes('data-rate-feedback')).toBe('error')
    expect(cell().attributes('data-rate-error')).toBe('true')
    expect(cell().attributes('data-rate-error-at')).toMatch(/^\d{4}-\d{2}-\d{2}T/)
    vi.advanceTimersByTime(1000)
    await wrapper.vm.$nextTick()
    expect(cell().attributes('data-rate-feedback')).toBe('undefined')
    expect(cell().attributes('data-rate-error')).toBe('true')

    await cell().get('[data-test="probe-rate"]').trigger('click')
    await flushPromises()
    expect(cell().attributes('data-rate-feedback')).toBe('success')
    expect(cell().attributes('data-rate-error')).toBe('false')
    expect(cell().attributes('data-rate-error-at')).toBe('')

    await cell().get('[data-test="query-quota"]').trigger('click')
    await flushPromises()
    expect(cell().attributes('data-quota-feedback')).toBe('success')
    vi.advanceTimersByTime(500)

    await cell().get('[data-test="query-quota"]').trigger('click')
    await flushPromises()
    expect(cell().attributes('data-quota-feedback')).toBe('error')
    vi.advanceTimersByTime(500)
    await wrapper.vm.$nextTick()
    expect(cell().attributes('data-quota-feedback')).toBe('error')
    vi.advanceTimersByTime(500)
    await wrapper.vm.$nextTick()
    expect(cell().attributes('data-quota-feedback')).toBe('undefined')

    wrapper.unmount()
  })

  it('renders only identified upstreams with their official generation logos', async () => {
    const siteLogoKey = '0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef'
    listAccounts.mockResolvedValueOnce({
      items: [
        account(1, { extra: { upstream_identity: {
          detector_version: 2, status: 'identified', provider: 'sub2api', site_logo_key: siteLogoKey, detected_at: '2026-07-20T00:00:00Z'
        } } }),
        account(2, { extra: { upstream_identity: {
          detector_version: 1, status: 'identified', provider: 'new_api', variant: 'legacy', detected_at: '2026-07-20T00:00:00Z'
        } } }),
        account(3, { extra: { upstream_identity: {
          detector_version: 1, status: 'identified', provider: 'new_api', variant: 'modern', detected_at: '2026-07-20T00:00:00Z'
        } } }),
        account(4, { extra: { upstream_identity: {
          detector_version: 1, status: 'failed', detected_at: '2026-07-20T00:00:00Z'
        } } })
      ],
      total: 4,
      page: 1,
      page_size: 20,
      pages: 1
    })

    const wrapper = mountView()
    await flushPromises()
    const badge = (id: number) => wrapper.find(
      `[data-test="virtual-row"][data-account-id="${id}"] [data-testid="upstream-identity-badge"]`
    )

    expect(badge(1).text()).toBe('Sub2API')
    expect(badge(1).get('img').attributes('src')).toBe('/logo.svg')
    expect(badge(1).element.firstElementChild?.tagName).toBe('IMG')
    expect(badge(2).text()).toBe('New API')
    expect(badge(3).text()).toBe('New API')
    expect(badge(2).get('img').attributes('src')).not.toBe(badge(3).get('img').attributes('src'))
    expect(badge(2).element.firstElementChild?.tagName).toBe('IMG')
    expect(badge(3).element.firstElementChild?.tagName).toBe('IMG')
    expect(badge(2).attributes('title')).toBeUndefined()
    expect(badge(3).attributes('title')).toBeUndefined()
    expect(badge(4).exists()).toBe(false)
    await vi.waitFor(() => expect(createObjectURL).toHaveBeenCalledOnce())
    await wrapper.vm.$nextTick()
    const siteBadge = wrapper.get(
      '[data-test="virtual-row"][data-account-id="1"] [data-testid="upstream-site-logo-badge"]'
    )
    expect(getUpstreamSiteLogo).toHaveBeenCalledOnce()
    expect(getUpstreamSiteLogo).toHaveBeenCalledWith(siteLogoKey, expect.any(AbortSignal))
    expect(siteBadge.get('img').attributes('src')).toBe('blob:upstream-site-logo')
    expect(siteBadge.attributes('title')).toBeUndefined()
    expect(siteBadge.element.parentElement).toBe(badge(1).element.parentElement)
    expect(siteBadge.element.previousElementSibling).toBe(badge(1).element)
    expect(siteBadge.element.parentElement?.classList.contains('items-stretch')).toBe(true)
    expect(siteBadge.element.parentElement?.classList.contains('overflow-hidden')).toBe(true)
    expect(siteBadge.get('img').classes()).toContain('h-3.5')
    wrapper.unmount()
    expect(revokeObjectURL).toHaveBeenCalledWith('blob:upstream-site-logo')
  })
})
