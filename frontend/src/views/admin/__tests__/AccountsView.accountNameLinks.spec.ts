import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'

import AccountsView from '../AccountsView.vue'

const {
  listAccounts,
  listWithEtag,
  getBatchTodayStats,
  getAllProxies,
  getAllGroups
} = vi.hoisted(() => ({
  listAccounts: vi.fn(),
  listWithEtag: vi.fn(),
  getBatchTodayStats: vi.fn(),
  getAllProxies: vi.fn(),
  getAllGroups: vi.fn()
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    accounts: {
      list: listAccounts,
      listWithEtag,
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
    showSuccess: vi.fn(),
    showInfo: vi.fn()
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

const DataTableStub = {
  props: ['columns', 'data'],
  template: `
    <div data-test="data-table">
      <div v-for="row in data" :key="row.id" data-test="account-row">
        <slot name="cell-name" :value="row.name" :row="row" />
      </div>
    </div>
  `
}

function accountFixture(overrides: Record<string, unknown>) {
  return {
    id: 1,
    name: 'test-account',
    platform: 'openai',
    type: 'apikey',
    status: 'active',
    schedulable: true,
    created_at: '2026-03-07T10:00:00Z',
    updated_at: '2026-03-07T10:00:00Z',
    ...overrides
  }
}

async function mountWithAccounts(items: Array<Record<string, unknown>>) {
  listAccounts.mockResolvedValue({
    items,
    total: items.length,
    page: 1,
    page_size: 20,
    pages: 1
  })
  listWithEtag.mockResolvedValue({
    notModified: true,
    etag: null,
    data: null
  })
  getBatchTodayStats.mockResolvedValue({ stats: {} })
  getAllProxies.mockResolvedValue([])
  getAllGroups.mockResolvedValue([])

  const wrapper = mount(AccountsView, {
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
        AccountTableFilters: { template: '<div></div>' },
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

  await flushPromises()
  return wrapper
}

describe('admin AccountsView account name links', () => {
  beforeEach(() => {
    localStorage.clear()
    listAccounts.mockReset()
    listWithEtag.mockReset()
    getBatchTodayStats.mockReset()
    getAllProxies.mockReset()
    getAllGroups.mockReset()
  })

  it('renders an API key account name with an http URL as an external link', async () => {
    const wrapper = await mountWithAccounts([
      accountFixture({ name: 'https://example.com/path?tab=accounts' })
    ])

    const link = wrapper.get('a')
    expect(link.text()).toBe('https://example.com/path?tab=accounts')
    expect(link.attributes('href')).toBe('https://example.com/path?tab=accounts')
    expect(link.attributes('target')).toBe('_blank')
    expect(link.attributes('rel')).toBe('noopener noreferrer')
  })

  it('normalizes a bare website domain API key account name to https href', async () => {
    const wrapper = await mountWithAccounts([
      accountFixture({ name: 'example.com' })
    ])

    const link = wrapper.get('a')
    expect(link.text()).toBe('example.com')
    expect(link.attributes('href')).toBe('https://example.com')
  })

  it('keeps URL-like names plain for non-API-key accounts', async () => {
    const wrapper = await mountWithAccounts([
      accountFixture({ name: 'https://example.com', type: 'oauth' })
    ])

    expect(wrapper.find('a').exists()).toBe(false)
    expect(wrapper.text()).toContain('https://example.com')
  })

  it('keeps non-URL API key account names plain', async () => {
    const wrapper = await mountWithAccounts([
      accountFixture({ name: 'primary-openai-key' })
    ])

    expect(wrapper.find('a').exists()).toBe(false)
    expect(wrapper.text()).toContain('primary-openai-key')
  })
})
