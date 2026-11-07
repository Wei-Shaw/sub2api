import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { DOMWrapper, flushPromises, mount } from '@vue/test-utils'
import { defineComponent, onMounted, inject } from 'vue'

import AccountsView from '../AccountsView.vue'
import AccountActionMenu from '@/components/admin/account/AccountActionMenu.vue'
import AccountTableActions from '@/components/admin/account/AccountTableActions.vue'
import AccountTableFilters from '@/components/admin/account/AccountTableFilters.vue'
import Pagination from '@/components/common/Pagination.vue'
import { accountQuotaRefreshContext } from '@/utils/accountQuotaRefresh'
import type { Account } from '@/types'

const {
  getBatchUsage,
  getUsage,
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
  getBatchUsage: vi.fn(),
  getUsage: vi.fn(),
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
      getBatchUsage,
      getUsage,
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

function mountView(stubActionMenu = true, groupsCell = AccountGroupsCellStub) {
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
        AccountGroupsCell: groupsCell,
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

describe('admin AccountsView lite account list', () => {
  beforeEach(() => {
    localStorage.clear()
    getBatchUsage.mockReset().mockResolvedValue({ usage: {}, errors: {} })
    getUsage.mockReset().mockResolvedValue({ source: 'active', updated_at: 'fresh' })
    listAccounts.mockReset().mockResolvedValue({ items: [listRow], total: 1, page: 1, page_size: 20, pages: 1 })
    listWithEtag.mockReset().mockResolvedValue({ notModified: true, etag: 'compact-etag', data: null })
    getById.mockReset().mockResolvedValue(fullAccount)
    getBatchTodayStats.mockReset().mockResolvedValue({ stats: {} })
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
  it('does not actively scan quotas on the initial low-cost load', async () => {
    const wrapper = mountView()
    await flushPromises()
    expect(getUsage).not.toHaveBeenCalled()
    wrapper.unmount()
  })

  it('refreshes all 60 current-page rows without mounted usage cells, even with hidden columns', async () => {
    localStorage.setItem('account-hidden-columns', JSON.stringify(['usage', 'today_stats']))
    const rows = Array.from({ length: 60 }, (_, index) => ({ ...listRow, id: index + 1 }))
    listAccounts.mockResolvedValue({ items: rows, total: 180, pages: 3 })
    getById.mockImplementation(async (id: number) => ({ ...listRow, id, name: `fresh-${id}` }))
    const wrapper = mountView()
    await flushPromises()
    wrapper.findComponent(AccountTableActions).vm.$emit('refresh')
    await flushPromises()
    expect(getUsage).toHaveBeenCalledTimes(60)
    expect(getUsage.mock.calls.map(call => call[0]).sort((a, b) => a - b)).toEqual(rows.map(row => row.id))
    expect(getUsage.mock.calls.every(call => call[1] === 'active' && call[2] === true)).toBe(true)
    expect(getBatchTodayStats).not.toHaveBeenCalled()
    expect(wrapper.findAll('[data-account-name]')[59].attributes('data-account-name')).toBe('fresh-60')
    wrapper.unmount()
  })

  it('queries quotas after a 304 and resets the timer only after the whole round finishes', async () => {
    vi.useFakeTimers()
    vi.spyOn(document, 'hidden', 'get').mockReturnValue(false)
    localStorage.setItem('account-auto-refresh', JSON.stringify({ enabled: true, interval_seconds: 5 }))
    let finish!: (value: object) => void
    getUsage.mockReturnValueOnce(new Promise(resolve => { finish = resolve }))
    const wrapper = mountView()
    await flushPromises()
    await vi.advanceTimersByTimeAsync(6000)
    expect(listWithEtag).toHaveBeenCalledTimes(1)
    expect(getUsage).toHaveBeenCalledWith(42, 'active', true)
    await vi.advanceTimersByTimeAsync(20000)
    expect(listWithEtag).toHaveBeenCalledTimes(1)
    finish({ updated_at: 'fresh' })
    await flushPromises()
    await vi.advanceTimersByTimeAsync(5000)
    expect(listWithEtag).toHaveBeenCalledTimes(1)
    await vi.advanceTimersByTimeAsync(1000)
    expect(listWithEtag).toHaveBeenCalledTimes(2)
    wrapper.unmount()
  })

  it('coalesces repeated manual refreshes and never queries residual rows after list failure', async () => {
    const wrapper = mountView()
    await flushPromises()
    let finish!: (value: object) => void
    listAccounts.mockReturnValueOnce(new Promise(resolve => { finish = resolve }))
    const actions = wrapper.findComponent(AccountTableActions)
    actions.vm.$emit('refresh')
    actions.vm.$emit('refresh')
    await flushPromises()
    expect(listAccounts).toHaveBeenCalledTimes(2)
    expect(getUsage).not.toHaveBeenCalled()
    finish({ items: [listRow], total: 1, pages: 1 })
    await flushPromises()
    expect(getUsage).toHaveBeenCalledTimes(1)
    vi.spyOn(console, 'error').mockImplementation(() => {})
    listAccounts.mockRejectedValueOnce(new Error('list unavailable'))
    actions.vm.$emit('refresh')
    await flushPromises()
    expect(getUsage).toHaveBeenCalledTimes(1)
    expect(showError).toHaveBeenCalledTimes(1)
    expect(showError).toHaveBeenCalledWith('admin.accounts.failedToLoad')
    wrapper.unmount()
  })

  it('does not query residual rows after an automatic list failure', async () => {
    vi.useFakeTimers()
    vi.spyOn(document, 'hidden', 'get').mockReturnValue(false)
    localStorage.setItem('account-auto-refresh', JSON.stringify({ enabled: true, interval_seconds: 5 }))
    vi.spyOn(console, 'error').mockImplementation(() => {})
    listWithEtag.mockRejectedValueOnce(new Error('ETag list unavailable'))
    const wrapper = mountView()
    await flushPromises()
    await vi.advanceTimersByTimeAsync(6000)
    expect(listWithEtag).toHaveBeenCalledOnce()
    expect(getUsage).not.toHaveBeenCalled()
    wrapper.unmount()
  })

  it('discards late ETag data and errors after filtering, without restoring its ETag', async () => {
    vi.useFakeTimers()
    vi.spyOn(document, 'hidden', 'get').mockReturnValue(false)
    localStorage.setItem('account-auto-refresh', JSON.stringify({ enabled: true, interval_seconds: 5 }))
    let finish!: (value: object) => void
    listWithEtag.mockReturnValueOnce(new Promise(resolve => { finish = resolve }))
    const wrapper = mountView()
    await flushPromises()
    await vi.advanceTimersByTimeAsync(6000)
    wrapper.findComponent(AccountTableFilters).vm.$emit('update:filters', { platform: 'anthropic' })
    await flushPromises()
    finish({ notModified: false, etag: 'stale-etag', data: { items: [{ ...listRow, name: 'stale' }], total: 1, pages: 1 } })
    await flushPromises()
    expect(wrapper.text()).not.toContain('stale')
    expect(getUsage).not.toHaveBeenCalled()
    await vi.advanceTimersByTimeAsync(6000)
    expect(listWithEtag.mock.calls[1][3]).toMatchObject({ etag: null })
    wrapper.unmount()
  })

  it('discards late full list data when a page changes even if the API ignores abort', async () => {
    listAccounts.mockResolvedValue({ items: [listRow], total: 2, pages: 2 })
    const wrapper = mountView()
    await flushPromises()
    let finish!: (value: object) => void
    listAccounts.mockReturnValueOnce(new Promise(resolve => { finish = resolve }))
    wrapper.findComponent(AccountTableActions).vm.$emit('refresh')
    await flushPromises()
    listAccounts.mockResolvedValueOnce({ items: [{ ...listRow, id: 43, name: 'page two' }], total: 2, pages: 2 })
    wrapper.findComponent(Pagination).vm.$emit('update:page', 2)
    await flushPromises()
    finish({ items: [{ ...listRow, name: 'old page one' }], total: 2, pages: 2 })
    await flushPromises()
    expect(wrapper.get('[data-account-name]').attributes('data-account-name')).toBe('page two')
    expect(getUsage).not.toHaveBeenCalled()
    wrapper.unmount()
  })

  it('stops queued quota tasks and ignores late usage readbacks after a page-size change', async () => {
    const rows = Array.from({ length: 8 }, (_, index) => ({ ...listRow, id: index + 1 }))
    listAccounts.mockResolvedValue({ items: rows, total: 8, pages: 1 })
    let finish!: (value: object) => void
    getUsage.mockReturnValue(new Promise(resolve => { finish = resolve }))
    const wrapper = mountView()
    await flushPromises()
    wrapper.findComponent(AccountTableActions).vm.$emit('refresh')
    await flushPromises()
    expect(getUsage).toHaveBeenCalledTimes(4)
    listAccounts.mockResolvedValueOnce({ items: [{ ...listRow, name: 'new scope' }], total: 1, pages: 1 })
    wrapper.findComponent(Pagination).vm.$emit('update:pageSize', 10)
    await flushPromises()
    finish({ updated_at: 'old quota' })
    await flushPromises()
    expect(getUsage).toHaveBeenCalledTimes(4)
    expect(getById).not.toHaveBeenCalled()
    expect(wrapper.get('[data-account-name]').attributes('data-account-name')).toBe('new scope')
    wrapper.unmount()
  })

  it('keeps automatic refresh paused while hidden or a menu is open', async () => {
    vi.useFakeTimers()
    const hidden = vi.spyOn(document, 'hidden', 'get').mockReturnValue(true)
    localStorage.setItem('account-auto-refresh', JSON.stringify({ enabled: true, interval_seconds: 5 }))
    const wrapper = mountView()
    await flushPromises()
    await vi.advanceTimersByTimeAsync(10000)
    expect(listWithEtag).not.toHaveBeenCalled()
    hidden.mockReturnValue(false)
    await wrapper.findAll('button').find(button => button.text() === 'common.more')!.trigger('click')
    await vi.advanceTimersByTimeAsync(10000)
    expect(listWithEtag).not.toHaveBeenCalled()
    wrapper.unmount()
  })

  it('does not commit quota results after unmount', async () => {
    let finish!: (value: object) => void
    getUsage.mockReturnValueOnce(new Promise(resolve => { finish = resolve }))
    const wrapper = mountView()
    await flushPromises()
    wrapper.findComponent(AccountTableActions).vm.$emit('refresh')
    await flushPromises()
    wrapper.unmount()
    finish({ updated_at: 'late' })
    await flushPromises()
    expect(getById).not.toHaveBeenCalled()
    expect(showWarning).not.toHaveBeenCalled()
  })

  it('keeps a late passive batch from replacing the active usage/error/loading state', async () => {
    vi.useFakeTimers()
    let finish!: (value: object) => void
    getBatchUsage.mockReturnValueOnce(new Promise(resolve => { finish = resolve }))
    const UsageProbe = defineComponent({
      props: ['account', 'requestBatchedUsage', 'batchedUsage', 'batchedUsageError', 'batchedUsageLoading'],
      setup(props) { onMounted(() => props.requestBatchedUsage(props.account)) },
      template: '<div data-test="usage-probe">{{ batchedUsage?.updated_at }}|{{ batchedUsageError }}|{{ batchedUsageLoading }}</div>'
    })
    // Include a real usage slot without rendering the expensive table.
    const UsageTable = defineComponent({
      props: ['data'],
      template: '<div><div v-for="row in data" :key="row.id"><slot name="cell-usage" :row="row" /></div></div>'
    })
    const wrapper = mount(AccountsView, {
      global: { stubs: {
        AppLayout: { template: '<div><slot /></div>' },
        TablePageLayout: { template: '<div><slot name="filters" /><slot name="table" /></div>' },
        DataTable: UsageTable, AccountUsageCell: UsageProbe,
        AccountTableActions: true, AccountTableFilters: true, Icon: true,
        ImportDataModal: true, CreateAccountModal: true, EditAccountModal: true,
        AccountActionMenu: true, HelpTooltip: true,
        ReAuthAccountModal: true, AccountTestModal: true, AccountStatsModal: true,
        ScheduledTestsPanel: true, SyncFromCrsModal: true, TempUnschedStatusModal: true,
        ErrorPassthroughRulesModal: true, TLSFingerprintProfilesModal: true,
        BulkEditAccountModal: true, ConfirmDialog: true
      } }
    })
    await flushPromises()
    await vi.advanceTimersByTimeAsync(0)
    expect(getBatchUsage).toHaveBeenCalledOnce()
    wrapper.findComponent(AccountTableActions).vm.$emit('refresh')
    await flushPromises()
    expect(wrapper.get('[data-test="usage-probe"]').text()).toBe('fresh||false')
    finish({ usage: { 42: { updated_at: 'stale passive' } }, errors: { 42: 'old failure' } })
    await flushPromises()
    expect(wrapper.get('[data-test="usage-probe"]').text()).toBe('fresh||false')
    wrapper.unmount()
  })

  it('refreshes the data page in a mobile viewport without mounted usage rows', async () => {
    vi.spyOn(window, 'matchMedia').mockReturnValue({ matches: false, addEventListener: vi.fn(), removeEventListener: vi.fn() } as unknown as MediaQueryList)
    const wrapper = mountView()
    await flushPromises()
    wrapper.findComponent(AccountTableActions).vm.$emit('refresh')
    await flushPromises()
    expect(getUsage).toHaveBeenCalledWith(42, 'active', true)
    wrapper.unmount()
  })

  it.each(['hidden', 'modal'])('stops starting remaining automatic quota tasks when the page becomes %s', async pause => {
    vi.useFakeTimers()
    const hidden = vi.spyOn(document, 'hidden', 'get').mockReturnValue(false)
    localStorage.setItem('account-auto-refresh', JSON.stringify({ enabled: true, interval_seconds: 5 }))
    const rows = Array.from({ length: 12 }, (_, index) => ({ ...listRow, id: index + 1 }))
    listAccounts.mockResolvedValue({ items: rows, total: rows.length, pages: 1 })
    getById.mockImplementation(async (id: number) => ({ ...listRow, id, name: `committed-${id}` }))
    let finish!: (value: object) => void
    getUsage.mockReturnValue(new Promise(resolve => { finish = resolve }))
    const wrapper = mountView()
    await flushPromises()
    await vi.advanceTimersByTimeAsync(6000)
    expect(getUsage).toHaveBeenCalledTimes(4)
    if (pause === 'hidden') hidden.mockReturnValue(true)
    else wrapper.findComponent(AccountTableActions).vm.$emit('create')
    await flushPromises()
    finish({ updated_at: 'fresh' })
    await flushPromises()
    // Existing requests may commit, but the next eight rows must not start.
    expect(getUsage).toHaveBeenCalledTimes(4)
    expect(getById).toHaveBeenCalledTimes(4)
    await vi.advanceTimersByTimeAsync(20000)
    expect(listWithEtag).toHaveBeenCalledTimes(1)
    wrapper.unmount()
  })

  it('does not overwrite a fresh inline account commit with an overlapping old full list response', async () => {
    const QueryBridge = defineComponent({
      setup() {
        const controller = inject(accountQuotaRefreshContext)!
        return { query: () => controller.refresh(listRow as Account) }
      },
      template: '<button data-test="inline-query" @click="query">Query</button>'
    })
    const wrapper = mountView(true, QueryBridge)
    await flushPromises()
    let finishList!: (value: object) => void
    listAccounts.mockReturnValueOnce(new Promise(resolve => { finishList = resolve }))
    wrapper.findComponent(AccountTableActions).vm.$emit('refresh')
    await flushPromises()
    getById.mockResolvedValueOnce({ ...listRow, name: 'new quota account', extra: { codex_5h_used_percent: 73 } })
    await wrapper.get('[data-test="inline-query"]').trigger('click')
    await flushPromises()
    expect(wrapper.get('[data-account-name]').attributes('data-account-name')).toBe('new quota account')
    let finishQuota!: (value: object) => void
    getUsage.mockReturnValueOnce(new Promise(resolve => { finishQuota = resolve }))
    finishList({ items: [{ ...listRow, name: 'old list account' }], total: 1, pages: 1 })
    await flushPromises()
    expect(wrapper.get('[data-account-name]').attributes('data-account-name')).toBe('new quota account')
    wrapper.unmount()
    finishQuota({ updated_at: 'late' })
    await flushPromises()
  })

  it('forces passive/local usage only for unsupported-active current-page rows on manual refresh', async () => {
    const rows = [
      { ...listRow, id: 1, platform: 'anthropic', type: 'setup-token' },
      { ...listRow, id: 2, platform: 'gemini', type: 'oauth' },
      { ...listRow, id: 3, platform: 'openai', type: 'oauth' },
      { ...listRow, id: 4, platform: 'anthropic', type: 'apikey' }
    ]
    listAccounts.mockResolvedValue({ items: rows, total: rows.length, pages: 1 })
    getById.mockImplementation(async (id: number) => rows.find(row => row.id === id))
    getBatchUsage.mockResolvedValue({ usage: { 1: { updated_at: 'new passive' }, 2: { updated_at: 'new local' } }, errors: {} })
    const wrapper = mountView()
    await flushPromises()
    expect(getBatchUsage).not.toHaveBeenCalled()
    wrapper.findComponent(AccountTableActions).vm.$emit('refresh')
    await flushPromises()
    expect(getBatchUsage).toHaveBeenCalledWith([1, 2], true)
    expect(getUsage).toHaveBeenCalledTimes(1)
    expect(getUsage).toHaveBeenCalledWith(3, 'active', true)
    wrapper.findComponent(AccountTableActions).vm.$emit('refresh')
    await flushPromises()
    expect(getBatchUsage).toHaveBeenCalledTimes(2)
    expect(getBatchUsage).toHaveBeenLastCalledWith([1, 2], true)
    wrapper.unmount()
  })

})
