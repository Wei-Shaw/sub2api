import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { defineComponent } from 'vue'

import AccountsView from '../AccountsView.vue'

const {
  f102ListAccounts,
  f102ListWithEtag,
  f102GetBatchTodayStats,
  f102GetUpstreamBillingProbeSettings,
  f102GetAllProxies,
  f102GetAllGroups,
  f102ShowError
} = vi.hoisted(() => ({
  f102ListAccounts: vi.fn(),
  f102ListWithEtag: vi.fn(),
  f102GetBatchTodayStats: vi.fn(),
  f102GetUpstreamBillingProbeSettings: vi.fn(),
  f102GetAllProxies: vi.fn(),
  f102GetAllGroups: vi.fn(),
  f102ShowError: vi.fn()
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    accounts: {
      list: f102ListAccounts,
      listWithEtag: f102ListWithEtag,
      getBatchTodayStats: f102GetBatchTodayStats,
      getUpstreamBillingProbeSettings: f102GetUpstreamBillingProbeSettings
    },
    proxies: { getAll: f102GetAllProxies },
    groups: { getAll: f102GetAllGroups }
  }
}))

vi.mock('vue-router', () => ({
  useRoute: () => ({ query: {} })
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({ showError: f102ShowError, showSuccess: vi.fn(), showInfo: vi.fn(), showWarning: vi.fn() })
}))

vi.mock('@/stores/auth', () => ({
  useAuthStore: () => ({ token: 'test-token' })
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return { ...actual, useI18n: () => ({ t: (key: string) => key }) }
})

const f102Account = (id: number, platform: string, type: string) => ({
  id,
  name: `account-${id}`,
  platform,
  type,
  status: 'active',
  schedulable: true,
  created_at: '2026-09-01T00:00:00Z',
  updated_at: '2026-09-01T00:00:00Z'
})

// 150 matching accounts: the first 100 (by sort order) are OpenAI API keys, the rest Anthropic OAuth.
const f102AllAccounts = Array.from({ length: 150 }, (_, index) =>
  index < 100 ? f102Account(index + 1, 'openai', 'apikey') : f102Account(index + 1, 'anthropic', 'oauth')
)

const f102PagedList = async (page: number, pageSize: number) => {
  const start = (page - 1) * pageSize
  return {
    items: f102AllAccounts.slice(start, start + pageSize),
    total: f102AllAccounts.length,
    page,
    page_size: pageSize,
    pages: Math.ceil(f102AllAccounts.length / pageSize)
  }
}

const f102DataTableStub = defineComponent({
  name: 'DataTable',
  props: { data: { type: Array, default: () => [] } },
  template: '<div><div v-for="row in data" :key="row.id" data-test="select-row"><slot name="cell-select" :row="row" /></div></div>'
})

const f102PaginationStub = defineComponent({
  name: 'Pagination',
  emits: ['update:page', 'update:pageSize'],
  template: '<button data-test="next-page" @click="$emit(\'update:page\', 2)">next</button>'
})

const f102BulkActionsBarStub = defineComponent({
  name: 'AccountBulkActionsBar',
  props: ['selectedIds', 'busy'],
  emits: ['edit-selected', 'edit-filtered', 'select-all-results'],
  template: `
    <div>
      <button data-test="select-all-results" @click="$emit('select-all-results')">all</button>
      <button data-test="edit-selected" @click="$emit('edit-selected')">edit selected</button>
      <button data-test="edit-filtered" @click="$emit('edit-filtered')">edit filtered</button>
    </div>
  `
})

const f102BulkEditModalStub = defineComponent({
  name: 'BulkEditAccountModal',
  props: ['show', 'target'],
  template: '<div data-test="bulk-edit-modal" :data-show="String(show)" />'
})

const f102MountView = () => mount(AccountsView, {
  global: {
    stubs: {
      AppLayout: { template: '<div><slot /></div>' },
      TablePageLayout: { template: '<div><slot name="table" /><slot name="pagination" /></div>' },
      DataTable: f102DataTableStub,
      Pagination: f102PaginationStub,
      ConfirmDialog: true,
      AccountTableActions: true,
      AccountTableFilters: true,
      AccountBulkActionsBar: f102BulkActionsBarStub,
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
      BulkEditAccountModal: f102BulkEditModalStub,
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

type F102Target = { mode: string; selectedPlatforms: string[]; selectedTypes: string[]; previewCount?: number; accountIds?: number[] }

const f102Target = (wrapper: ReturnType<typeof f102MountView>) =>
  wrapper.getComponent(f102BulkEditModalStub).props('target') as F102Target

describe('admin AccountsView bulk edit selection metadata (F1-02)', () => {
  beforeEach(() => {
    localStorage.clear()
    f102ListAccounts.mockReset()
    f102ListWithEtag.mockReset().mockResolvedValue({ notModified: true, etag: null, data: null })
    f102GetBatchTodayStats.mockReset().mockResolvedValue({ stats: {} })
    f102GetUpstreamBillingProbeSettings.mockReset().mockResolvedValue({ enabled: true })
    f102GetAllProxies.mockReset().mockResolvedValue([])
    f102GetAllGroups.mockReset().mockResolvedValue([])
    f102ShowError.mockReset()
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('derives platforms and types from rows selected on different pages', async () => {
    f102ListAccounts
      .mockResolvedValueOnce({ items: [f102Account(7, 'openai', 'apikey')], total: 2, page: 1, page_size: 1, pages: 2 })
      .mockResolvedValueOnce({ items: [f102Account(11, 'anthropic', 'oauth')], total: 2, page: 2, page_size: 1, pages: 2 })
    const wrapper = f102MountView()
    await flushPromises()

    await wrapper.get('[data-test="select-row"] input').trigger('change')
    await wrapper.get('[data-test="next-page"]').trigger('click')
    await flushPromises()
    await wrapper.get('[data-test="select-row"] input').trigger('change')
    await wrapper.get('[data-test="edit-selected"]').trigger('click')
    await flushPromises()

    const target = f102Target(wrapper)
    expect(target.accountIds).toEqual([7, 11])
    expect([...target.selectedPlatforms].sort()).toEqual(['anthropic', 'openai'])
    expect([...target.selectedTypes].sort()).toEqual(['apikey', 'oauth'])
    wrapper.unmount()
  })

  it('derives platforms and types from every result after "select all results"', async () => {
    f102ListAccounts.mockImplementation(f102PagedList)
    const wrapper = f102MountView()
    await flushPromises()

    await wrapper.get('[data-test="select-all-results"]').trigger('click')
    await flushPromises()
    await wrapper.get('[data-test="edit-selected"]').trigger('click')
    await flushPromises()

    const target = f102Target(wrapper)
    expect(target.accountIds).toHaveLength(150)
    expect([...target.selectedPlatforms].sort()).toEqual(['anthropic', 'openai'])
    expect([...target.selectedTypes].sort()).toEqual(['apikey', 'oauth'])
    wrapper.unmount()
  })

  it('derives platforms and types from every matching account in filtered mode', async () => {
    f102ListAccounts.mockImplementation(f102PagedList)
    const wrapper = f102MountView()
    await flushPromises()

    await wrapper.get('[data-test="edit-filtered"]').trigger('click')
    await flushPromises()

    expect(wrapper.get('[data-test="bulk-edit-modal"]').attributes('data-show')).toBe('true')
    const target = f102Target(wrapper)
    expect(target.mode).toBe('filtered')
    expect(target.previewCount).toBe(150)
    expect([...target.selectedPlatforms].sort()).toEqual(['anthropic', 'openai'])
    expect([...target.selectedTypes].sort()).toEqual(['apikey', 'oauth'])
    wrapper.unmount()
  })
})
