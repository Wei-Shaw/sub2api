import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { defineComponent } from 'vue'
import { flushPromises, mount } from '@vue/test-utils'

import AccountsView from '../AccountsView.vue'

const {
  listAccounts,
  listWithEtag,
  getBatchTodayStats,
  getUpstreamBillingProbeSettings,
  getAllProxies,
  getAllGroups,
  deleteAccount,
  batchRefresh,
  refreshCredentials,
  resetAccountQuota,
  showError,
  showSuccess,
  showInfo,
  routeState
} = vi.hoisted(() => ({
  listAccounts: vi.fn(),
  listWithEtag: vi.fn(),
  getBatchTodayStats: vi.fn(),
  getUpstreamBillingProbeSettings: vi.fn(),
  getAllProxies: vi.fn(),
  getAllGroups: vi.fn(),
  deleteAccount: vi.fn(),
  batchRefresh: vi.fn(),
  refreshCredentials: vi.fn(),
  resetAccountQuota: vi.fn(),
  showError: vi.fn(),
  showSuccess: vi.fn(),
  showInfo: vi.fn(),
  routeState: { query: {} as Record<string, unknown> }
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    accounts: {
      list: listAccounts,
      listWithEtag,
      getBatchTodayStats,
      getUpstreamBillingProbeSettings,
      delete: deleteAccount,
      batchRefresh,
      refreshCredentials,
      resetAccountQuota
    },
    proxies: { getAll: getAllProxies },
    groups: { getAll: getAllGroups }
  }
}))

vi.mock('vue-router', () => ({
  useRoute: () => routeState
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({ showError, showSuccess, showInfo, showWarning: vi.fn() })
}))

vi.mock('@/stores/auth', () => ({
  useAuthStore: () => ({ token: 'test-token' })
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string, params?: Record<string, unknown>) => (params ? `${key}:${JSON.stringify(params)}` : key)
    })
  }
})

const account = (id: number, type = 'oauth') => ({
  id,
  name: `account-${id}`,
  platform: 'anthropic',
  type,
  status: 'active',
  schedulable: true,
  created_at: '2026-09-01T00:00:00Z',
  updated_at: '2026-09-01T00:00:00Z'
})

const page = (items: unknown[]) => ({ items, total: items.length, page: 1, page_size: 20, pages: 1 })

// Renders the rows plus the empty slot, mirroring DataTable's contract.
const DataTableStub = {
  props: ['data'],
  template: `
    <div>
      <div v-if="!data.length" data-test="empty"><slot name="empty" /></div>
      <div v-for="row in data" :key="row.id">
        <slot name="cell-select" :row="row" />
        <slot name="cell-actions" :row="row" />
      </div>
    </div>
  `
}

const ConfirmDialogStub = defineComponent({
  name: 'ConfirmDialog',
  props: ['show', 'title', 'message'],
  emits: ['confirm', 'cancel'],
  template: '<div v-if="show" data-test="confirm-dialog" :data-title="title">{{ message }}<button data-test="confirm-ok" @click="$emit(\'confirm\')">ok</button></div>'
})

const AccountActionMenuStub = defineComponent({
  name: 'AccountActionMenu',
  props: ['account'],
  emits: ['reset-quota', 'refresh-token'],
  template: '<div />'
})

const AccountBulkActionsBarStub = defineComponent({
  name: 'AccountBulkActionsBar',
  props: ['selectedIds', 'busy'],
  emits: ['refresh-token', 'select-page'],
  template: `
    <div>
      <span data-test="busy">{{ String(busy) }}</span>
      <button data-test="select-page" @click="$emit('select-page')">page</button>
      <button data-test="refresh-token" @click="$emit('refresh-token')">refresh</button>
    </div>
  `
})

const CreateAccountModalStub = defineComponent({
  name: 'CreateAccountModal',
  emits: ['created'],
  template: '<div />'
})

const AccountTestModalStub = defineComponent({
  name: 'AccountTestModal',
  props: ['show', 'account'],
  template: '<div data-test="test-modal" :data-show="String(show)" :data-account="account?.id ?? \'\'" />'
})

const mountView = () => mount(AccountsView, {
  global: {
    stubs: {
      AppLayout: { template: '<div><slot /></div>' },
      TablePageLayout: { template: '<div><slot name="filters" /><slot name="table" /></div>' },
      DataTable: DataTableStub,
      Pagination: true,
      ConfirmDialog: ConfirmDialogStub,
      AccountTableActions: true,
      AccountTableFilters: true,
      AccountBulkActionsBar: AccountBulkActionsBarStub,
      AccountActionMenu: AccountActionMenuStub,
      ImportDataModal: true,
      ReAuthAccountModal: true,
      AccountTestModal: AccountTestModalStub,
      AccountStatsModal: true,
      ScheduledTestsPanel: true,
      SyncFromCrsModal: true,
      TempUnschedStatusModal: true,
      ErrorPassthroughRulesModal: true,
      TLSFingerprintProfilesModal: true,
      CreateAccountModal: CreateAccountModalStub,
      EditAccountModal: true,
      BulkEditAccountModal: true,
      PlatformTypeBadge: true,
      AccountCapacityCell: true,
      AccountStatusIndicator: true,
      AccountTodayStatsCell: true,
      AccountGroupsCell: true,
      AccountUsageCell: true,
      Icon: true
    }
  }
})

describe('admin AccountsView error feedback', () => {
  beforeEach(() => {
    localStorage.clear()
    routeState.query = {}
    for (const fn of [listAccounts, listWithEtag, getBatchTodayStats, getUpstreamBillingProbeSettings, getAllProxies,
      getAllGroups, deleteAccount, batchRefresh, refreshCredentials, resetAccountQuota, showError, showSuccess, showInfo]) {
      fn.mockReset()
    }
    listAccounts.mockResolvedValue(page([]))
    listWithEtag.mockResolvedValue({ notModified: true, etag: null, data: null })
    getBatchTodayStats.mockResolvedValue({ stats: {} })
    getUpstreamBillingProbeSettings.mockResolvedValue({ enabled: true, interval_minutes: 30 })
    getAllProxies.mockResolvedValue([])
    getAllGroups.mockResolvedValue([])
    vi.spyOn(console, 'error').mockImplementation(() => {})
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('shows an error state instead of "no data" when the list request fails, and retries', async () => {
    listAccounts.mockRejectedValueOnce({ status: 500, message: 'database unavailable' })
    const wrapper = mountView()
    await flushPromises()

    const errorState = wrapper.get('[data-testid="accounts-load-error"]')
    expect(errorState.text()).toContain('admin.accounts.failedToLoad')
    expect(errorState.text()).toContain('database unavailable')
    expect(wrapper.text()).not.toContain('empty.noData')
    expect(showError).toHaveBeenCalledWith('database unavailable')
    expect(getBatchTodayStats).not.toHaveBeenCalled()

    await errorState.get('button').trigger('click')
    await flushPromises()

    expect(listAccounts).toHaveBeenCalledTimes(2)
    expect(wrapper.find('[data-testid="accounts-load-error"]').exists()).toBe(false)
    expect(wrapper.text()).toContain('empty.noData')
  })

  it('asks for confirmation before resetting quota and reports failures', async () => {
    listAccounts.mockResolvedValue(page([account(3)]))
    resetAccountQuota.mockRejectedValueOnce({ message: 'quota reset denied' })
    const wrapper = mountView()
    await flushPromises()

    wrapper.getComponent(AccountActionMenuStub).vm.$emit('reset-quota', account(3))
    await flushPromises()

    expect(resetAccountQuota).not.toHaveBeenCalled()
    const dialog = wrapper.findAll('[data-test="confirm-dialog"]').find(node => node.attributes('data-title') === 'admin.accounts.resetQuota')
    expect(dialog?.text()).toContain('admin.accounts.resetQuotaConfirm:{"name":"account-3"}')

    await dialog!.get('[data-test="confirm-ok"]').trigger('click')
    await flushPromises()

    expect(resetAccountQuota).toHaveBeenCalledWith(3)
    expect(showError).toHaveBeenCalledWith('quota reset denied')
  })

  it('reports single-account refresh and delete failures instead of failing silently', async () => {
    listAccounts.mockResolvedValue(page([account(5)]))
    refreshCredentials.mockRejectedValueOnce({ message: 'refresh token revoked' })
    deleteAccount.mockRejectedValueOnce({})
    const wrapper = mountView()
    await flushPromises()

    wrapper.getComponent(AccountActionMenuStub).vm.$emit('refresh-token', account(5))
    await flushPromises()
    expect(showError).toHaveBeenCalledWith('refresh token revoked')

    await wrapper.findAll('button').find(node => node.text() === 'common.delete')!.trigger('click')
    const dialog = wrapper.findAll('[data-test="confirm-dialog"]').find(node => node.attributes('data-title') === 'admin.accounts.deleteAccount')
    await dialog!.get('[data-test="confirm-ok"]').trigger('click')
    await flushPromises()
    expect(showError).toHaveBeenCalledWith('admin.accounts.failedToDelete')
  })

  it('shows a success toast after a single-account token refresh', async () => {
    listAccounts.mockResolvedValue(page([account(5)]))
    refreshCredentials.mockResolvedValueOnce({ account: account(5), warning: false, message: '' })
    const wrapper = mountView()
    await flushPromises()

    wrapper.getComponent(AccountActionMenuStub).vm.$emit('refresh-token', account(5))
    await flushPromises()

    expect(showSuccess).toHaveBeenCalledWith('admin.accounts.tokenRefreshed')
  })

  it('marks bulk actions busy while running, confirms with the count and extracts the error message', async () => {
    listAccounts.mockResolvedValue(page([account(1), account(2)]))
    let rejectRefresh: (reason: unknown) => void = () => {}
    batchRefresh.mockImplementationOnce(() => new Promise((_, reject) => { rejectRefresh = reject }))
    const confirmSpy = vi.spyOn(window, 'confirm').mockReturnValue(true)
    const wrapper = mountView()
    await flushPromises()

    await wrapper.get('[data-test="select-page"]').trigger('click')
    await wrapper.get('[data-test="refresh-token"]').trigger('click')
    await flushPromises()

    expect(confirmSpy).toHaveBeenCalledWith('admin.accounts.bulkActions.confirmRefreshToken:{"count":2}')
    expect(wrapper.get('[data-test="busy"]').text()).toBe('true')

    // A second click while the first batch is still running must not start another batch.
    await wrapper.get('[data-test="refresh-token"]').trigger('click')
    expect(batchRefresh).toHaveBeenCalledTimes(1)

    rejectRefresh({ status: 502, message: 'upstream timeout' })
    await flushPromises()

    expect(wrapper.get('[data-test="busy"]').text()).toBe('false')
    expect(showError).toHaveBeenCalledWith('upstream timeout')
    expect(showError).not.toHaveBeenCalledWith('[object Object]')
  })
})
