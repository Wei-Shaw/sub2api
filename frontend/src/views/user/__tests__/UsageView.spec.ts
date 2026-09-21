import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'

import UsageView from '../UsageView.vue'
import UsageTable from '@/components/admin/usage/UsageTable.vue'
import UsageStatsCards from '@/components/admin/usage/UsageStatsCards.vue'

const { query, getStats, showError } = vi.hoisted(() => ({
  query: vi.fn(),
  getStats: vi.fn(),
  showError: vi.fn(),
}))

vi.mock('@/api', () => ({
  usageAPI: { query, getStats },
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({ showError }),
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({ t: (key: string) => key }),
  }
})

const usageLog = {
  id: 1,
  request_id: 'req-r4',
  actual_cost: 0.08,
  total_cost: 0.1,
  rate_multiplier: 1,
  input_cost: 0.02,
  output_cost: 0.03,
  cache_creation_cost: 0,
  cache_read_cost: 0,
  input_tokens: 10,
  output_tokens: 20,
  cache_creation_tokens: 0,
  cache_read_tokens: 0,
  cache_creation_5m_tokens: 0,
  cache_creation_1h_tokens: 0,
  image_count: 0,
  image_size: null,
  first_token_ms: 12,
  duration_ms: 345,
  created_at: '2026-09-21T00:00:00Z',
  model: 'gpt-5.4',
  reasoning_effort: null,
  ip_address: '203.0.113.10',
  api_key: { name: 'demo-key' },
  group: { name: 'default' },
  billing_mode: 'token',
  request_type: 'sync',
  stream: false,
  native_compaction_v2: false,
}

const stats = {
  total_requests: 1,
  total_input_tokens: 10,
  total_output_tokens: 20,
  total_cache_tokens: 0,
  total_cache_read_tokens: 0,
  total_cache_creation_tokens: 0,
  total_tokens: 30,
  total_cost: 0.1,
  total_actual_cost: 0.08,
  average_duration_ms: 345,
  average_latency_ms: 12,
}

const mountUsageView = () => mount(UsageView, {
  global: {
    stubs: {
      AppLayout: { template: '<div><slot /></div>' },
      Pagination: true,
      DateRangePicker: true,
      UsageStatsCards: true,
      UsageTable: true,
    },
  },
})

describe('user UsageView R4', () => {
  beforeEach(() => {
    query.mockReset().mockResolvedValue({ items: [usageLog], total: 1, pages: 1 })
    getStats.mockReset().mockResolvedValue(stats)
    showError.mockReset()
  })

  it('loads the ten-row usage list and summary for the selected date range', async () => {
    mountUsageView()
    await flushPromises()

    expect(query).toHaveBeenCalledWith(expect.objectContaining({
      page: 1,
      page_size: 10,
      sort_by: 'created_at',
      sort_order: 'desc',
    }), expect.anything())
    expect(getStats).toHaveBeenCalledWith(expect.objectContaining({
      start_date: expect.any(String),
      end_date: expect.any(String),
    }))
  })

  it('shows only the nine R4 columns and user-facing display modes', async () => {
    const wrapper = mountUsageView()
    await flushPromises()

    const table = wrapper.findComponent(UsageTable)
    expect(table.props('columns').map((column: { key: string }) => column.key)).toEqual([
      'created_at',
      'api_key',
      'group',
      'model',
      'latency',
      'input_tokens',
      'output_tokens',
      'cost',
      'ip_address',
    ])
    expect(table.props('currencySymbol')).toBe('¥')
    expect(table.props('latencyDisplay')).toBe('preferred')
    expect(table.props('showCostDetails')).toBe(false)

    const cards = wrapper.findComponent(UsageStatsCards)
    expect(cards.props('currencySymbol')).toBe('¥')
    expect(cards.props('showCostBreakdown')).toBe(false)
  })

  it('reloads both list and summary when the date range changes', async () => {
    const wrapper = mountUsageView()
    await flushPromises()
    query.mockClear()
    getStats.mockClear()

    ;(wrapper.vm as any).onDateRangeChange({
      startDate: '2026-09-01',
      endDate: '2026-09-20',
      preset: null,
    })
    await flushPromises()

    expect(query).toHaveBeenCalledWith(expect.objectContaining({
      page: 1,
      page_size: 10,
      start_date: '2026-09-01',
      end_date: '2026-09-20',
    }), expect.anything())
    expect(getStats).toHaveBeenCalledWith({
      start_date: '2026-09-01',
      end_date: '2026-09-20',
    })
  })
})
