import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'

import AccountsView from '../AccountsView.vue'

const {
  listAccounts,
  listWithEtag,
  getBatchTodayStats,
  getAllProxies,
  getAllGroups,
  listBatches
} = vi.hoisted(() => ({
  listAccounts: vi.fn(),
  listWithEtag: vi.fn(),
  getBatchTodayStats: vi.fn(),
  getAllProxies: vi.fn(),
  getAllGroups: vi.fn(),
  listBatches: vi.fn()
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

vi.mock('@/api/admin/batches', () => ({
  list: listBatches
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
  props: ['columns', 'data'],
  template: `
    <div data-test="data-table">
      <div v-for="row in data" :key="row.id" :data-test="'account-row-' + row.id">
        <slot name="cell-platform_type" :row="row" />
      </div>
    </div>
  `
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
        BatchManagerModal: true,
        CreateAccountModal: true,
        EditAccountModal: true,
        BulkEditAccountModal: true,
        PlatformTypeBadge: true,
        AccountCapacityCell: true,
        AccountStatusIndicator: true,
        AccountTodayStatsCell: true,
        AccountGroupsCell: true,
        AccountUsageCell: true,
        HelpTooltip: true,
        Icon: true
      }
    }
  })
}

describe('admin AccountsView token presence badges', () => {
  beforeEach(() => {
    localStorage.clear()

    listAccounts.mockReset()
    listWithEtag.mockReset()
    getBatchTodayStats.mockReset()
    getAllProxies.mockReset()
    getAllGroups.mockReset()
    listBatches.mockReset()

    listWithEtag.mockResolvedValue({
      notModified: true,
      etag: null,
      data: null
    })
    getBatchTodayStats.mockResolvedValue({ stats: {} })
    getAllProxies.mockResolvedValue([])
    getAllGroups.mockResolvedValue([])
    listBatches.mockResolvedValue([])
  })

  it('shows only badges for tokens that are actually present', async () => {
    listAccounts.mockResolvedValue({
      items: [
        {
          id: 1,
          name: 'at-only',
          platform: 'openai',
          type: 'oauth',
          status: 'active',
          schedulable: false,
          credentials_status: {
            has_access_token: true
          },
          created_at: '2026-06-12T00:00:00Z',
          updated_at: '2026-06-12T00:00:00Z'
        },
        {
          id: 2,
          name: 'rt-only',
          platform: 'gemini',
          type: 'oauth',
          status: 'active',
          schedulable: false,
          credentials_status: {
            has_refresh_token: true
          },
          created_at: '2026-06-12T00:00:00Z',
          updated_at: '2026-06-12T00:00:00Z'
        },
        {
          id: 3,
          name: 'none',
          platform: 'anthropic',
          type: 'oauth',
          status: 'active',
          schedulable: false,
          credentials_status: {},
          created_at: '2026-06-12T00:00:00Z',
          updated_at: '2026-06-12T00:00:00Z'
        }
      ],
      total: 3,
      page: 1,
      page_size: 20,
      pages: 1
    })

    const wrapper = mountView()
    await flushPromises()

    const atOnlyRow = wrapper.get('[data-test="account-row-1"]')
    expect(atOnlyRow.get('[data-test="token-badge-access_token"]').text()).toBe('AT')
    expect(atOnlyRow.get('[data-test="token-badge-access_token"]').attributes('title')).toBe(
      'admin.accounts.tokenPresence.accessTokenPresent'
    )
    expect(atOnlyRow.find('[data-test="token-badge-refresh_token"]').exists()).toBe(false)

    const rtOnlyRow = wrapper.get('[data-test="account-row-2"]')
    expect(rtOnlyRow.find('[data-test="token-badge-access_token"]').exists()).toBe(false)
    expect(rtOnlyRow.get('[data-test="token-badge-refresh_token"]').text()).toBe('RT')
    expect(rtOnlyRow.get('[data-test="token-badge-refresh_token"]').attributes('title')).toBe(
      'admin.accounts.tokenPresence.refreshTokenPresent'
    )

    const noneRow = wrapper.get('[data-test="account-row-3"]')
    expect(noneRow.find('[data-test^="token-badge-"]').exists()).toBe(false)
    expect(wrapper.text()).not.toContain('accessTokenMissing')
    expect(wrapper.text()).not.toContain('refreshTokenMissing')
  })
})
