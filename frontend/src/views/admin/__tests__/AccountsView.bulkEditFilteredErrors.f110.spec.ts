import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { defineComponent } from 'vue'

import AccountsView from '../AccountsView.vue'

const {
  f110ListAccounts,
  f110ListWithEtag,
  f110GetBatchTodayStats,
  f110GetUpstreamBillingProbeSettings,
  f110GetAllProxies,
  f110GetAllGroups,
  f110ShowError
} = vi.hoisted(() => ({
  f110ListAccounts: vi.fn(),
  f110ListWithEtag: vi.fn(),
  f110GetBatchTodayStats: vi.fn(),
  f110GetUpstreamBillingProbeSettings: vi.fn(),
  f110GetAllProxies: vi.fn(),
  f110GetAllGroups: vi.fn(),
  f110ShowError: vi.fn()
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    accounts: {
      list: f110ListAccounts,
      listWithEtag: f110ListWithEtag,
      getBatchTodayStats: f110GetBatchTodayStats,
      getUpstreamBillingProbeSettings: f110GetUpstreamBillingProbeSettings
    },
    proxies: { getAll: f110GetAllProxies },
    groups: { getAll: f110GetAllGroups }
  }
}))

vi.mock('vue-router', () => ({
  useRoute: () => ({ query: {} })
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({ showError: f110ShowError, showSuccess: vi.fn(), showInfo: vi.fn(), showWarning: vi.fn() })
}))

vi.mock('@/stores/auth', () => ({
  useAuthStore: () => ({ token: 'test-token' })
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return { ...actual, useI18n: () => ({ t: (key: string) => key }) }
})

const f110Page = {
  items: [{
    id: 1,
    name: 'account-1',
    platform: 'anthropic',
    type: 'oauth',
    status: 'active',
    schedulable: true,
    created_at: '2026-09-01T00:00:00Z',
    updated_at: '2026-09-01T00:00:00Z'
  }],
  total: 1,
  page: 1,
  page_size: 20,
  pages: 1
}

const f110BulkActionsBarStub = defineComponent({
  name: 'AccountBulkActionsBar',
  props: ['selectedIds', 'busy'],
  emits: ['edit-filtered'],
  template: `
    <div>
      <span data-test="busy">{{ String(busy) }}</span>
      <button data-test="edit-filtered" @click="$emit('edit-filtered')">edit filtered</button>
    </div>
  `
})

const f110MountView = () => mount(AccountsView, {
  global: {
    stubs: {
      AppLayout: { template: '<div><slot /></div>' },
      TablePageLayout: { template: '<div><slot name="table" /></div>' },
      DataTable: true,
      Pagination: true,
      ConfirmDialog: true,
      AccountTableActions: true,
      AccountTableFilters: true,
      AccountBulkActionsBar: f110BulkActionsBarStub,
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
      BulkEditAccountModal: { props: ['show'], template: '<div data-test="bulk-edit-modal" :data-show="String(show)" />' },
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

describe('admin AccountsView filtered bulk edit errors (F1-10)', () => {
  beforeEach(() => {
    localStorage.clear()
    f110ListAccounts.mockReset().mockResolvedValue(f110Page)
    f110ListWithEtag.mockReset().mockResolvedValue({ notModified: true, etag: null, data: null })
    f110GetBatchTodayStats.mockReset().mockResolvedValue({ stats: {} })
    f110GetUpstreamBillingProbeSettings.mockReset().mockResolvedValue({ enabled: true })
    f110GetAllProxies.mockReset().mockResolvedValue([])
    f110GetAllGroups.mockReset().mockResolvedValue([])
    f110ShowError.mockReset()
    vi.spyOn(console, 'error').mockImplementation(() => {})
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('shows the list error, keeps the modal closed and resets the busy flag', async () => {
    const wrapper = f110MountView()
    await flushPromises()

    f110ListAccounts.mockRejectedValueOnce({ status: 500, message: 'upstream timeout' })
    await wrapper.get('[data-test="edit-filtered"]').trigger('click')
    await flushPromises()

    expect(f110ShowError).toHaveBeenCalledWith('upstream timeout')
    expect(wrapper.get('[data-test="bulk-edit-modal"]').attributes('data-show')).toBe('false')
    expect(wrapper.get('[data-test="busy"]').text()).toBe('false')
    wrapper.unmount()
  })

  it('marks bulk actions busy while loading and ignores repeated clicks', async () => {
    const wrapper = f110MountView()
    await flushPromises()
    const f110CallsBefore = f110ListAccounts.mock.calls.length

    let f110Resolve: (value: unknown) => void = () => {}
    f110ListAccounts.mockImplementationOnce(() => new Promise(resolve => { f110Resolve = resolve }))
    await wrapper.get('[data-test="edit-filtered"]').trigger('click')
    await flushPromises()
    expect(wrapper.get('[data-test="busy"]').text()).toBe('true')

    await wrapper.get('[data-test="edit-filtered"]').trigger('click')
    await flushPromises()
    expect(f110ListAccounts.mock.calls.length).toBe(f110CallsBefore + 1)

    f110Resolve({ ...f110Page, page_size: 1000 })
    await flushPromises()
    expect(wrapper.get('[data-test="busy"]').text()).toBe('false')
    expect(wrapper.get('[data-test="bulk-edit-modal"]').attributes('data-show')).toBe('true')
    wrapper.unmount()
  })
})
