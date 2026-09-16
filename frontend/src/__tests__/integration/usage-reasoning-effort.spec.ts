import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { defineComponent } from 'vue'

import UserUsageView from '@/views/user/UsageView.vue'
import AdminUsageView from '@/views/admin/UsageView.vue'

const {
  userQuery,
  userGetStats,
  userGetDashboardModels,
  userGetDashboardSnapshotV2,
  userKeysList,
  userGroupsGetAvailable,
  adminList,
  adminGetStats,
  adminGetSnapshotV2,
  adminGetModelStats,
  adminGetById,
  listErrorLogs,
} = vi.hoisted(() => ({
  userQuery: vi.fn(),
  userGetStats: vi.fn(),
  userGetDashboardModels: vi.fn(),
  userGetDashboardSnapshotV2: vi.fn(),
  userKeysList: vi.fn(),
  userGroupsGetAvailable: vi.fn(),
  adminList: vi.fn(),
  adminGetStats: vi.fn(),
  adminGetSnapshotV2: vi.fn(),
  adminGetModelStats: vi.fn(),
  adminGetById: vi.fn(),
  listErrorLogs: vi.fn(),
}))

const messages: Record<string, string> = {
  'admin.dashboard.timeRange': 'Time range',
  'admin.dashboard.granularity': 'Granularity',
  'admin.dashboard.day': 'Day',
  'admin.dashboard.hour': 'Hour',
  'admin.users.columnSettings': 'Columns',
  'admin.usage.group': 'Group',
  'admin.usage.user': 'User',
  'admin.usage.account': 'Account',
  'admin.usage.billingType': 'Billing type',
  'admin.usage.billingMode': 'Billing mode',
  'admin.usage.allTypes': 'All types',
  'admin.usage.allBillingTypes': 'All billing types',
  'admin.usage.billingTypeBalance': 'Balance',
  'admin.usage.billingTypeSubscription': 'Subscription',
  'admin.usage.allBillingModes': 'All billing modes',
  'admin.usage.billingModeToken': 'Token',
  'admin.usage.billingModePerRequest': 'Per request',
  'admin.usage.billingModeImage': 'Image',
  'admin.usage.allGroups': 'All groups',
  'admin.usage.allModels': 'All models',
  'admin.usage.ipAddress': 'IP',
  'usage.allApiKeys': 'All API Keys',
  'usage.apiKeyFilter': 'API Key',
  'usage.model': 'Model',
  'usage.type': 'Type',
  'usage.reasoningEffort': 'Reasoning Effort',
  'usage.endpoint': 'Endpoint',
  'usage.tokens': 'Tokens',
  'usage.cost': 'Cost',
  'usage.latency': 'Latency',
  'usage.time': 'Time',
  'usage.userAgent': 'User Agent',
  'usage.ws': 'WS',
  'usage.stream': 'Stream',
  'usage.sync': 'Sync',
  'usage.exporting': 'Exporting',
  'usage.exportCsv': 'Export CSV',
  'usage.failedToLoad': 'Failed to load',
  'usage.noRecords': 'No records',
  'empty.noData': 'No data',
  'common.refresh': 'Refresh',
  'common.reset': 'Reset',
}

vi.mock('@/api', () => ({
  usageAPI: {
    query: userQuery,
    getStats: userGetStats,
    getDashboardModels: userGetDashboardModels,
    getDashboardSnapshotV2: userGetDashboardSnapshotV2,
  },
  keysAPI: {
    list: userKeysList,
  },
  userGroupsAPI: {
    getAvailable: userGroupsGetAvailable,
  },
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    usage: {
      list: adminList,
      getStats: adminGetStats,
    },
    dashboard: {
      getSnapshotV2: adminGetSnapshotV2,
      getModelStats: adminGetModelStats,
    },
    users: {
      getById: adminGetById,
    },
  },
}))

vi.mock('@/api/admin/usage', () => ({
  adminUsageAPI: {
    list: adminList,
  },
}))

vi.mock('@/api/admin/ops', () => ({
  listErrorLogs,
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({
    showError: vi.fn(),
    showWarning: vi.fn(),
    showSuccess: vi.fn(),
    showInfo: vi.fn(),
  }),
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string) => messages[key] ?? key,
    }),
  }
})

vi.mock('vue-router', () => ({
  useRoute: () => ({ query: {} }),
}))

const layoutStub = { template: '<div><slot /></div>' }
const chartStub = { template: '<div />' }
const iconStub = { template: '<span />' }
const adminFiltersStub = defineComponent({
  template: '<div><slot name="after-reset" /></div>',
})

const emptyStats = {
  total_requests: 1,
  total_input_tokens: 10,
  total_output_tokens: 20,
  total_cache_tokens: 0,
  total_tokens: 30,
  total_cost: 0.1,
  total_actual_cost: 0.08,
  average_duration_ms: 12,
  endpoints: [],
  upstream_endpoints: [],
  endpoint_paths: [],
}

const userMappedLog = {
  id: 11,
  request_id: 'req-user-mapped',
  created_at: '2026-08-25T00:00:00Z',
  model: 'gpt-5.4',
  reasoning_effort: 'max',
  actual_cost: 0.01,
  total_cost: 0.01,
  rate_multiplier: 1,
  input_cost: 0.01,
  output_cost: 0,
  cache_creation_cost: 0,
  cache_read_cost: 0,
  input_tokens: 8,
  output_tokens: 2,
  cache_creation_tokens: 0,
  cache_read_tokens: 0,
  cache_creation_5m_tokens: 0,
  cache_creation_1h_tokens: 0,
  image_count: 0,
  billing_mode: 'token',
  request_type: 'sync',
  stream: false,
  api_key: { name: 'user-key' },
}

const adminMappedLog = {
  ...userMappedLog,
  request_id: 'req-admin-mapped',
  user_id: 7,
  user: { email: 'liu.jialin@code-dance.com' },
  account: { name: 'codex-wang' },
  upstream_reasoning_effort: 'xhigh',
}

const sharedPageStubs = {
  AppLayout: layoutStub,
  Pagination: true,
  Select: true,
  DateRangePicker: true,
  Icon: iconStub,
  UsageStatsCards: chartStub,
  ModelDistributionChart: chartStub,
  GroupDistributionChart: chartStub,
  EndpointDistributionChart: chartStub,
  TokenUsageTrend: chartStub,
  IpGeoCell: true,
  EmptyState: true,
}

function reasoningCellText(wrapper: ReturnType<typeof mount>): string {
  return wrapper.get('[data-testid="reasoning-effort-cell"]').text()
}

describe('usage reasoning effort page display', () => {
  beforeEach(() => {
    localStorage.clear()
    userQuery.mockReset().mockResolvedValue({ items: [userMappedLog], total: 1, pages: 1 })
    userGetStats.mockReset().mockResolvedValue(emptyStats)
    userGetDashboardModels.mockReset().mockResolvedValue({ models: [], start_date: '2026-08-25', end_date: '2026-08-25' })
    userGetDashboardSnapshotV2.mockReset().mockResolvedValue({
      generated_at: '2026-08-25T00:00:00Z',
      start_date: '2026-08-25',
      end_date: '2026-08-25',
      granularity: 'hour',
      trend: [],
      groups: [],
    })
    userKeysList.mockReset().mockResolvedValue({ items: [{ id: 1, name: 'user-key' }] })
    userGroupsGetAvailable.mockReset().mockResolvedValue([{ id: 1, name: 'default' }])

    adminList.mockReset().mockResolvedValue({ items: [adminMappedLog], total: 1, pages: 1 })
    adminGetStats.mockReset().mockResolvedValue(emptyStats)
    adminGetSnapshotV2.mockReset().mockResolvedValue({ trend: [], models: [], groups: [] })
    adminGetModelStats.mockReset().mockResolvedValue({ models: [] })
    adminGetById.mockReset().mockResolvedValue({ id: 7, email: 'liu.jialin@code-dance.com' })
    listErrorLogs.mockReset().mockResolvedValue({ items: [], total: 0 })
  })

  afterEach(() => {
    localStorage.clear()
  })

  it('user usage page only shows the requested reasoning effort', async () => {
    const wrapper = mount(UserUsageView, {
      global: { stubs: sharedPageStubs },
    })
    await flushPromises()

    const cell = reasoningCellText(wrapper)
    expect(cell).toContain('Max')
    expect(cell).not.toContain('XHigh')
    expect(cell).not.toContain('↳')
    expect(wrapper.text()).not.toContain('XHigh')
  })

  it('admin usage page shows requested and mapped effort after the column is enabled', async () => {
    const wrapper = mount(AdminUsageView, {
      global: {
        stubs: {
          ...sharedPageStubs,
          UsageFilters: adminFiltersStub,
          UsageExportProgress: true,
          UsageCleanupDialog: true,
          UserBalanceHistoryModal: true,
          UserTokenRanking: true,
          OpsErrorLogTable: true,
          OpsErrorDetailModal: true,
        },
      },
    })
    await flushPromises()

    expect(wrapper.find('[data-testid="reasoning-effort-cell"]').exists()).toBe(false)

    await wrapper.get('[data-testid="usage-column-settings"]').trigger('click')
    await wrapper.get('[data-testid="usage-column-toggle-reasoning_effort"]').trigger('click')
    await flushPromises()

    const cell = reasoningCellText(wrapper)
    expect(cell).toContain('Max')
    expect(cell).toContain('XHigh')
    expect(cell).toContain('↳')
  })

  it('admin usage page shows a single value when reasoning effort was not mapped', async () => {
    adminList.mockResolvedValue({
      items: [{ ...adminMappedLog, request_id: 'req-admin-plain', reasoning_effort: 'high', upstream_reasoning_effort: null }],
      total: 1,
      pages: 1,
    })

    const wrapper = mount(AdminUsageView, {
      global: {
        stubs: {
          ...sharedPageStubs,
          UsageFilters: adminFiltersStub,
          UsageExportProgress: true,
          UsageCleanupDialog: true,
          UserBalanceHistoryModal: true,
          UserTokenRanking: true,
          OpsErrorLogTable: true,
          OpsErrorDetailModal: true,
        },
      },
    })
    await flushPromises()

    await wrapper.get('[data-testid="usage-column-settings"]').trigger('click')
    await wrapper.get('[data-testid="usage-column-toggle-reasoning_effort"]').trigger('click')
    await flushPromises()

    const cell = reasoningCellText(wrapper)
    expect(cell).toContain('High')
    expect(cell).not.toContain('↳')
  })
})
