import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'

import AccountsView from '../AccountsView.vue'

// 回归防护：request-batched-usage 只能传给父页面真会批量拉取的账号。
// 一旦传了该回调，AccountUsageCell 就进入批量托管模式并放弃自身取数；
// 若绑定条件与 accountSupportsBatchUsage 不一致（Kiro 曾因此永久显示 "-"），
// 该平台的用量窗口会彻底失效。
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
      getUpstreamBillingProbeSettings: vi
        .fn()
        .mockResolvedValue({ enabled: true, interval_minutes: 30 }),
      delete: vi.fn(),
      batchClearError: vi.fn(),
      batchRefresh: vi.fn(),
      toggleSchedulable: vi.fn()
    },
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

// 渲染 cell-usage slot，才能拿到传给 AccountUsageCell 的 props
const DataTableStub = {
  props: ['columns', 'data'],
  template: `
    <div data-test="data-table">
      <div v-for="row in data" :key="row.id" :data-test="'row-' + row.id">
        <slot name="cell-usage" :row="row" />
      </div>
    </div>
  `
}

// 捕获每个账号收到的 requestBatchedUsage
const capturedProps: Record<string, { requestBatchedUsage: unknown }> = {}
const AccountUsageCellSpy = {
  props: ['account', 'requestBatchedUsage'],
  setup(props: { account: { id: number }; requestBatchedUsage: unknown }) {
    capturedProps[String(props.account.id)] = {
      requestBatchedUsage: props.requestBatchedUsage
    }
    return () => null
  }
}

function accountFixture(over: Record<string, unknown>) {
  return {
    id: 1,
    name: 'acc',
    platform: 'anthropic',
    type: 'oauth',
    status: 'active',
    schedulable: true,
    priority: 50,
    weight: 1,
    groups: [],
    credentials: {},
    created_at: '2026-01-01T00:00:00Z',
    ...over
  }
}

function mountView() {
  return mount(AccountsView, {
    global: {
      stubs: {
        AppLayout: { template: '<div><slot /></div>' },
        TablePageLayout: {
          template:
            '<div><slot name="filters" /><slot name="table" /><slot name="pagination" /></div>'
        },
        DataTable: DataTableStub,
        AccountUsageCell: AccountUsageCellSpy,
        HelpTooltip: true,
        Pagination: true,
        ConfirmDialog: true,
        AccountTableActions: {
          template: '<div><slot name="beforeCreate" /><slot name="after" /></div>'
        },
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
        Icon: true
      }
    }
  })
}

describe('admin AccountsView batched usage wiring', () => {
  beforeEach(() => {
    localStorage.clear()
    for (const key of Object.keys(capturedProps)) delete capturedProps[key]

    // 绑定条件要求桌面视口
    vi.stubGlobal('matchMedia', (query: string) => ({
      matches: true,
      media: query,
      onchange: null,
      addEventListener: vi.fn(),
      removeEventListener: vi.fn(),
      addListener: vi.fn(),
      removeListener: vi.fn(),
      dispatchEvent: vi.fn()
    }))

    for (const fn of [listAccounts, listWithEtag, getBatchTodayStats, getAllProxies, getAllGroups]) {
      fn.mockReset()
    }
    listAccounts.mockResolvedValue({
      items: [
        accountFixture({ id: 101, platform: 'anthropic', type: 'oauth' }),
        accountFixture({ id: 102, platform: 'kiro', type: 'oauth' }),
        accountFixture({
          id: 103,
          platform: 'kiro',
          type: 'api_key',
          credentials: { api_key: 'k', base_url: 'https://relay.example.com' }
        })
      ],
      total: 3,
      page: 1,
      page_size: 20,
      pages: 1
    })
    listWithEtag.mockResolvedValue({ notModified: true, etag: null, data: null })
    getBatchTodayStats.mockResolvedValue({ stats: {} })
    getAllProxies.mockResolvedValue([])
    getAllGroups.mockResolvedValue([])
  })

  it('批量托管只覆盖父页面支持批量的平台，Kiro 保留自身取数路径', async () => {
    const wrapper = mountView()
    await flushPromises()

    // Anthropic OAuth 在 accountSupportsBatchUsage 名单内 → 交给批量
    expect(typeof capturedProps['101']?.requestBatchedUsage).toBe('function')

    // Kiro 不在名单内 → 必须为 null，否则单元格会放弃取数并永久显示 "-"
    expect(capturedProps['102']?.requestBatchedUsage).toBeNull()
    expect(capturedProps['103']?.requestBatchedUsage).toBeNull()

    wrapper.unmount()
  })
})
