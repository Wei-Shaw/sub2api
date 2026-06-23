import { describe, expect, it, vi, beforeEach } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'

import AvailableChannelsView from '../AvailableChannelsView.vue'

const { getAvailableChannels, getChannelPrices, getAvailableGroups, getUserGroupRates, showError } = vi.hoisted(() => ({
  getAvailableChannels: vi.fn(),
  getChannelPrices: vi.fn(),
  getAvailableGroups: vi.fn(),
  getUserGroupRates: vi.fn(),
  showError: vi.fn(),
}))

const messages: Record<string, string> = {
  'availableChannels.searchPlaceholder': '搜索渠道或模型...',
  'availableChannels.noPricing': '未配置定价',
  'availableChannels.noModels': '未配置模型',
  'availableChannels.empty': '暂无可用渠道',
  'availableChannels.columns.name': '渠道名',
  'availableChannels.columns.description': '描述',
  'availableChannels.columns.platform': '平台',
  'availableChannels.columns.groups': '我可访问的分组',
  'availableChannels.columns.supportedModels': '支持模型',
  'availableChannels.priceSummary.title': '当前价格',
  'availableChannels.priceSummary.subtitle': '按当前账号可见渠道与分组计算',
  'availableChannels.priceSummary.input': '输入',
  'availableChannels.priceSummary.output': '输出',
  'availableChannels.priceSummary.cacheWrite': '缓存写入',
  'availableChannels.priceSummary.cacheRead': '缓存读取',
  'availableChannels.priceSummary.priorityInput': '优先输入',
  'availableChannels.priceSummary.priorityOutput': '优先输出',
  'availableChannels.priceSummary.priorityCacheRead': '优先缓存读取',
  'availableChannels.priceSummary.imageTitle': '生图',
  'availableChannels.priceSummary.imageOutput': '图片输出',
  'availableChannels.priceSummary.image1k': '1K',
  'availableChannels.priceSummary.image2k': '2K',
  'availableChannels.priceSummary.image4k': '4K',
  'common.refresh': '刷新',
}

vi.mock('@/api/channels', () => ({
  default: {
    getAvailable: getAvailableChannels,
    getPrices: getChannelPrices,
  },
}))

vi.mock('@/api/groups', () => ({
  default: {
    getAvailable: getAvailableGroups,
    getUserGroupRates,
  },
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({ showError }),
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

const AppLayoutStub = { template: '<div><slot /></div>' }
const TablePageLayoutStub = {
  template: '<div><slot name="filters" /><slot name="table" /><slot /></div>',
}
const AvailableChannelsTableStub = {
  props: ['rows'],
  template: '<div data-test="available-channels-table">{{ rows.length }}</div>',
}

describe('AvailableChannelsView price summary', () => {
  beforeEach(() => {
    getAvailableChannels.mockReset()
    getChannelPrices.mockReset()
    getAvailableGroups.mockReset()
    getUserGroupRates.mockReset()
    showError.mockReset()
    getChannelPrices.mockResolvedValue([])
  })

  it('shows current GPT 5.4, GPT 5.5 and image generation prices', async () => {
    getAvailableChannels.mockResolvedValue([
      {
        name: 'Codex Pool',
        description: '主池',
        platforms: [
          {
            platform: 'openai',
            groups: [
              {
                id: 2,
                name: 'codex-pool-19-usd',
                platform: 'openai',
                subscription_type: 'subscription',
                rate_multiplier: 1,
                is_exclusive: false,
              },
            ],
            supported_models: [
              {
                name: 'gpt-5.4',
                platform: 'openai',
                pricing: {
                  billing_mode: 'token',
                  input_price: 0.0000025,
                  output_price: 0.000015,
                  cache_write_price: 0.00000125,
                  cache_read_price: 0.00000025,
                  image_output_price: null,
                  per_request_price: null,
                  intervals: [],
                },
              },
              {
                name: 'gpt-5.5',
                platform: 'openai',
                pricing: {
                  billing_mode: 'token',
                  input_price: 0.000005,
                  output_price: 0.00003,
                  cache_write_price: 0.0000025,
                  cache_read_price: 0.0000005,
                  image_output_price: null,
                  per_request_price: null,
                  intervals: [],
                },
              },
            ],
          },
        ],
      },
    ])
    getAvailableGroups.mockResolvedValue([
      {
        id: 2,
        name: 'codex-pool-19-usd',
        platform: 'openai',
        allow_image_generation: true,
        image_price_1k: 0.1,
        image_price_2k: 0.2,
        image_price_4k: 0.4,
      },
    ])
    getUserGroupRates.mockResolvedValue({})

    const wrapper = mount(AvailableChannelsView, {
      global: {
        stubs: {
          AppLayout: AppLayoutStub,
          TablePageLayout: TablePageLayoutStub,
          AvailableChannelsTable: AvailableChannelsTableStub,
          Icon: true,
        },
      },
    })

    await flushPromises()

    const text = wrapper.text()
    expect(text).toContain('当前价格')
    expect(text).toContain('gpt-5.4')
    expect(text).toContain('$2.5 / 1M token')
    expect(text).toContain('$15 / 1M token')
    expect(text).toContain('gpt-5.5')
    expect(text).toContain('$5 / 1M token')
    expect(text).toContain('$30 / 1M token')
    expect(text).toContain('生图')
    expect(text).toContain('1K')
    expect(text).toContain('$0.10 / 张')
    expect(text).toContain('2K')
    expect(text).toContain('$0.20 / 张')
    expect(text).toContain('4K')
    expect(text).toContain('$0.40 / 张')
  })

  it('shows billing prices even when available channel table is empty', async () => {
    getAvailableChannels.mockResolvedValue([])
    getChannelPrices.mockResolvedValue([
      {
        name: 'gpt-5.4',
        billing_mode: 'token',
        source: 'litellm',
        input_price: 0.0000025,
        output_price: 0.000015,
        cache_write_price: 0,
        cache_read_price: 0.00000025,
        priority_input_price: 0.00000375,
        priority_output_price: 0.0000225,
        priority_cache_read_price: 0.000000375,
      },
      {
        name: 'gpt-5.5',
        billing_mode: 'token',
        source: 'litellm',
        input_price: 0.000005,
        output_price: 0.00003,
        cache_write_price: 0,
        cache_read_price: 0.0000005,
        priority_input_price: 0.0000075,
        priority_output_price: 0.000045,
        priority_cache_read_price: 0.00000075,
      },
    ])
    getAvailableGroups.mockResolvedValue([
      {
        id: 2,
        name: 'codex-pool-19-usd',
        platform: 'openai',
        allow_image_generation: true,
        image_price_1k: 0.1,
        image_price_2k: 0.2,
        image_price_4k: 0.4,
      },
    ])
    getUserGroupRates.mockResolvedValue({})

    const wrapper = mount(AvailableChannelsView, {
      global: {
        stubs: {
          AppLayout: AppLayoutStub,
          TablePageLayout: TablePageLayoutStub,
          AvailableChannelsTable: AvailableChannelsTableStub,
          Icon: true,
        },
      },
    })

    await flushPromises()

    const text = wrapper.text()
    expect(wrapper.find('[data-test="available-channels-table"]').text()).toBe('0')
    expect(text).toContain('gpt-5.4')
    expect(text).toContain('$2.5 / 1M token')
    expect(text).toContain('$15 / 1M token')
    expect(text).toContain('$3.75 / 1M token')
    expect(text).toContain('$22.5 / 1M token')
    expect(text).toContain('$0 / 1M token')
    expect(text).toContain('$0.25 / 1M token')
    expect(text).toContain('gpt-5.5')
    expect(text).toContain('$5 / 1M token')
    expect(text).toContain('$30 / 1M token')
    expect(text).toContain('$7.5 / 1M token')
    expect(text).toContain('$45 / 1M token')
    expect(text).toContain('$0.5 / 1M token')
    expect(text).toContain('生图')
    expect(text).toContain('$0.40 / 张')
  })
})
