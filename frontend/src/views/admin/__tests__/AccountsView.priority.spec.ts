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
  showSuccess
} = vi.hoisted(() => ({
  listAccounts: vi.fn(),
  listWithEtag: vi.fn(),
  updateAccount: vi.fn(),
  getBatchTodayStats: vi.fn(),
  getAllProxies: vi.fn(),
  getAllGroups: vi.fn(),
  showSuccess: vi.fn()
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    accounts: {
      list: listAccounts,
      listWithEtag,
      update: updateAccount,
      getBatchTodayStats,
      delete: vi.fn(),
      batchClearError: vi.fn(),
      batchRefresh: vi.fn(),
      toggleSchedulable: vi.fn()
    },
    proxies: {
      getAll: getAllProxies
    },
    groups: {
      getAll: getAllGroups
    }
  }
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({
    showError: vi.fn(),
    showSuccess,
    showInfo: vi.fn()
  })
}))

vi.mock('@/stores/auth', () => ({
  useAuthStore: () => ({
    token: 'test-token',
    isSimpleMode: false
  })
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string) => key
    })
  }
})

const DataTableStub = {
  props: ['data'],
  template: `
    <div>
      <div v-for="row in data" :key="row.id" :data-test="'priority-' + row.id">
        <slot name="cell-priority" :row="row" :value="row.priority" />
      </div>
    </div>
  `
}

const baseAccount = {
  id: 1,
  name: 'openai-account',
  platform: 'openai',
  type: 'apikey',
  status: 'active',
  schedulable: true,
  concurrency: 1,
  priority: 1000,
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
        TablePageLayout: {
          template: '<div><slot name="filters" /><slot name="table" /><slot name="pagination" /></div>'
        },
        DataTable: DataTableStub,
        HelpTooltip: true,
        Pagination: true,
        ConfirmDialog: true,
        AccountTableActions: { template: '<div><slot name="after" /></div>' },
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
        Icon: true
      }
    }
  })
}

describe('admin AccountsView priority editing', () => {
  beforeEach(() => {
    localStorage.clear()
    localStorage.setItem(
      'account-hidden-columns',
      JSON.stringify(['today_stats', 'proxy', 'notes', 'scheduler_score', 'rate_multiplier'])
    )
    localStorage.setItem('account-hidden-columns-version', 'scheduler-score-hidden-by-default')

    listAccounts.mockReset()
    listWithEtag.mockReset()
    updateAccount.mockReset()
    getBatchTodayStats.mockReset()
    getAllProxies.mockReset()
    getAllGroups.mockReset()
    showSuccess.mockReset()

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

  it('updates the account and patches the row with the API response', async () => {
    let resolveUpdate: ((account: typeof baseAccount) => void) | undefined
    updateAccount.mockImplementation(() => new Promise((resolve) => {
      resolveUpdate = resolve
    }))

    const wrapper = mountView()
    await flushPromises()
    await wrapper.get('[data-test="priority-1"] [data-test="priority-value"]').trigger('dblclick')
    const input = wrapper.get('[data-test="priority-1"] input')

    await input.setValue('50')
    await input.trigger('blur')

    expect(updateAccount).toHaveBeenCalledWith(1, { priority: 50 })
    expect(wrapper.find('[data-test="priority-1"] input').exists()).toBe(false)
    expect(wrapper.get('[data-test="priority-1"] [data-test="priority-value"]').text()).toBe('50')
    expect(wrapper.find('[data-test="priority-1"] [data-test="priority-saving"]').exists()).toBe(true)

    resolveUpdate?.({
      ...baseAccount,
      priority: 50,
      updated_at: '2026-01-02T00:00:00Z'
    })
    await flushPromises()

    expect(wrapper.get('[data-test="priority-1"] [data-test="priority-value"]').text()).toBe('50')
    expect(wrapper.find('[data-test="priority-1"] [data-test="priority-saving"]').exists()).toBe(false)
    expect(showSuccess).toHaveBeenCalledWith('admin.accounts.priorityUpdated')
  })
})
