import { beforeEach, describe, expect, it, vi } from 'vitest'
import { defineComponent } from 'vue'
import { flushPromises, mount } from '@vue/test-utils'
import OpsUserUsageStatsCard from '../OpsUserUsageStatsCard.vue'

const mockGetUserUsageStats = vi.fn()
const mockPush = vi.fn()

vi.mock('@/api/admin/ops', () => ({
  opsAPI: {
    getUserUsageStats: (...args: any[]) => mockGetUserUsageStats(...args)
  }
}))

vi.mock('vue-router', () => ({
  useRouter: () => ({ push: mockPush })
}))

vi.mock('vue-i18n', async (importOriginal) => {
  const actual = await importOriginal<typeof import('vue-i18n')>()
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string, params?: Record<string, any>) => {
        if (key === 'admin.ops.userUsageStats.pageInfo' && params) {
          return `第 ${params.page}/${params.total} 页`
        }
        return key
      }
    })
  }
})

const SelectStub = defineComponent({
  name: 'SelectControlStub',
  props: {
    modelValue: { type: [String, Number], default: '' }
  },
  emits: ['update:modelValue'],
  template: '<div class="select-stub" />'
})

const EmptyStateStub = defineComponent({
  name: 'EmptyState',
  props: {
    title: { type: String, default: '' },
    description: { type: String, default: '' }
  },
  template: '<div class="empty-state">{{ title }}|{{ description }}</div>'
})

const sampleResponse = {
  time_range: '24h' as const,
  start_time: '2026-07-14T00:00:00Z',
  end_time: '2026-07-15T00:00:00Z',
  items: [
    {
      user_id: 7,
      username: 'alice',
      email: 'alice@example.com',
      request_count: 12,
      input_tokens: 1000,
      output_tokens: 300,
      cache_tokens: 200,
      total_tokens: 1500,
      actual_cost: 1.25,
      last_request_at: '2026-07-14T23:59:00Z'
    }
  ],
  total: 32,
  top_n: 20
}

describe('OpsUserUsageStatsCard', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockGetUserUsageStats.mockResolvedValue(sampleResponse)
  })

  it('默认查询近 24 小时 Top 20，并透传平台与分组', async () => {
    mount(OpsUserUsageStatsCard, {
      props: { platformFilter: 'openai', groupIdFilter: 9, refreshToken: 0 },
      global: { stubs: { Select: SelectStub, EmptyState: EmptyStateStub } }
    })
    await flushPromises()

    expect(mockGetUserUsageStats).toHaveBeenCalledWith({
      time_range: '24h',
      platform: 'openai',
      group_id: 9,
      top_n: 20
    })
  })

  it('支持切换到分页查询', async () => {
    mockGetUserUsageStats.mockImplementation(async (params: Record<string, any>) => ({
      ...sampleResponse,
      page: params.page ?? 1,
      page_size: params.page_size ?? 20,
      top_n: params.top_n ?? null,
      total: 32
    }))
    const wrapper = mount(OpsUserUsageStatsCard, {
      props: { refreshToken: 0 },
      global: { stubs: { Select: SelectStub, EmptyState: EmptyStateStub } }
    })
    await flushPromises()

    const selects = wrapper.findAllComponents(SelectStub)
    await selects[1].vm.$emit('update:modelValue', 'pagination')
    await flushPromises()

    expect(mockGetUserUsageStats).toHaveBeenCalledWith(expect.objectContaining({ page: 1, page_size: 20 }))
    const buttons = wrapper.findAll('button')
    await buttons.find((button) => button.text() === 'admin.ops.userUsageStats.nextPage')!.trigger('click')
    await flushPromises()
    expect(mockGetUserUsageStats).toHaveBeenCalledWith(expect.objectContaining({ page: 2, page_size: 20 }))
  })

  it('点击用户跳转到该用户的用量明细', async () => {
    const wrapper = mount(OpsUserUsageStatsCard, {
      props: { refreshToken: 0 },
      global: { stubs: { Select: SelectStub, EmptyState: EmptyStateStub } }
    })
    await flushPromises()

    await wrapper.get('tbody button').trigger('click')
    expect(mockPush).toHaveBeenCalledWith({ path: '/admin/usage', query: { user_id: '7' } })
  })

  it('空数据与接口错误都有明确状态', async () => {
    mockGetUserUsageStats.mockResolvedValueOnce({ ...sampleResponse, items: [], total: 0 })
    const emptyWrapper = mount(OpsUserUsageStatsCard, {
      props: { refreshToken: 0 },
      global: { stubs: { Select: SelectStub, EmptyState: EmptyStateStub } }
    })
    await flushPromises()
    expect(emptyWrapper.find('.empty-state').exists()).toBe(true)

    mockGetUserUsageStats.mockRejectedValueOnce(new Error('加载失败'))
    const errorWrapper = mount(OpsUserUsageStatsCard, {
      props: { refreshToken: 1 },
      global: { stubs: { Select: SelectStub, EmptyState: EmptyStateStub } }
    })
    await flushPromises()
    expect(errorWrapper.text()).toContain('加载失败')
  })
})
