import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { DOMWrapper, enableAutoUnmount, flushPromises, mount } from '@vue/test-utils'
import { defineComponent } from 'vue'

import AccountsView from '../AccountsView.vue'
import AccountActionMenu from '@/components/admin/account/AccountActionMenu.vue'
import AccountPerformanceCell from '@/components/account/AccountPerformanceCell.vue'
import Pagination from '@/components/common/Pagination.vue'
import type { BatchAccountPerformanceResponse } from '@/api/admin/accounts'

enableAutoUnmount(afterEach)

const {
  listAccounts,
  listWithEtag,
  getById,
  getBatchTodayStats,
  getBatchPerformance,
  getUpstreamBillingProbeSettings,
  getAllProxies,
  getAllGroups,
  refreshCredentials,
  showError,
  showWarning
} = vi.hoisted(() => ({
  listAccounts: vi.fn(),
  listWithEtag: vi.fn(),
  getById: vi.fn(),
  getBatchTodayStats: vi.fn(),
  getBatchPerformance: vi.fn(),
  getUpstreamBillingProbeSettings: vi.fn(),
  getAllProxies: vi.fn(),
  getAllGroups: vi.fn(),
  refreshCredentials: vi.fn(),
  showError: vi.fn(),
  showWarning: vi.fn()
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    accounts: {
      list: listAccounts,
      getById,
      listWithEtag,
      getBatchTodayStats,
      getBatchPerformance,
      getUpstreamBillingProbeSettings,
      delete: vi.fn(),
      batchClearError: vi.fn(),
      batchRefresh: vi.fn(),
      toggleSchedulable: vi.fn(),
      refreshCredentials
    },
    proxies: { getAll: getAllProxies },
    groups: { getAll: getAllGroups }
  }
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({ showError, showWarning, showSuccess: vi.fn(), showInfo: vi.fn() })
}))

vi.mock('@/stores/auth', () => ({
  useAuthStore: () => ({ token: 'test-token', isSimpleMode: false })
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return { ...actual, useI18n: () => ({ t: (key: string) => key }) }
})

const DataTableStub = defineComponent({
  props: {
    data: { type: Array, default: () => [] },
    columns: { type: Array, default: () => [] }
  },
  template: `
    <div>
      <div v-for="row in data" :key="row.id" :data-account-name="row.name">
        <slot name="cell-groups" :row="row" />
        <slot name="cell-actions" :row="row" />
        <slot v-if="columns.some(column => column.key === 'performance')" name="cell-performance" :row="row" />
      </div>
    </div>
  `
})

const AccountGroupsCellStub = defineComponent({
  props: { groups: { type: Array, default: () => [] } },
  template: '<span data-test="account-groups">{{ groups.map(group => group.name).join(",") }}</span>'
})

const EditAccountModalStub = defineComponent({
  props: { show: Boolean, account: { type: Object, default: null } },
  template: '<div data-test="edit-account">{{ show ? account?.name : "" }}</div>'
})

const AccountTestModalStub = defineComponent({
  props: { show: Boolean, account: { type: Object, default: null } },
  template: '<div data-test="test-account">{{ show ? account?.name : "" }}</div>'
})

const AccountStatsModalStub = defineComponent({
  props: { show: Boolean, account: { type: Object, default: null } },
  template: '<div data-test="stats-account">{{ show ? account?.name : "" }}</div>'
})

function mountView(stubActionMenu = true) {
  return mount(AccountsView, {
    attachTo: document.body,
    global: {
      stubs: {
        AppLayout: { template: '<div><slot /></div>' },
        TablePageLayout: { template: '<div><slot name="filters" /><slot name="table" /><slot name="pagination" /></div>' },
        DataTable: DataTableStub,
        AccountTableActions: { template: '<div><slot name="after" /></div>' },
        AccountTableFilters: true,
        AccountBulkActionsBar: true,
        Pagination: true,
        ConfirmDialog: true,
        AccountActionMenu: stubActionMenu,
        ImportDataModal: true,
        ReAuthAccountModal: true,
        AccountTestModal: AccountTestModalStub,
        AccountStatsModal: AccountStatsModalStub,
        ScheduledTestsPanel: true,
        SyncFromCrsModal: true,
        TempUnschedStatusModal: true,
        ErrorPassthroughRulesModal: true,
        TLSFingerprintProfilesModal: true,
        CreateAccountModal: true,
        EditAccountModal: EditAccountModalStub,
        BulkEditAccountModal: true,
        PlatformTypeBadge: true,
        AccountCapacityCell: true,
        AccountStatusIndicator: true,
        AccountTodayStatsCell: true,
        AccountPerformanceCell: true,
        AccountGroupsCell: AccountGroupsCellStub,
        AccountUsageCell: true,
        UpstreamBillingRateCell: true,
        HelpTooltip: true,
        Icon: true,
        Teleport: stubActionMenu
      }
    }
  })
}

const listRow = {
  id: 42,
  name: 'compact row',
  platform: 'openai',
  type: 'oauth',
  status: 'active',
  schedulable: true,
  concurrency: 2,
  priority: 1,
  group_ids: [7],
  extra: {},
  credentials: {}
}

const fullAccount = {
  ...listRow,
  groups: [{ id: 7, name: 'codex', platform: 'openai' }],
  account_groups: [{ account_id: 42, group_id: 7 }],
  credentials: { api_key: 'redacted' },
  extra: { detail_only: true }
}

const performanceResponse: BatchAccountPerformanceResponse = {
  stats: {
    '42': {
      request_count: 8, ttft_ms: 820, tps: 46.3, cache_rate: 0.725,
      ttft_samples: 8, tps_samples: 8, cache_samples: 8,
      last_request_at: '2026-09-16T02:59:00Z'
    }
  },
  window_start: '2026-09-16T02:00:00Z',
  window_end: '2026-09-16T03:00:00Z'
}

describe('admin AccountsView lite account list', () => {
  beforeEach(() => {
    localStorage.clear()
    vi.spyOn(document, 'visibilityState', 'get').mockReturnValue('visible')
    listAccounts.mockReset().mockResolvedValue({ items: [listRow], total: 1, page: 1, page_size: 20, pages: 1 })
    listWithEtag.mockReset().mockResolvedValue({ notModified: true, etag: 'compact-etag', data: null })
    getById.mockReset().mockResolvedValue(fullAccount)
    getBatchTodayStats.mockReset().mockResolvedValue({ stats: {} })
    getBatchPerformance.mockReset().mockResolvedValue(performanceResponse)
    getUpstreamBillingProbeSettings.mockReset().mockResolvedValue({ enabled: true })
    getAllProxies.mockReset().mockResolvedValue([])
    getAllGroups.mockReset().mockResolvedValue([{ id: 7, name: 'codex', platform: 'openai' }])
    refreshCredentials.mockReset()
    showError.mockReset()
    showWarning.mockReset()
  })

  afterEach(() => {
    vi.useRealTimers()
    vi.restoreAllMocks()
  })

  it('keeps lite=1 on the initial list request', async () => {
    const wrapper = mountView()
    await flushPromises()

    expect(listAccounts).toHaveBeenCalledWith(
      1,
      20,
      expect.objectContaining({ lite: '1' }),
      expect.objectContaining({ signal: expect.any(AbortSignal) })
    )
    wrapper.unmount()
  })

  it('maps group_ids through the group catalog for the table cell', async () => {
    const wrapper = mountView()
    await flushPromises()

    expect(wrapper.get('[data-test="account-groups"]').text()).toBe('codex')
    wrapper.unmount()
  })

  it('keeps the action menu open during internal scrolling but closes it on table scrolling', async () => {
    const wrapper = mountView(false)
    await flushPromises()

    const trigger = wrapper.findAll('button').find(button => button.text() === 'common.more')!
    await trigger.trigger('click')
    const menu = new DOMWrapper(document.body.querySelector('.action-menu-content')!)
    menu.element.dispatchEvent(new Event('scroll'))
    await flushPromises()
    expect(wrapper.findComponent(AccountActionMenu).props('show')).toBe(true)

    menu.get('button').element.dispatchEvent(new Event('scroll'))
    await flushPromises()
    expect(wrapper.findComponent(AccountActionMenu).props('show')).toBe(true)

    wrapper.getComponent(DataTableStub).element.dispatchEvent(new Event('scroll'))
    await flushPromises()
    expect(wrapper.findComponent(AccountActionMenu).props('show')).toBe(false)
    wrapper.unmount()
  })

  it('keeps lite=1 on automatic ETag refreshes', async () => {
    vi.useFakeTimers()
    vi.spyOn(document, 'hidden', 'get').mockReturnValue(false)
    localStorage.setItem('account-auto-refresh', JSON.stringify({ enabled: true, interval_seconds: 5 }))
    const wrapper = mountView()
    await flushPromises()

    await vi.advanceTimersByTimeAsync(6000)
    await flushPromises()

    expect(listWithEtag).toHaveBeenCalledWith(
      1,
      20,
      expect.objectContaining({ lite: '1' }),
      expect.objectContaining({ etag: null })
    )
    wrapper.unmount()
  })

  it('shows performance after usage and refreshes passively with account auto-refresh disabled', async () => {
    vi.useFakeTimers()
    const wrapper = mountView()
    await flushPromises()

    const columns = wrapper.getComponent(DataTableStub).props('columns') as { key: string; sortable: boolean }[]
    const usageIndex = columns.findIndex(column => column.key === 'usage')
    expect(columns[usageIndex + 1]).toMatchObject({ key: 'performance', sortable: false })
    expect(getBatchPerformance).toHaveBeenCalledTimes(1)
    expect(getBatchPerformance).toHaveBeenCalledWith([42], { signal: expect.any(AbortSignal) })
    expect(wrapper.getComponent(AccountPerformanceCell).props()).toMatchObject({
      stats: performanceResponse.stats['42'],
      windowStart: performanceResponse.window_start,
      windowEnd: performanceResponse.window_end
    })

    await vi.advanceTimersByTimeAsync(30_000)
    expect(getBatchPerformance).toHaveBeenCalledTimes(2)
    expect(listWithEtag).not.toHaveBeenCalled()
    expect(refreshCredentials).not.toHaveBeenCalled()
    expect(getBatchTodayStats).toHaveBeenCalledTimes(1)
    wrapper.unmount()
    await vi.advanceTimersByTimeAsync(60_000)
    expect(getBatchPerformance).toHaveBeenCalledTimes(2)
  })

  it('refreshes performance even when account list ETags remain unchanged', async () => {
    vi.useFakeTimers()
    localStorage.setItem('account-auto-refresh', JSON.stringify({ enabled: true, interval_seconds: 5 }))
    const wrapper = mountView()
    await flushPromises()
    await vi.advanceTimersByTimeAsync(30_000)
    expect(listWithEtag).toHaveBeenCalled()
    expect(getBatchPerformance).toHaveBeenCalledTimes(2)
    wrapper.unmount()
  })

  it('pauses in background tabs and immediately refreshes on returning', async () => {
    vi.useFakeTimers()
    const visibility = vi.spyOn(document, 'visibilityState', 'get').mockReturnValue('visible')
    const wrapper = mountView()
    await flushPromises()

    visibility.mockReturnValue('hidden')
    document.dispatchEvent(new Event('visibilitychange'))
    await flushPromises()
    await vi.advanceTimersByTimeAsync(60_000)
    expect(getBatchPerformance).toHaveBeenCalledTimes(1)

    visibility.mockReturnValue('visible')
    document.dispatchEvent(new Event('visibilitychange'))
    await flushPromises()
    expect(getBatchPerformance).toHaveBeenCalledTimes(2)
    wrapper.unmount()
  })

  it('does not fetch until an initially hidden page becomes visible', async () => {
    const visibility = vi.spyOn(document, 'visibilityState', 'get').mockReturnValue('hidden')
    const wrapper = mountView()
    await flushPromises()
    expect(getBatchPerformance).not.toHaveBeenCalled()

    visibility.mockReturnValue('visible')
    document.dispatchEvent(new Event('visibilitychange'))
    await flushPromises()
    expect(getBatchPerformance).toHaveBeenCalledTimes(1)
    wrapper.unmount()
  })

  it('does not poll an empty account list', async () => {
    vi.useFakeTimers()
    listAccounts.mockResolvedValueOnce({ items: [], total: 0, page: 1, page_size: 20, pages: 0 })
    const wrapper = mountView()
    await flushPromises()
    await vi.advanceTimersByTimeAsync(90_000)
    expect(getBatchPerformance).not.toHaveBeenCalled()
    wrapper.unmount()
  })

  it('does not fetch a hidden performance column and fetches immediately when shown', async () => {
    vi.useFakeTimers()
    localStorage.setItem('account-hidden-columns', JSON.stringify(['performance']))
    const wrapper = mountView()
    await flushPromises()
    await vi.advanceTimersByTimeAsync(30_000)
    expect(getBatchPerformance).not.toHaveBeenCalled()

    const toolsButton = wrapper.findAll('button').find(button => button.text().includes('admin.accounts.moreActions'))
    expect(toolsButton).toBeTruthy()
    await toolsButton!.trigger('click')
    const columnButton = wrapper.findAll('button').find(button => button.text() === 'admin.accounts.columns.performance')
    expect(columnButton).toBeTruthy()
    await columnButton!.trigger('click')
    await flushPromises()
    expect(getBatchPerformance).toHaveBeenCalledTimes(1)

    await columnButton!.trigger('click')
    await flushPromises()
    await vi.advanceTimersByTimeAsync(60_000)
    expect(getBatchPerformance).toHaveBeenCalledTimes(1)
    wrapper.unmount()
  })

  it('cancels old-page requests and ignores their late responses', async () => {
    let resolveOld: (response: BatchAccountPerformanceResponse) => void = () => {}
    getBatchPerformance.mockReturnValueOnce(new Promise(resolve => { resolveOld = resolve }))
    listAccounts.mockResolvedValueOnce({ items: [listRow], total: 21, page: 1, page_size: 20, pages: 2 })
    const wrapper = mountView()
    await flushPromises()
    const oldSignal = getBatchPerformance.mock.calls[0][1].signal as AbortSignal

    listAccounts.mockResolvedValueOnce({ items: [{ ...listRow, id: 43 }], total: 21, page: 2, page_size: 20, pages: 2 })
    const nextResponse = { ...performanceResponse, stats: { '43': { ...performanceResponse.stats['42'], tps: 91 } } }
    getBatchPerformance.mockResolvedValueOnce(nextResponse)
    wrapper.getComponent(Pagination).vm.$emit('update:page', 2)
    await flushPromises()
    expect(oldSignal.aborted).toBe(true)
    expect(getBatchPerformance).toHaveBeenLastCalledWith([43], { signal: expect.any(AbortSignal) })
    expect(wrapper.getComponent(AccountPerformanceCell).props('stats').tps).toBe(91)

    resolveOld(performanceResponse)
    await flushPromises()
    expect(wrapper.getComponent(AccountPerformanceCell).props('stats').tps).toBe(91)
    wrapper.unmount()
  })

  it('marks refresh errors as stale and replaces expired-window values with empty statistics', async () => {
    vi.useFakeTimers()
    const logError = vi.spyOn(console, 'error').mockImplementation(() => {})
    const wrapper = mountView()
    await flushPromises()
    getBatchPerformance.mockRejectedValueOnce(new Error('query failed'))
    await vi.advanceTimersByTimeAsync(30_000)
    expect(wrapper.getComponent(AccountPerformanceCell).props()).toMatchObject({
      stats: performanceResponse.stats['42'], error: true
    })
    expect(logError).toHaveBeenCalledWith('Failed to load passive account performance:', expect.any(Error))

    const empty = {
      request_count: 0, ttft_ms: null, tps: null, cache_rate: null,
      ttft_samples: 0, tps_samples: 0, cache_samples: 0, last_request_at: null
    }
    getBatchPerformance.mockResolvedValueOnce({ ...performanceResponse, stats: { '42': empty } })
    await vi.advanceTimersByTimeAsync(30_000)
    expect(wrapper.getComponent(AccountPerformanceCell).props()).toMatchObject({ stats: empty, error: false })
    wrapper.unmount()
  })

  it('does not overlap slow passive requests and aborts on unmount', async () => {
    vi.useFakeTimers()
    getBatchPerformance.mockReturnValueOnce(new Promise(() => {}))
    const wrapper = mountView()
    await flushPromises()
    const signal = getBatchPerformance.mock.calls[0][1].signal as AbortSignal
    await vi.advanceTimersByTimeAsync(90_000)
    expect(getBatchPerformance).toHaveBeenCalledTimes(1)
    wrapper.unmount()
    expect(signal.aborted).toBe(true)
  })

  it('loads the full account by id before opening edit, test, and stats actions', async () => {
    const wrapper = mountView()
    await flushPromises()

    const editButton = wrapper.findAll('button').find(button => button.text().includes('common.edit'))
    expect(editButton).toBeTruthy()
    await editButton!.trigger('click')
    await flushPromises()
    expect(getById).toHaveBeenCalledWith(42)
    expect(wrapper.get('[data-test="edit-account"]').text()).toBe('compact row')

    const menu = wrapper.findComponent(AccountActionMenu)
    menu.vm.$emit('test', listRow)
    await flushPromises()
    expect(getById).toHaveBeenCalledTimes(2)
    expect(wrapper.get('[data-test="test-account"]').text()).toBe('compact row')

    menu.vm.$emit('stats', listRow)
    await flushPromises()
    expect(getById).toHaveBeenCalledTimes(3)
    expect(wrapper.get('[data-test="stats-account"]').text()).toBe('compact row')
    wrapper.unmount()
  })

  it('shows the warning and patches the account after a partial Antigravity refresh', async () => {
    refreshCredentials.mockResolvedValue({
      account: { ...fullAccount, name: 'refreshed account' },
      message: 'Token refreshed, but project_id is temporarily unavailable',
      warning: 'missing_project_id_temporary'
    })
    const wrapper = mountView(false)
    await flushPromises()

    wrapper.findComponent(AccountActionMenu).vm.$emit('refresh-token', listRow)
    await flushPromises()

    expect(refreshCredentials).toHaveBeenCalledWith(42)
    expect(wrapper.get('[data-account-name]').attributes('data-account-name')).toBe('refreshed account')
    expect(showWarning).toHaveBeenCalledWith('Token refreshed, but project_id is temporarily unavailable')
    wrapper.unmount()
  })

  it('shows an error and keeps the modal closed when detail loading fails', async () => {
    const consoleError = vi.spyOn(console, 'error').mockImplementation(() => {})
    getById.mockRejectedValueOnce(new Error('detail failed'))
    const wrapper = mountView()
    await flushPromises()

    const editButton = wrapper.findAll('button').find(button => button.text().includes('common.edit'))
    await editButton!.trigger('click')
    await flushPromises()

    expect(showError).toHaveBeenCalledWith('detail failed')
    expect(wrapper.get('[data-test="edit-account"]').text()).toBe('')
    consoleError.mockRestore()
    wrapper.unmount()
  })
})
