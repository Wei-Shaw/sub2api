import { beforeEach, describe, expect, it, vi } from 'vitest'
import { defineComponent } from 'vue'
import { flushPromises, mount } from '@vue/test-utils'
import OpsOpenAIRoutingCard from '../OpsOpenAIRoutingCard.vue'

const mockGetOpenAIRoutingStats = vi.fn()

vi.mock('@/api/admin/ops', () => ({
  opsAPI: {
    getOpenAIRoutingStats: (...args: any[]) => mockGetOpenAIRoutingStats(...args),
  },
}))

vi.mock('vue-i18n', async (importOriginal) => {
  const actual = await importOriginal<typeof import('vue-i18n')>()
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string) => key,
    }),
  }
})

const EmptyStateStub = defineComponent({
  name: 'EmptyState',
  props: {
    title: { type: String, default: '' },
    description: { type: String, default: '' },
  },
  template: '<div class="empty-state">{{ title }}|{{ description }}</div>',
})

const sampleResponse = {
  time_range: '1h' as const,
  start_time: '2026-01-01T00:00:00Z',
  end_time: '2026-01-01T01:00:00Z',
  platform: 'openai',
  group_id: 7,
  request_count_by_group: { active: 10, exhausted: 4 },
  total_tokens_by_group: { active: 1200, exhausted: 320 },
  input_tokens_by_group: { active: 700, exhausted: 200 },
  output_tokens_by_group: { active: 500, exhausted: 120 },
}

describe('OpsOpenAIRoutingCard', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('renders active/exhausted request and token distributions', async () => {
    mockGetOpenAIRoutingStats.mockResolvedValue(sampleResponse)

    const wrapper = mount(OpsOpenAIRoutingCard, {
      props: {
        platformFilter: 'openai',
        groupIdFilter: 7,
        timeRange: '1h',
        refreshToken: 0,
      },
      global: {
        stubs: {
          EmptyState: EmptyStateStub,
        },
      },
    })

    await flushPromises()

    expect(mockGetOpenAIRoutingStats).toHaveBeenCalledWith(
      expect.objectContaining({
        time_range: '1h',
        platform: 'openai',
        group_id: 7,
      })
    )

    expect(wrapper.text()).toContain('admin.ops.openaiRouting.requestCount')
    expect(wrapper.text()).toContain('admin.ops.openaiRouting.totalTokens')
    expect(wrapper.text()).toContain('admin.ops.openaiRouting.inputTokens')
    expect(wrapper.text()).toContain('admin.ops.openaiRouting.outputTokens')
    expect(wrapper.text()).toContain('1,200')
    expect(wrapper.text()).toContain('320')
  })

  it('passes custom time range to API', async () => {
    mockGetOpenAIRoutingStats.mockResolvedValue(sampleResponse)

    mount(OpsOpenAIRoutingCard, {
      props: {
        timeRange: 'custom',
        startTime: '2026-01-01T00:00:00Z',
        endTime: '2026-01-01T02:00:00Z',
        refreshToken: 1,
      },
      global: {
        stubs: {
          EmptyState: EmptyStateStub,
        },
      },
    })

    await flushPromises()

    expect(mockGetOpenAIRoutingStats).toHaveBeenCalledWith(
      expect.objectContaining({
        time_range: 'custom',
        start_time: '2026-01-01T00:00:00Z',
        end_time: '2026-01-01T02:00:00Z',
      })
    )
  })
})
