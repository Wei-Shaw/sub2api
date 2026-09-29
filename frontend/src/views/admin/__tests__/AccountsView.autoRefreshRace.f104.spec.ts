import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { defineComponent } from 'vue'

import AccountsView from '../AccountsView.vue'

const {
  f104ListAccounts,
  f104ListWithEtag,
  f104GetBatchTodayStats,
  f104GetUpstreamBillingProbeSettings,
  f104GetAllProxies,
  f104GetAllGroups
} = vi.hoisted(() => ({
  f104ListAccounts: vi.fn(),
  f104ListWithEtag: vi.fn(),
  f104GetBatchTodayStats: vi.fn(),
  f104GetUpstreamBillingProbeSettings: vi.fn(),
  f104GetAllProxies: vi.fn(),
  f104GetAllGroups: vi.fn()
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    accounts: {
      list: f104ListAccounts,
      listWithEtag: f104ListWithEtag,
      getBatchTodayStats: f104GetBatchTodayStats,
      getUpstreamBillingProbeSettings: f104GetUpstreamBillingProbeSettings
    },
    proxies: { getAll: f104GetAllProxies },
    groups: { getAll: f104GetAllGroups }
  }
}))

vi.mock('vue-router', () => ({
  useRoute: () => ({ query: {} })
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({ showError: vi.fn(), showSuccess: vi.fn(), showInfo: vi.fn(), showWarning: vi.fn() })
}))

vi.mock('@/stores/auth', () => ({
  useAuthStore: () => ({ token: 'test-token' })
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return { ...actual, useI18n: () => ({ t: (key: string) => key }) }
})

const f104Account = (id: number) => ({
  id,
  name: `account-${id}`,
  platform: 'anthropic',
  type: 'oauth',
  status: 'active',
  schedulable: true,
  created_at: '2026-09-01T00:00:00Z',
  updated_at: '2026-09-01T00:00:00Z'
})

const f104DataTableStub = defineComponent({
  name: 'DataTable',
  props: { data: { type: Array, default: () => [] } },
  template: '<div data-test="rows">{{ data.map(row => row.id).join(",") }}</div>'
})

const f104PaginationStub = defineComponent({
  name: 'Pagination',
  props: ['page', 'total', 'pageSize'],
  emits: ['update:page', 'update:pageSize'],
  template: '<button data-test="page-2" @click="$emit(\'update:page\', 2)">2</button>'
})

const f104MountView = () => mount(AccountsView, {
  global: {
    stubs: {
      AppLayout: { template: '<div><slot /></div>' },
      TablePageLayout: { template: '<div><slot name="table" /><slot name="pagination" /></div>' },
      DataTable: f104DataTableStub,
      Pagination: f104PaginationStub,
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
      Icon: true
    }
  }
})

describe('admin AccountsView auto refresh race (F1-04)', () => {
  beforeEach(() => {
    localStorage.clear()
    f104ListAccounts.mockReset()
    f104ListWithEtag.mockReset()
    f104GetBatchTodayStats.mockReset().mockResolvedValue({ stats: {} })
    f104GetUpstreamBillingProbeSettings.mockReset().mockResolvedValue({ enabled: true })
    f104GetAllProxies.mockReset().mockResolvedValue([])
    f104GetAllGroups.mockReset().mockResolvedValue([])
  })

  afterEach(() => {
    vi.useRealTimers()
    vi.restoreAllMocks()
  })

  it('discards an ETag refresh response that arrives after the user moved to another page', async () => {
    vi.useFakeTimers()
    vi.spyOn(document, 'hidden', 'get').mockReturnValue(false)
    localStorage.setItem('account-auto-refresh', JSON.stringify({ enabled: true, interval_seconds: 5 }))
    f104ListAccounts.mockResolvedValueOnce({ items: [f104Account(1), f104Account(2)], total: 40, page: 1, page_size: 20, pages: 2 })
    let f104ResolveEtag: (value: unknown) => void = () => {}
    f104ListWithEtag.mockImplementationOnce(() => new Promise(resolve => { f104ResolveEtag = resolve }))
    f104ListWithEtag.mockResolvedValue({ notModified: true, etag: null, data: null })

    const wrapper = f104MountView()
    await flushPromises()
    expect(wrapper.get('[data-test="rows"]').text()).toBe('1,2')

    await vi.advanceTimersByTimeAsync(6000)
    expect(f104ListWithEtag).toHaveBeenCalledTimes(1)
    expect(f104ListWithEtag.mock.calls[0][0]).toBe(1)

    f104ListAccounts.mockResolvedValueOnce({ items: [f104Account(3), f104Account(4)], total: 40, page: 2, page_size: 20, pages: 2 })
    await wrapper.get('[data-test="page-2"]').trigger('click')
    await flushPromises()
    expect(wrapper.get('[data-test="rows"]').text()).toBe('3,4')

    // The stale page-1 response lands after the page-2 load finished.
    f104ResolveEtag({
      notModified: false,
      etag: '"stale-page-1"',
      data: { items: [f104Account(1), f104Account(2)], total: 40, page: 1, page_size: 20, pages: 2 }
    })
    await flushPromises()

    expect(wrapper.get('[data-test="rows"]').text()).toBe('3,4')

    // The stale etag must not be reused for the page-2 query either.
    await vi.advanceTimersByTimeAsync(6000)
    const f104LastCall = f104ListWithEtag.mock.calls[f104ListWithEtag.mock.calls.length - 1]
    expect(f104LastCall[0]).toBe(2)
    expect(f104LastCall[3]).toMatchObject({ etag: null })
    wrapper.unmount()
  })
})
