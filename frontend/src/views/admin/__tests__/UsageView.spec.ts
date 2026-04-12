import { describe, expect, it, vi, beforeEach, afterEach } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'

import UsageView from '../UsageView.vue'

const { list, getStats, getSnapshotV2, getById, exportList, saveAsMock } = vi.hoisted(() => {
  vi.stubGlobal('localStorage', {
    getItem: vi.fn(() => null),
    setItem: vi.fn(),
    removeItem: vi.fn(),
  })

    return {
      list: vi.fn(),
      exportList: vi.fn(),
      getStats: vi.fn(),
      getSnapshotV2: vi.fn(),
      getById: vi.fn(),
      saveAsMock: vi.fn(),
    }
})

const messages: Record<string, string> = {
  'admin.dashboard.timeRange': 'Time Range',
  'admin.dashboard.day': 'Day',
  'admin.dashboard.hour': 'Hour',
  'admin.usage.failedToLoadUser': 'Failed to load user',
  'usage.grossInputTokens': 'Total Input Tokens',
  'usage.netInputTokens': 'Net Input Tokens',
  'usage.totalTokens': 'Total Tokens',
}

const formatLocalDate = (date: Date): string => {
  const year = date.getFullYear()
  const month = String(date.getMonth() + 1).padStart(2, '0')
  const day = String(date.getDate()).padStart(2, '0')
  return `${year}-${month}-${day}`
}

vi.mock('@/api/admin', () => ({
  adminAPI: {
    usage: {
      list,
      getStats,
    },
    dashboard: {
      getSnapshotV2,
    },
    users: {
      getById,
    },
  },
}))

vi.mock('@/api/admin/usage', () => ({
  adminUsageAPI: {
    list: exportList,
  },
}))

vi.mock('file-saver', () => ({ saveAs: saveAsMock }))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({
    showError: vi.fn(),
    showWarning: vi.fn(),
    showSuccess: vi.fn(),
    showInfo: vi.fn(),
  }),
}))

vi.mock('@/utils/format', () => ({
  formatReasoningEffort: (value: string | null | undefined) => value ?? '-',
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
  useRoute: () => ({
    query: {}
  })
}))

const AppLayoutStub = { template: '<div><slot /></div>' }
const UsageFiltersStub = { template: '<div><slot name="after-reset" /></div>' }
const ModelDistributionChartStub = {
  props: ['metric'],
  emits: ['update:metric'],
  template: `
    <div data-test="model-chart">
      <span class="metric">{{ metric }}</span>
      <button class="switch-metric" @click="$emit('update:metric', 'actual_cost')">switch</button>
    </div>
  `,
}
const GroupDistributionChartStub = {
  props: ['metric'],
  emits: ['update:metric'],
  template: `
    <div data-test="group-chart">
      <span class="metric">{{ metric }}</span>
      <button class="switch-metric" @click="$emit('update:metric', 'actual_cost')">switch</button>
    </div>
  `,
}

describe('admin UsageView distribution metric toggles', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    list.mockReset()
    exportList.mockReset()
    getStats.mockReset()
    getSnapshotV2.mockReset()
    getById.mockReset()
    saveAsMock.mockReset()

    list.mockResolvedValue({
      items: [],
      total: 0,
      pages: 0,
    })
    exportList.mockResolvedValue({
      items: [],
      total: 0,
      pages: 0,
    })
    getStats.mockResolvedValue({
      total_requests: 0,
      total_input_tokens: 0,
      total_output_tokens: 0,
      total_cache_tokens: 0,
      total_tokens: 0,
      total_cost: 0,
      total_actual_cost: 0,
      average_duration_ms: 0,
    })
    getSnapshotV2.mockResolvedValue({
      trend: [],
      models: [],
      groups: [],
    })
  })

  afterEach(() => {
    vi.useRealTimers()
  })

  it('keeps model and group metric toggles independent without refetching chart data', async () => {
    const wrapper = mount(UsageView, {
      global: {
        stubs: {
          AppLayout: AppLayoutStub,
          UsageStatsCards: true,
          UsageFilters: UsageFiltersStub,
          UsageTable: true,
          UsageExportProgress: true,
          UsageCleanupDialog: true,
          UserBalanceHistoryModal: true,
          Pagination: true,
          Select: true,
          DateRangePicker: true,
          Icon: true,
          TokenUsageTrend: true,
          ModelDistributionChart: ModelDistributionChartStub,
          GroupDistributionChart: GroupDistributionChartStub,
        },
      },
    })

    vi.advanceTimersByTime(120)
    await flushPromises()

    expect(getSnapshotV2).toHaveBeenCalledTimes(1)
    const now = new Date()
    const yesterday = new Date(now.getTime() - 24 * 60 * 60 * 1000)
    expect(getSnapshotV2).toHaveBeenCalledWith(expect.objectContaining({
      start_date: formatLocalDate(yesterday),
      end_date: formatLocalDate(now),
      granularity: 'hour'
    }))

    const modelChart = wrapper.find('[data-test="model-chart"]')
    const groupChart = wrapper.find('[data-test="group-chart"]')

    expect(modelChart.find('.metric').text()).toBe('tokens')
    expect(groupChart.find('.metric').text()).toBe('tokens')

    await modelChart.find('.switch-metric').trigger('click')
    await flushPromises()

    expect(modelChart.find('.metric').text()).toBe('actual_cost')
    expect(groupChart.find('.metric').text()).toBe('tokens')
    expect(getSnapshotV2).toHaveBeenCalledTimes(1)

    await groupChart.find('.switch-metric').trigger('click')
    await flushPromises()

    expect(modelChart.find('.metric').text()).toBe('actual_cost')
    expect(groupChart.find('.metric').text()).toBe('actual_cost')
    expect(getSnapshotV2).toHaveBeenCalledTimes(1)
  })

  it('exports admin usage with total input, net input and total tokens', async () => {
    const exportedLogs = [
      {
        request_id: 'req-admin-export',
        created_at: '2026-03-08T00:00:00Z',
        user: { email: 'demo@example.com' },
        api_key: { name: 'demo-key' },
        account: { name: 'demo-account' },
        model: 'gpt-5.4',
        upstream_model: '',
        reasoning_effort: null,
        group: { name: 'default' },
        inbound_endpoint: '',
        upstream_endpoint: '',
        routing_target_group: '',
        routing_schedule_layer: '',
        routing_selected_account_name: '',
        routing_effective_model: '',
        routing_failover_count: null,
        routing_failover_final_reason: '',
        billing_mode: 'token',
        input_tokens: 100,
        output_tokens: 20,
        cache_read_tokens: 30,
        cache_creation_tokens: 10,
        input_cost: 0,
        output_cost: 0,
        cache_read_cost: 0,
        cache_creation_cost: 0,
        rate_multiplier: 1,
        account_rate_multiplier: 1,
        total_cost: 0,
        actual_cost: 0,
        first_token_ms: 5,
        duration_ms: 100,
        user_agent: '',
        ip_address: '',
      },
    ]

    list.mockResolvedValue({ items: exportedLogs, total: 1, pages: 1 })
    exportList.mockResolvedValue({ items: exportedLogs, total: 1, pages: 1 })

    const aoaToSheetSpy = vi.fn(() => ({}))
    const sheetAddSpy = vi.fn()
    vi.doMock('xlsx', () => ({
      utils: {
        aoa_to_sheet: aoaToSheetSpy,
        sheet_add_aoa: sheetAddSpy,
        book_new: vi.fn(() => ({})),
        book_append_sheet: vi.fn(),
      },
      write: vi.fn(() => new ArrayBuffer(8)),
    }))

    const wrapper = mount(UsageView, {
      global: {
        stubs: {
          AppLayout: AppLayoutStub,
          UsageStatsCards: true,
          UsageFilters: UsageFiltersStub,
          UsageTable: true,
          UsageExportProgress: true,
          UsageCleanupDialog: true,
          UserBalanceHistoryModal: true,
          Pagination: true,
          Select: true,
          DateRangePicker: true,
          Icon: true,
          TokenUsageTrend: true,
          ModelDistributionChart: true,
          GroupDistributionChart: true,
          EndpointDistributionChart: true,
        },
      },
    })

    await flushPromises()
    const setupState = (wrapper.vm as any).$?.setupState
    await setupState.exportToExcel()

    expect(exportList).toHaveBeenCalled()
    const headerRow = aoaToSheetSpy.mock.calls[0]?.[0]?.[0] || []
    const dataRow = sheetAddSpy.mock.calls[0]?.[1]?.[0] || []
    expect(headerRow).toContain('Total Input Tokens')
    expect(headerRow).toContain('Net Input Tokens')
    expect(headerRow).toContain('Total Tokens')
    expect(dataRow).toContain(140)
    expect(dataRow).toContain(100)
    expect(dataRow).toContain(160)
    expect(saveAsMock).toHaveBeenCalled()
  })
})
