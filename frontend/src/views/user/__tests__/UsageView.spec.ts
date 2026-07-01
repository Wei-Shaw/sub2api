import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { nextTick } from 'vue'

import UsageView from '../UsageView.vue'

const {
  query,
  getStats,
  getDashboardModels,
  getDashboardSnapshotV2,
  list,
  getAvailable,
  showError,
  showWarning,
  showSuccess,
  showInfo,
} = vi.hoisted(() => ({
  query: vi.fn(),
  getStats: vi.fn(),
  getDashboardModels: vi.fn(),
  getDashboardSnapshotV2: vi.fn(),
  list: vi.fn(),
  getAvailable: vi.fn(),
  showError: vi.fn(),
  showWarning: vi.fn(),
  showSuccess: vi.fn(),
  showInfo: vi.fn(),
}))

const messages: Record<string, string> = {
  'admin.dashboard.timeRange': 'Time range',
  'admin.dashboard.granularity': 'Granularity',
  'admin.dashboard.day': 'Day',
  'admin.dashboard.hour': 'Hour',
  'admin.users.columnSettings': 'Columns',
  'admin.usage.group': 'Group',
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
  'admin.usage.inputCost': 'Input Cost',
  'admin.usage.outputCost': 'Output Cost',
  'admin.usage.cacheCreationCost': 'Cache Creation Cost',
  'admin.usage.cacheReadCost': 'Cache Read Cost',
  'usage.allApiKeys': 'All API Keys',
  'usage.apiKeyFilter': 'API Key',
  'usage.model': 'Model',
  'usage.type': 'Type',
  'usage.ws': 'WS',
  'usage.stream': 'Stream',
  'usage.sync': 'Sync',
  'usage.exporting': 'Exporting',
  'usage.exportCsv': 'Export CSV',
  'usage.failedToLoad': 'Failed to load',
  'usage.noDataToExport': 'No data',
  'usage.preparingExport': 'Preparing export',
  'usage.exportSuccess': 'Export success',
  'usage.exportFailed': 'Export failed',
  'usage.tabs.usage': 'Usage',
  'usage.tabs.errors': 'Errors',
  'usage.endpointDistribution': 'Endpoint Distribution',
  'usage.totalRequests': 'Total requests',
  'usage.inSelectedRange': 'in selected range',
  'usage.totalTokens': 'Total tokens',
  'usage.in': 'Input',
  'usage.out': 'Output',
  'usage.cacheTotal': 'Cache',
  'usage.cacheBreakdown': 'Cache Token Breakdown',
  'usage.cacheCreationTokensLabel': 'Cache Creation',
  'usage.cacheReadTokensLabel': 'Cache Read',
  'usage.totalCost': 'Total cost',
  'usage.standardCost': 'standard',
  'usage.avgDuration': 'Average duration',
  'usage.costDetails': 'Cost Breakdown',
  'usage.inputTokenPrice': 'Input price',
  'usage.outputTokenPrice': 'Output price',
  'usage.imageOutputCost': 'Image Output Cost',
  'usage.imageOutputTokens': 'Image Output Tokens',
  'usage.perMillionTokens': '/ 1M tokens',
  'usage.serviceTier': 'Service tier',
  'usage.serviceTierPriority': 'Fast',
  'usage.serviceTierFlex': 'Flex',
  'usage.serviceTierStandard': 'Standard',
  'usage.rate': 'Rate',
  'usage.original': 'Original',
  'usage.billed': 'Billed',
  'usage.unitPrice': 'Unit price',
  'usage.imageUnit': ' images',
  'usage.imageCount': 'Image count',
  'usage.imageBillingSize': 'Billing size',
  'usage.imageInputSize': 'Input size',
  'usage.imageOutputSize': 'Output size',
  'usage.imageSizeSource': 'Size source',
  'usage.imageSizeBreakdown': 'Size breakdown',
  'usage.imageSizeSourceOutput': 'Upstream output',
  'usage.imageSizeSourceInput': 'Request input',
  'usage.imageSizeSourceDefault': 'Default billing tier',
  'usage.imageSizeSourceLegacy': 'Legacy record',
  'usage.imageSizeSourceMissing': 'Not recorded',
  'usage.imageSizeNotRecorded': 'not recorded',
  'usage.imageSizeLegacyUnstandardized': 'legacy unstandardized',
  'usage.imageSizeUnknown': 'unknown',
  'usage.imageUnitPrice': 'Per-image price',
  'usage.imageTotalPrice': 'Image total price',
  'usage.cacheHit': 'Cache hit',
  'usage.cacheCreate': 'Cache write',
  'usage.cacheHitRate': 'Cache hit rate',
  'usage.reasoningEffort': 'Reasoning Effort',
  'usage.cost': 'Cost',
  'usage.firstToken': 'First Token',
  'usage.duration': 'Duration',
  'usage.time': 'Time',
  'usage.userAgent': 'User Agent',
  'usage.perRequest': 'per request',
  'usage.accountBilled': 'Account billed',
  'usage.userBilled': 'User billed',
}

vi.mock('@/api', () => ({
  usageAPI: {
    query,
    getStats,
    getDashboardModels,
    getDashboardSnapshotV2,
  },
  keysAPI: {
    list,
  },
  userGroupsAPI: {
    getAvailable,
  },
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({ showError, showWarning, showSuccess, showInfo }),
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

const simpleStub = { template: '<div><slot /></div>' }
const chartStub = { template: '<div />' }
const dataTableStub = {
  props: ['data'],
  template: `
    <div>
      <div v-for="row in data" :key="row.request_id || row.id">
        <slot name="cell-billing_mode" :row="row" />
        <slot name="cell-tokens" :row="row" />
        <slot name="cell-cost" :row="row" />
      </div>
      <slot v-if="data.length === 0" name="empty" />
    </div>
  `,
}

const usageLog = {
  id: 1,
  request_id: 'req-user-export',
  actual_cost: 0.092883,
  total_cost: 0.092883,
  total_cache_tokens: 278276,
  total_cache_creation_tokens: 4,
  total_cache_read_tokens: 278272,
  rate_multiplier: 1,
  service_tier: 'priority',
  input_cost: 0.020285,
  output_cost: 0.00303,
  cache_creation_cost: 0.000001,
  cache_read_cost: 0.069568,
  input_tokens: 4057,
  output_tokens: 101,
  cache_creation_tokens: 4,
  cache_read_tokens: 278272,
  cache_creation_5m_tokens: 0,
  cache_creation_1h_tokens: 0,
  image_count: 0,
  image_size: null,
  first_token_ms: 12,
  duration_ms: 345,
  created_at: '2026-03-08T00:00:00Z',
  model: 'gpt-5.4',
  reasoning_effort: null,
  ip_address: '203.0.113.10',
  api_key: { name: 'demo-key' },
  billing_mode: 'token',
  request_type: 'sync',
  stream: false,
}

function mountUsageView() {
  return mount(UsageView, {
    global: {
      stubs: {
        AppLayout: simpleStub,
        Pagination: true,
        Select: true,
        DateRangePicker: true,
        Icon: true,
        DataTable: dataTableStub,
        EmptyState: true,
        ModelDistributionChart: chartStub,
        GroupDistributionChart: chartStub,
        EndpointDistributionChart: chartStub,
        TokenUsageTrend: chartStub,
        UserErrorRequestsTable: chartStub,
        Teleport: true,
      },
    },
  })
}

describe('user UsageView', () => {
  beforeEach(() => {
    query.mockReset()
    getStats.mockReset()
    getDashboardModels.mockReset()
    getDashboardSnapshotV2.mockReset()
    list.mockReset()
    getAvailable.mockReset()
    showError.mockReset()
    showWarning.mockReset()
    showSuccess.mockReset()
    showInfo.mockReset()

    vi.spyOn(HTMLElement.prototype, 'getBoundingClientRect').mockReturnValue({
      x: 0,
      y: 0,
      top: 20,
      left: 20,
      right: 120,
      bottom: 40,
      width: 100,
      height: 20,
      toJSON: () => ({}),
    } as DOMRect)

    query.mockResolvedValue({ items: [usageLog], total: 1, pages: 1 })
    getStats.mockResolvedValue({
      total_requests: 1,
      total_input_tokens: 10,
      total_output_tokens: 20,
      total_cache_tokens: 0,
      total_cache_creation_tokens: 0,
      total_cache_read_tokens: 0,
      total_tokens: 30,
      total_cost: 0.1,
      total_actual_cost: 0.08,
      average_duration_ms: 12,
      endpoints: [],
      upstream_endpoints: [],
      endpoint_paths: [],
    })
    getDashboardModels.mockResolvedValue({
      models: [{ model: 'gpt-5.4', requests: 1, input_tokens: 10, output_tokens: 20, cache_creation_tokens: 0, cache_read_tokens: 0, total_tokens: 30, cost: 0.1, actual_cost: 0.08 }],
      start_date: '2026-03-08',
      end_date: '2026-03-08',
    })
    getDashboardSnapshotV2.mockResolvedValue({
      generated_at: '2026-03-08T00:00:00Z',
      start_date: '2026-03-08',
      end_date: '2026-03-08',
      granularity: 'hour',
      trend: [],
      groups: [],
    })
    list.mockResolvedValue({ items: [{ id: 1, name: 'demo-key' }] })
    getAvailable.mockResolvedValue([{ id: 1, name: 'default' }])
  })

  it('loads logs, stats, model stats, and snapshot on first render', async () => {
    mountUsageView()
    await flushPromises()

    expect(query).toHaveBeenCalled()
    expect(getStats).toHaveBeenCalled()
    expect(getDashboardModels).toHaveBeenCalled()
    expect(getDashboardSnapshotV2).toHaveBeenCalledWith(expect.objectContaining({
      include_trend: true,
      include_model_stats: false,
      include_group_stats: true,
    }))
    expect(list).toHaveBeenCalledWith(1, 100)
    expect(getAvailable).toHaveBeenCalled()
  })

  it('renders split cache stats without raw i18n keys', async () => {
    query.mockResolvedValue({ items: [], total: 0, pages: 0 })
    getStats.mockResolvedValue({
      total_requests: 1,
      total_input_tokens: 4057,
      total_output_tokens: 101,
      total_cache_tokens: 278276,
      total_cache_creation_tokens: 4,
      total_cache_read_tokens: 278272,
      total_tokens: 282434,
      total_cost: 0.1,
      total_actual_cost: 0.1,
      average_duration_ms: 1,
      endpoints: [],
      upstream_endpoints: [],
      endpoint_paths: [],
    })
    list.mockResolvedValue({ items: [] })

    const wrapper = mountUsageView()
    await flushPromises()
    await nextTick()

    const text = wrapper.text().replace(/\s+/g, ' ')
    expect(text).toContain('Total tokens')
    expect(text).toContain('282.43K')
    expect(text).toContain('Cache: 278.28K')
    expect(text).toContain('Cache Token Breakdown')
    expect(text).toMatch(/Cache Creation\s*4/)
    expect(text).toMatch(/Cache Read\s*278\.27K/)
    expect(text).not.toContain('usage.cacheTotal')
    expect(text).not.toContain('usage.cacheBreakdown')
    expect(text).not.toContain('usage.cacheCreationTokensLabel')
    expect(text).not.toContain('usage.cacheReadTokensLabel')
  })


  it('shows fast service tier and unit prices in user tooltip', async () => {
    query.mockResolvedValue({
      items: [
        {
          request_id: 'req-user-1',
          actual_cost: 0.092883,
          total_cost: 0.092883,
          rate_multiplier: 1,
          service_tier: 'priority',
          input_cost: 0.020285,
          output_cost: 0.00303,
          cache_creation_cost: 0,
          cache_read_cost: 0.069568,
          input_tokens: 4057,
          output_tokens: 101,
          cache_creation_tokens: 0,
          cache_read_tokens: 278272,
          cache_creation_5m_tokens: 0,
          cache_creation_1h_tokens: 0,
          image_count: 0,
          image_size: null,
          first_token_ms: null,
          duration_ms: 1,
          created_at: '2026-03-08T00:00:00Z',
        },
      ],
      total: 1,
      pages: 1,
    })
    getStats.mockResolvedValue({
      total_requests: 1,
      total_input_tokens: 10,
      total_output_tokens: 20,
      total_cache_tokens: 0,
      total_cache_creation_tokens: 0,
      total_cache_read_tokens: 0,
      total_tokens: 30,
      total_cost: 0.1,
      total_actual_cost: 0.08,
      average_duration_ms: 12,
      endpoints: [],
      upstream_endpoints: [],
      endpoint_paths: [],
    })
    getDashboardModels.mockResolvedValue({
      models: [{ model: 'gpt-5.4', requests: 1, input_tokens: 10, output_tokens: 20, cache_creation_tokens: 0, cache_read_tokens: 0, total_tokens: 30, cost: 0.1, actual_cost: 0.08 }],
      start_date: '2026-03-08',
      end_date: '2026-03-08',
    })
    getDashboardSnapshotV2.mockResolvedValue({
      generated_at: '2026-03-08T00:00:00Z',
      start_date: '2026-03-08',
      end_date: '2026-03-08',
      granularity: 'hour',
      trend: [],
      groups: [],
    })
    list.mockResolvedValue({ items: [{ id: 1, name: 'demo-key' }] })
    getAvailable.mockResolvedValue([{ id: 1, name: 'default' }])

    const wrapper = mountUsageView()
    await flushPromises()
    await nextTick()

    const tooltipTriggers = wrapper.findAll('.group.relative')
    expect(tooltipTriggers.length).toBeGreaterThanOrEqual(2)
    await tooltipTriggers[tooltipTriggers.length - 1].trigger('mouseenter')
    await nextTick()

    const text = wrapper.text().replace(/\s+/g, ' ')
    expect(text).toContain('Service tier')
    expect(text).toContain('Fast')
    expect(text).toContain('Rate')
    expect(text).toContain('1.00x')
    expect(text).toContain('User billed')
    expect(text).toContain('$0.092883')
    expect(text).toContain('$5.0000 / 1M tokens')
    expect(text).toContain('$30.0000 / 1M tokens')
  })

  it('exports csv with current filters and without admin-only fields', async () => {
    const wrapper = mountUsageView()
    await flushPromises()

    let exportedBlob: Blob | null = null
    let csvContent = ''
    const OriginalBlob = globalThis.Blob
    vi.stubGlobal('Blob', vi.fn((parts: BlobPart[], options?: BlobPropertyBag) => {
      csvContent = parts.map((part) => String(part)).join('')
      return new OriginalBlob(parts, options)
    }))
    const originalCreateObjectURL = window.URL.createObjectURL
    const originalRevokeObjectURL = window.URL.revokeObjectURL
    window.URL.createObjectURL = vi.fn((blob: Blob | MediaSource) => {
      exportedBlob = blob as Blob
      return 'blob:usage-export'
    }) as typeof window.URL.createObjectURL
    window.URL.revokeObjectURL = vi.fn(() => {}) as typeof window.URL.revokeObjectURL
    const clickSpy = vi.spyOn(HTMLAnchorElement.prototype, 'click').mockImplementation(() => {})

    await (wrapper.vm as any).exportToCSV()

    expect(exportedBlob).not.toBeNull()
    expect(query).toHaveBeenCalledWith(expect.objectContaining({
      page_size: 100,
      sort_by: 'created_at',
      sort_order: 'desc',
    }))
    expect(clickSpy).toHaveBeenCalled()
    expect(showSuccess).toHaveBeenCalled()
    expect(csvContent.startsWith('\uFEFF')).toBe(true)
    expect(csvContent.slice(1)).toBe([
      'Time,API Key Name,Model,Reasoning Effort,Inbound Endpoint,IP Address,Type,Billing Mode,Input Tokens,Output Tokens,Cache Read Tokens,Cache Creation Tokens,Rate Multiplier,Billed Cost,Original Cost,First Token (ms),Duration (ms)',
      '2026-03-08T00:00:00Z,demo-key,gpt-5.4,"\'-",,203.0.113.10,Sync,Token,4057,101,278272,4,1,0.09288300,0.09288300,12,345',
    ].join('\n'))
    expect(csvContent).toContain('IP Address')
    expect(csvContent).toContain('203.0.113.10')
    expect(csvContent).toContain('Billed Cost')
    expect(csvContent).toContain('Original Cost')
    expect(csvContent).not.toContain('Upstream Endpoint')
    expect(csvContent).not.toContain('account_cost')
    expect(csvContent).not.toContain('account_rate_multiplier')

    window.URL.createObjectURL = originalCreateObjectURL
    window.URL.revokeObjectURL = originalRevokeObjectURL
    vi.unstubAllGlobals()
    clickSpy.mockRestore()
  })

  it('exports historical image rows with image billing mode derived from image_count', async () => {
    query.mockResolvedValue({
      items: [
        {
          ...usageLog,
          request_id: 'req-user-export-legacy-image',
          actual_cost: 0.2,
          total_cost: 0.2,
          input_cost: 0,
          output_cost: 0,
          cache_creation_cost: 0,
          cache_read_cost: 0,
          input_tokens: 0,
          output_tokens: 0,
          cache_creation_tokens: 0,
          cache_read_tokens: 0,
          image_count: 1,
          model: 'gpt-image-2',
          billing_mode: null,
          ip_address: null,
        },
      ],
      total: 1,
      pages: 1,
    })

    const wrapper = mountUsageView()
    await flushPromises()

    let csvContent = ''
    const OriginalBlob = globalThis.Blob
    vi.stubGlobal('Blob', vi.fn((parts: BlobPart[], options?: BlobPropertyBag) => {
      csvContent = parts.map((part) => String(part)).join('')
      return new OriginalBlob(parts, options)
    }))
    const originalCreateObjectURL = window.URL.createObjectURL
    const originalRevokeObjectURL = window.URL.revokeObjectURL
    window.URL.createObjectURL = vi.fn(() => 'blob:usage-export') as typeof window.URL.createObjectURL
    window.URL.revokeObjectURL = vi.fn(() => {}) as typeof window.URL.revokeObjectURL
    const clickSpy = vi.spyOn(HTMLAnchorElement.prototype, 'click').mockImplementation(() => {})

    await (wrapper.vm as any).exportToCSV()

    expect(csvContent).toContain('Billing Mode')
    expect(csvContent).toContain('Image')
    expect(csvContent).not.toContain(',Token,0,0,0,0,')

    window.URL.createObjectURL = originalCreateObjectURL
    window.URL.revokeObjectURL = originalRevokeObjectURL
    vi.unstubAllGlobals()
    clickSpy.mockRestore()
  })
})
