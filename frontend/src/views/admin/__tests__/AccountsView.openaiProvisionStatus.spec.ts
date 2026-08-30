import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'

import AccountsView from '../AccountsView.vue'

const {
  listAccounts,
  listWithEtag,
  getBatchTodayStats,
  getAllProxies,
  getAllGroups,
  getOpenAIAutoProvisionStatus,
  resetOpenAIAutoProvisionStatus
} = vi.hoisted(() => ({
  listAccounts: vi.fn(),
  listWithEtag: vi.fn(),
  getBatchTodayStats: vi.fn(),
  getAllProxies: vi.fn(),
  getAllGroups: vi.fn(),
  getOpenAIAutoProvisionStatus: vi.fn(),
  resetOpenAIAutoProvisionStatus: vi.fn()
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    accounts: {
      list: listAccounts,
      listWithEtag,
      getBatchTodayStats,
      getUpstreamBillingProbeSettings: vi.fn().mockResolvedValue({ enabled: true, interval_minutes: 30 }),
      delete: vi.fn(),
      batchClearError: vi.fn(),
      batchRefresh: vi.fn(),
      toggleSchedulable: vi.fn()
    },
    settings: { getOpenAIAutoProvisionStatus, resetOpenAIAutoProvisionStatus },
    proxies: { getAll: getAllProxies },
    groups: { getAll: getAllGroups }
  }
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({ showError: vi.fn(), showSuccess: vi.fn(), showInfo: vi.fn() })
}))

vi.mock('@/stores/auth', () => ({
  useAuthStore: () => ({ token: 'test-token' })
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return { ...actual, useI18n: () => ({ t: (key: string) => key }) }
})

function mountView() {
  return mount(AccountsView, {
    global: {
      stubs: {
        AppLayout: { template: '<div><slot /></div>' },
        TablePageLayout: { template: '<div><slot name="filters" /><slot name="table" /><slot name="pagination" /></div>' },
        DataTable: true,
        Pagination: true,
        ConfirmDialog: true,
        AccountTableActions: { template: '<div><slot name="beforeCreate" /><slot name="after" /></div>' },
        AccountTableFilters: { template: '<div />' },
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
        Icon: true
      }
    }
  })
}

describe('admin AccountsView OpenAI provisioning status', () => {
  beforeEach(() => {
    localStorage.clear()
    listAccounts.mockReset().mockResolvedValue({ items: [], total: 0, page: 1, page_size: 20, pages: 0 })
    listWithEtag.mockReset().mockResolvedValue({ notModified: true, etag: null, data: null })
    getBatchTodayStats.mockReset().mockResolvedValue({ stats: {} })
    getAllProxies.mockReset().mockResolvedValue([])
    getAllGroups.mockReset().mockResolvedValue([])
    resetOpenAIAutoProvisionStatus.mockReset().mockResolvedValue({
      phase: 'idle',
      enabled: true,
      target: 3,
      interval_seconds: 15,
      healthy_account_count: 1,
      pending_provision_count: 0,
      pending_reauthorization_count: 1,
      last_provision_requested_count: 2,
      last_callback_requested_count: 0,
      last_callback_succeeded_count: 0,
      last_callback_failed_count: 0,
      last_callback_pending_count: 0,
      provision_dispatch_stale: false,
      provision_resettable: false,
      provision_retry_requested: false
    })
    getOpenAIAutoProvisionStatus.mockReset().mockResolvedValue({
      phase: 'waiting_for_provision_callback',
      enabled: true,
      target: 3,
      interval_seconds: 15,
      healthy_account_count: 1,
      pending_provision_count: 2,
      pending_reauthorization_count: 1,
      last_check_started_at: '2026-08-31T01:00:00Z',
      last_check_completed_at: '2026-08-31T01:00:01Z',
      next_check_at: '2026-08-31T01:00:16Z',
      last_provision_requested_count: 2,
      last_provision_requested_at: '2026-08-31T01:00:01Z',
      last_callback_at: '2026-08-31T00:59:00Z',
      last_callback_kind: 'registration',
      last_callback_status: 'completed',
      last_callback_requested_count: 2,
      last_callback_succeeded_count: 1,
      last_callback_failed_count: 1,
      last_callback_pending_count: 0,
      provision_dispatch_stale: false,
      provision_resettable: true,
      provision_retry_requested: false
    })
  })

  afterEach(() => {
    vi.useRealTimers()
  })

  it('shows provisioning status in the account toolbar and uses the existing refresh cycle', async () => {
    vi.useFakeTimers()
    localStorage.setItem('account-auto-refresh', JSON.stringify({ enabled: true, interval_seconds: 5 }))
    const wrapper = mountView()
    await flushPromises()

    expect(getOpenAIAutoProvisionStatus).toHaveBeenCalledTimes(1)
    await vi.advanceTimersByTimeAsync(6000)
    await flushPromises()
    expect(getOpenAIAutoProvisionStatus).toHaveBeenCalledTimes(2)

    const statusButton = wrapper.get('[data-testid="openai-auto-provision-status"]')
    await statusButton.trigger('click')

    expect(wrapper.get('[data-testid="openai-auto-provision-status-panel"]').text()).toContain(
      'admin.accounts.openaiProvision.phase.waiting_for_provision_callback'
    )
    expect(wrapper.get('[data-testid="openai-auto-provision-healthy"]').text()).toContain('1 / 3')
    expect(wrapper.get('[data-testid="openai-auto-provision-pending"]').text()).toContain('2')
    expect(wrapper.get('[data-testid="openai-auto-provision-next-check"]').text()).toContain('2026')
    wrapper.unmount()
  })

  it('resets pending provisioning from the toolbar status panel', async () => {
    const wrapper = mountView()
    await flushPromises()

    await wrapper.get('[data-testid="openai-auto-provision-status"]').trigger('click')
    await wrapper.get('[data-testid="openai-auto-provision-reset"]').trigger('click')
    await flushPromises()

    expect(resetOpenAIAutoProvisionStatus).toHaveBeenCalledTimes(1)
    expect(wrapper.get('[data-testid="openai-auto-provision-pending"]').text()).toContain('0')
    wrapper.unmount()
  })
})
