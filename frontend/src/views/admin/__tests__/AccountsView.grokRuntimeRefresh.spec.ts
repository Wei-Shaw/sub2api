import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'

import AccountsView from '../AccountsView.vue'
import type { Account } from '@/types'

const {
  listAccounts,
  listWithEtag,
  getBatchTodayStats,
  getAccountById,
  getAllProxies,
  getAllGroups
} = vi.hoisted(() => ({
  listAccounts: vi.fn(),
  listWithEtag: vi.fn(),
  getBatchTodayStats: vi.fn(),
  getAccountById: vi.fn(),
  getAllProxies: vi.fn(),
  getAllGroups: vi.fn()
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    accounts: {
      list: listAccounts,
      listWithEtag,
      getBatchTodayStats,
      getById: getAccountById,
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
    showSuccess: vi.fn(),
    showInfo: vi.fn(),
    showWarning: vi.fn()
  })
}))

vi.mock('@/stores/auth', () => ({
  useAuthStore: () => ({
    token: 'test-token'
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

const baseAccount: Account = {
  id: 42,
  name: 'grok-free',
  platform: 'grok',
  type: 'oauth',
  proxy_id: null,
  concurrency: 1,
  priority: 1,
  status: 'active',
  error_message: null,
  last_used_at: null,
  expires_at: null,
  auto_pause_on_expired: true,
  created_at: '2026-07-15T00:00:00Z',
  updated_at: '2026-07-15T00:00:00Z',
  schedulable: true,
  rate_limited_at: null,
  rate_limit_reset_at: null,
  overload_until: null,
  temp_unschedulable_until: null,
  temp_unschedulable_reason: null,
  session_window_start: null,
  session_window_end: null,
  session_window_status: null
}

const pendingAccount: Account = {
  ...baseAccount,
  updated_at: '2026-07-15T00:01:00Z',
  schedulable: true,
  rate_limited_at: '2026-07-15T00:01:00Z',
  rate_limit_reset_at: '2000-01-01T00:00:00Z',
  grok_free_recovery_pending: true
}

const DataTableStub = {
  props: ['data'],
  template: `
    <div>
      <div v-for="row in data" :key="row.id">
        <slot name="cell-status" :row="row" />
        <slot name="cell-usage" :row="row" />
      </div>
    </div>
  `
}

const AccountStatusIndicatorStub = {
  props: ['account'],
  template: '<span :data-testid="`account-state-${account.id}`">{{ String(account.grok_free_recovery_pending === true) }}</span>'
}

const AccountUsageCellStub = {
  props: ['account'],
  emits: ['account-state-changed'],
  template: '<button :data-testid="`probe-${account.id}`" @click="$emit(\'account-state-changed\', account.id)">probe</button>'
}

const AccountTestModalStub = {
  emits: ['completed'],
  template: '<button data-testid="test-completed" @click="$emit(\'completed\', 42)">complete</button>'
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
        Pagination: true,
        ConfirmDialog: true,
        AccountTableActions: { template: '<div><slot name="beforeCreate" /><slot name="after" /></div>' },
        AccountTableFilters: true,
        AccountBulkActionsBar: true,
        AccountActionMenu: true,
        ImportDataModal: true,
        ReAuthAccountModal: true,
        AccountTestModal: AccountTestModalStub,
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
        AccountStatusIndicator: AccountStatusIndicatorStub,
        AccountTodayStatsCell: true,
        AccountGroupsCell: true,
        AccountUsageCell: AccountUsageCellStub,
        HelpTooltip: true,
        Icon: true
      }
    }
  })
}

describe('admin AccountsView Grok runtime state refresh', () => {
  beforeEach(() => {
    localStorage.clear()
    listAccounts.mockReset().mockResolvedValue({
      items: [{ ...baseAccount }],
      total: 1,
      page: 1,
      page_size: 20,
      pages: 1
    })
    listWithEtag.mockReset().mockResolvedValue({
      notModified: true,
      etag: null,
      data: null
    })
    getBatchTodayStats.mockReset().mockResolvedValue({ stats: {} })
    getAccountById.mockReset().mockResolvedValue({ ...pendingAccount })
    getAllProxies.mockReset().mockResolvedValue([])
    getAllGroups.mockReset().mockResolvedValue([])
  })

  it('refreshes the row immediately after a Grok quota probe', async () => {
    const wrapper = mountView()
    await flushPromises()

    expect(wrapper.get('[data-testid="account-state-42"]').text()).toBe('false')
    await wrapper.get('[data-testid="probe-42"]').trigger('click')
    await flushPromises()

    expect(getAccountById).toHaveBeenCalledWith(42)
    expect(wrapper.get('[data-testid="account-state-42"]').text()).toBe('true')
  })

  it('refreshes the row immediately after an account connection test', async () => {
    const wrapper = mountView()
    await flushPromises()

    await wrapper.get('[data-testid="test-completed"]').trigger('click')
    await flushPromises()

    expect(getAccountById).toHaveBeenCalledWith(42)
    expect(wrapper.get('[data-testid="account-state-42"]').text()).toBe('true')
  })
})
