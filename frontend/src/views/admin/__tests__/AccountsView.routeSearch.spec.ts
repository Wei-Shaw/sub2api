import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { defineComponent, reactive } from 'vue'
import { routeLocationKey } from 'vue-router'

import AccountsView from '../AccountsView.vue'

const {
  listAccounts,
  listWithEtag,
  getById,
  getBatchTodayStats,
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
  props: { data: { type: Array, default: () => [] } },
  template: `
    <div>
      <div v-for="row in data" :key="row.id" :data-account-name="row.name">
        <slot name="cell-groups" :row="row" />
        <slot name="cell-actions" :row="row" />
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

const route = reactive<{ query: Record<string, unknown> }>({ query: {} })

function mountView(stubActionMenu = true, withRouter = true) {
  return mount(AccountsView, {
    attachTo: document.body,
    global: {
      provide: withRouter ? { [routeLocationKey as symbol]: route } : {},
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

const lastListParams = () => listAccounts.mock.calls[listAccounts.mock.calls.length - 1][2]

describe('admin AccountsView ?search= route param', () => {
  beforeEach(() => {
    localStorage.clear()
    route.query = {}
    listAccounts.mockReset().mockResolvedValue({ items: [], total: 0, page: 1, page_size: 20, pages: 0 })
    listWithEtag.mockReset().mockResolvedValue({ notModified: true, etag: 'e', data: null })
    getById.mockReset()
    getBatchTodayStats.mockReset().mockResolvedValue({ stats: {} })
    getUpstreamBillingProbeSettings.mockReset().mockResolvedValue({ enabled: true })
    getAllProxies.mockReset().mockResolvedValue([])
    getAllGroups.mockReset().mockResolvedValue([])
    showError.mockReset()
    showWarning.mockReset()
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('prefills search from ?search= on the first request and ignores saved platform/status filters', async () => {
    localStorage.setItem('account-filter-values', JSON.stringify({ platform: 'openai', status: 'error' }))
    route.query = { search: '  demo-renew-24h ' }
    const wrapper = mountView()
    await flushPromises()

    expect(listAccounts).toHaveBeenCalledTimes(1)
    expect(lastListParams()).toEqual(expect.objectContaining({ search: 'demo-renew-24h', platform: '', status: '' }))
    // URL 参数只影响本次进入，不覆盖用户保存的筛选
    expect(JSON.parse(localStorage.getItem('account-filter-values')!)).toEqual({ platform: 'openai', status: 'error' })
    wrapper.unmount()
  })

  it('keeps saved filters and empty search without the param', async () => {
    localStorage.setItem('account-filter-values', JSON.stringify({ platform: 'openai', status: 'error' }))
    const wrapper = mountView()
    await flushPromises()

    expect(lastListParams()).toEqual(expect.objectContaining({ search: '', platform: 'openai', status: 'error' }))
    wrapper.unmount()
  })

  it('reloads with the new search when only the query changes on the same page', async () => {
    route.query = { search: 'demo-renew-24h' }
    const wrapper = mountView()
    await flushPromises()
    listAccounts.mockClear()

    route.query = { search: 'demo-renew-3d' }
    await flushPromises()

    expect(listAccounts).toHaveBeenCalledTimes(1)
    expect(lastListParams()).toEqual(expect.objectContaining({ search: 'demo-renew-3d' }))
    wrapper.unmount()
  })

  it('works without a router (no route injection)', async () => {
    const wrapper = mountView(true, false)
    await flushPromises()

    expect(lastListParams()).toEqual(expect.objectContaining({ search: '' }))
    wrapper.unmount()
  })
})
