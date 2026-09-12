import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'

import AccountsView from '../AccountsView.vue'

const {
  listAccounts,
  listWithEtag,
  updateAccount,
  getBatchTodayStats,
  getAllProxies,
  getAllGroups,
  showError
} = vi.hoisted(() => ({
  listAccounts: vi.fn(),
  listWithEtag: vi.fn(),
  updateAccount: vi.fn(),
  getBatchTodayStats: vi.fn(),
  getAllProxies: vi.fn(),
  getAllGroups: vi.fn(),
  showError: vi.fn()
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    accounts: {
      list: listAccounts,
      listWithEtag,
      update: updateAccount,
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
    showSuccess: vi.fn(),
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
      <div v-for="row in data" :key="row.id" data-test="priority-cell">
        <slot name="cell-priority" :value="row.priority" :row="row" />
      </div>
    </div>
  `
}

const baseAccount = {
  id: 1,
  name: 'primary-openai',
  platform: 'openai',
  type: 'oauth',
  status: 'active',
  schedulable: true,
  concurrency: 1,
  priority: 10,
  error_message: null,
  last_used_at: null,
  expires_at: null,
  auto_pause_on_expired: false,
  created_at: '2026-01-01T00:00:00Z',
  updated_at: '2026-01-01T00:00:00Z'
}

function mountView() {
  return mount(AccountsView, {
    global: {
      stubs: {
        AppLayout: { template: '<div><slot /></div>' },
        TablePageLayout: { template: '<div><slot name="table" /></div>' },
        DataTable: DataTableStub,
        Pagination: true,
        ConfirmDialog: true,
        AccountTableActions: true,
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
        AccountUsageCell: true,
        UpstreamBillingRateCell: true,
        HelpTooltip: true,
        Icon: true
      }
    }
  })
}

describe('admin AccountsView inline priority editing', () => {
  beforeEach(() => {
    localStorage.clear()
    listAccounts.mockReset()
    listWithEtag.mockReset()
    updateAccount.mockReset()
    getBatchTodayStats.mockReset()
    getAllProxies.mockReset()
    getAllGroups.mockReset()
    showError.mockReset()

    listAccounts.mockResolvedValue({
      items: [{ ...baseAccount }],
      total: 1,
      page: 1,
      page_size: 20,
      pages: 1
    })
    listWithEtag.mockResolvedValue({ notModified: true, etag: null, data: null })
    getBatchTodayStats.mockResolvedValue({ stats: {} })
    getAllProxies.mockResolvedValue([])
    getAllGroups.mockResolvedValue([])
  })

  it('updates only priority and patches the returned account into the row', async () => {
    updateAccount.mockResolvedValue({ ...baseAccount, priority: 3, updated_at: '2026-08-03T00:00:00Z' })
    const wrapper = mountView()
    await flushPromises()

    await wrapper.get('[data-testid="account-priority-edit"]').trigger('click')
    const input = wrapper.get('[data-testid="account-priority-input"]')
    expect((input.element as HTMLInputElement).value).toBe('10')

    await input.setValue('3')
    await input.trigger('keydown', { key: 'Enter' })
    await flushPromises()

    expect(updateAccount).toHaveBeenCalledWith(1, { priority: 3 })
    expect(wrapper.find('[data-testid="account-priority-input"]').exists()).toBe(false)
    expect(wrapper.get('[data-test="priority-cell"]').text()).toContain('3')
  })

  it('rejects non-positive priority without sending a request', async () => {
    const wrapper = mountView()
    await flushPromises()

    await wrapper.get('[data-testid="account-priority-edit"]').trigger('click')
    await wrapper.get('[data-testid="account-priority-input"]').setValue('0')
    await wrapper.get('[data-testid="account-priority-save"]').trigger('click')

    expect(updateAccount).not.toHaveBeenCalled()
    expect(showError).toHaveBeenCalledWith('admin.accounts.priorityInvalid')
    expect(wrapper.find('[data-testid="account-priority-input"]').exists()).toBe(true)
  })

  it('keeps the editor open when the update fails', async () => {
    updateAccount.mockRejectedValue(new Error('update failed'))
    const wrapper = mountView()
    await flushPromises()

    await wrapper.get('[data-testid="account-priority-edit"]').trigger('click')
    await wrapper.get('[data-testid="account-priority-input"]').setValue('4')
    await wrapper.get('[data-testid="account-priority-save"]').trigger('click')
    await flushPromises()

    expect(showError).toHaveBeenCalled()
    expect(wrapper.find('[data-testid="account-priority-input"]').exists()).toBe(true)
    expect((wrapper.get('[data-testid="account-priority-input"]').element as HTMLInputElement).value).toBe('4')
  })

  it('cancels editing without sending a request', async () => {
    const wrapper = mountView()
    await flushPromises()

    await wrapper.get('[data-testid="account-priority-edit"]').trigger('click')
    await wrapper.get('[data-testid="account-priority-input"]').setValue('5')
    await wrapper.get('[data-testid="account-priority-input"]').trigger('keydown', { key: 'Escape' })

    expect(updateAccount).not.toHaveBeenCalled()
    expect(wrapper.find('[data-testid="account-priority-input"]').exists()).toBe(false)
    expect(wrapper.get('[data-test="priority-cell"]').text()).toContain('10')
  })
})
