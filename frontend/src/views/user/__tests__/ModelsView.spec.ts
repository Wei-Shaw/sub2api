import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import ModelsView from '../ModelsView.vue'

const getAvailable = vi.hoisted(() => vi.fn())
const getModelPricingBatch = vi.hoisted(() => vi.fn())
const getUserGroupRates = vi.hoisted(() => vi.fn())
const showError = vi.hoisted(() => vi.fn())
const authState = vi.hoisted(() => ({ isAuthenticated: true }))

vi.mock('@/api/channels', () => ({
  default: { getAvailable, getModelPricingBatch },
}))

vi.mock('@/api/groups', () => ({
  default: { getUserGroupRates },
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({ showError }),
}))

vi.mock('@/stores/auth', () => ({
  useAuthStore: () => authState,
}))

vi.mock('@/utils/apiError', () => ({
  extractApiErrorMessage: (_err: unknown, fallback: string) => fallback,
}))

vi.mock('@/i18n', () => ({
  i18n: {
    global: {
      t: (key: string) => key,
    },
  },
}))

vi.mock('vue-i18n', () => ({
  createI18n: () => ({
    global: {
      locale: { value: 'zh' },
      t: (key: string) => key,
      setLocaleMessage: vi.fn(),
    },
  }),
  useI18n: () => ({
    t: (key: string) =>
      ({
        'models.title': '模型广场',
        'models.description': '查看当前可用渠道与模型价格',
        'models.searchPlaceholder': '搜索平台或模型',
        'models.platformFilter': '平台筛选',
        'models.allPlatforms': '全部平台',
        'models.channelCount': '{count} 个渠道',
        'models.modelCount': '{count} 个模型',
        'models.modelsUnit': '个模型',
        'models.modelName': '模型名称',
        'models.groupRate': '分组倍率',
        'models.billingMode': '计费方式',
        'models.currency': '币种',
        'models.sitePrice': '本站',
        'models.officialPrice': '官方',
        'models.savings': '省',
        'models.priceInput': '输入',
        'models.priceOutput': '输出',
        'models.priceCacheWrite': '缓存写',
        'models.priceCacheRead': '缓存读',
        'models.priceImage': '图片',
        'models.discount': '折扣',
        'models.noPricing': '未配置价格',
        'availableChannels.columns.name': '渠道',
        'availableChannels.columns.description': '说明',
        'availableChannels.columns.platform': '平台',
        'availableChannels.columns.groups': '可用范围',
        'availableChannels.columns.supportedModels': '模型与价格',
        'availableChannels.noPricing': '未配置',
        'availableChannels.noModels': '暂无模型',
        'availableChannels.empty': '暂无可用模型',
        'availableChannels.exclusive': '专属',
        'availableChannels.public': '公开',
        'availableChannels.exclusiveTooltip': '专属分组',
        'availableChannels.publicTooltip': '公开分组',
      })[key] || key,
  }),
}))

describe('ModelsView', () => {
  beforeEach(() => {
    authState.isAuthenticated = true
    getAvailable.mockReset()
    getModelPricingBatch.mockReset()
    getUserGroupRates.mockReset()
    showError.mockReset()
    getUserGroupRates.mockResolvedValue({})
    getModelPricingBatch.mockResolvedValue({
      prices: {
        'claude-sonnet-4': {
          found: true,
          billing_mode: 'token',
          input_price: 0.000003,
          output_price: 0.000015,
          cache_write_price: 0.00000375,
          cache_read_price: 0.0000003,
          image_output_price: null,
          per_request_price: null,
        },
      },
    })
    getAvailable.mockResolvedValue([
      {
        name: 'Claude 渠道',
        description: '官方 Claude',
        platforms: [
          {
            platform: 'anthropic',
            groups: [],
            supported_models: [
              {
                name: 'claude-sonnet-4',
                platform: 'anthropic',
                pricing: {
                  billing_mode: 'token',
                  input_price: 3,
                  output_price: 15,
                  cache_write_price: null,
                  cache_read_price: null,
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
  })

  it('renders a read-only model plaza using existing available-channel data', async () => {
    const wrapper = mount(ModelsView, {
      global: {
        stubs: {
          AppLayout: { template: '<div><slot /></div>' },
          Icon: { template: '<span />' },
          PlatformIcon: { template: '<span />' },
          GroupBadge: { template: '<span />' },
        },
      },
    })
    await flushPromises()

    expect(getAvailable).toHaveBeenCalled()
    expect(getModelPricingBatch).toHaveBeenCalledWith(['claude-sonnet-4'])
    expect(wrapper.text()).toContain('模型广场')
    expect(wrapper.text()).toContain('Claude 渠道')
    expect(wrapper.text()).toContain('claude-sonnet-4')
    expect(wrapper.text()).not.toContain('文生图暂不支持')
    expect(wrapper.text()).not.toContain('此页面仅用于查看')
  })

  it('uses the public model plaza endpoint without loading user group rates for guests', async () => {
    authState.isAuthenticated = false

    mount(ModelsView, {
      global: {
        stubs: {
          AppLayout: { template: '<div><slot /></div>' },
          Icon: { template: '<span />' },
          PlatformIcon: { template: '<span />' },
        },
      },
    })
    await flushPromises()

    expect(getAvailable).toHaveBeenCalledWith({ public: true })
    expect(getUserGroupRates).not.toHaveBeenCalled()
    expect(getModelPricingBatch).toHaveBeenCalledWith(['claude-sonnet-4'])
  })

  it('supports platform filtering and shows channel model prices inline', async () => {
    getAvailable.mockResolvedValue([
      {
        name: '多平台渠道',
        description: '统一渠道',
        platforms: [
          {
            platform: 'anthropic',
            groups: [],
            supported_models: [
              {
                name: 'claude-sonnet-4',
                platform: 'anthropic',
                pricing: {
                  billing_mode: 'token',
                  input_price: 0.000003,
                  output_price: 0.000015,
                  cache_write_price: 0.00000375,
                  cache_read_price: 0.0000003,
                  image_output_price: null,
                  per_request_price: null,
                  intervals: [],
                },
              },
            ],
          },
          {
            platform: 'openai',
            groups: [
              {
                id: 1,
                name: 'OpenAI 专业组',
                platform: 'openai',
                subscription_type: 'subscription',
                rate_multiplier: 0.8,
                is_exclusive: false,
              },
            ],
            supported_models: [
              {
                name: 'gpt-4.1',
                platform: 'openai',
                pricing: {
                  billing_mode: 'token',
                  input_price: 0.000001,
                  output_price: 0.000004,
                  cache_write_price: null,
                  cache_read_price: null,
                  image_output_price: null,
                  per_request_price: null,
                  intervals: [],
                },
              },
            ],
          },
          {
            platform: 'gemini',
            groups: [],
            supported_models: [
              {
                name: 'gemini-2.5-pro',
                platform: 'gemini',
                pricing: {
                  billing_mode: 'token',
                  input_price: 0.00000125,
                  output_price: 0.00001,
                  cache_write_price: null,
                  cache_read_price: null,
                  image_output_price: null,
                  per_request_price: null,
                  intervals: [],
                },
              },
            ],
          },
          {
            platform: 'deepseek',
            groups: [],
            supported_models: [
              {
                name: 'deepseek-chat',
                platform: 'deepseek',
                pricing: {
                  billing_mode: 'token',
                  input_price: 0.00000014,
                  output_price: 0.00000028,
                  cache_write_price: null,
                  cache_read_price: null,
                  image_output_price: null,
                  per_request_price: null,
                  intervals: [],
                },
              },
            ],
          },
          {
            platform: 'xai',
            groups: [],
            supported_models: [
              {
                name: 'grok-4',
                platform: 'xai',
                pricing: {
                  billing_mode: 'token',
                  input_price: 0.000003,
                  output_price: 0.000015,
                  cache_write_price: null,
                  cache_read_price: null,
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
    getModelPricingBatch.mockResolvedValue({
      prices: {
        'claude-sonnet-4': {
          found: true,
          billing_mode: 'token',
          input_price: 0.000003,
          output_price: 0.000015,
          cache_write_price: 0.00000375,
          cache_read_price: 0.0000003,
          image_output_price: null,
          per_request_price: null,
        },
        'gpt-4.1': {
          found: true,
          billing_mode: 'token',
          input_price: 0.000002,
          output_price: 0.000008,
          cache_write_price: null,
          cache_read_price: null,
          image_output_price: null,
          per_request_price: null,
        },
        'gemini-2.5-pro': {
          found: true,
          billing_mode: 'token',
          input_price: 0.00000125,
          output_price: 0.00001,
          cache_write_price: null,
          cache_read_price: null,
          image_output_price: null,
          per_request_price: null,
        },
        'deepseek-chat': {
          found: false,
        },
        'grok-4': {
          found: false,
        },
      },
    })

    const wrapper = mount(ModelsView, {
      global: {
        stubs: {
          AppLayout: { template: '<div><slot /></div>' },
          Icon: { template: '<span />' },
          PlatformIcon: { template: '<span />' },
        },
      },
    })
    await flushPromises()

    expect(wrapper.find('[data-testid="platform-filter"]').exists()).toBe(true)
    expect(wrapper.get('[data-testid="platform-filter-wrap"]').classes()).toContain('after:border-t-[6px]')
    expect(wrapper.get('[data-testid="platform-filter"]').classes()).toContain('appearance-none')
    expect(wrapper.findAll('[data-testid="platform-filter"] option').map((option) => option.text())).toEqual([
      '全部平台',
      'OpenAI',
      'Anthropic',
      'Gemini',
      'deepseek',
      'xai',
    ])
    expect(wrapper.findAll('.model-platform-section').length).toBe(0)
    expect(wrapper.get('[data-testid="platform-filter"]').text()).toContain('OpenAI')
    expect(wrapper.get('[data-testid="platform-filter"]').text()).not.toContain('openai')
    expect(wrapper.text()).toContain('claude-sonnet-4')
    expect(wrapper.text()).toContain('¥21.00')
    expect(wrapper.text()).toContain('¥105.00')
    expect(wrapper.findAll('.model-channel-card').length).toBe(5)
    expect(wrapper.findAll('.model-channel-platform-badge').map((badge) => badge.text())).toEqual([
      'OpenAI',
      'Anthropic',
      'Gemini',
      'deepseek',
      'xai',
    ])

    await wrapper.get('[data-testid="platform-filter"]').setValue('openai')

    expect(wrapper.findAll('.model-platform-section').length).toBe(0)
    expect(wrapper.text()).toContain('OpenAI')
    expect(wrapper.findAll('.model-channel-platform-badge').length).toBe(1)
    expect(wrapper.find('.model-channel-group-badges').text()).toContain('分组倍率x0.8')
    expect(wrapper.find('.model-channel-group-badges').text()).not.toContain('OpenAI 专业组')
    expect(wrapper.find('.model-channel-count').text()).toContain('1')
    expect(wrapper.text()).toContain('gpt-4.1')
    expect(wrapper.find('th.model-billing-mode-column').exists()).toBe(true)
    expect(wrapper.find('td.model-billing-mode-cell').text()).toContain('availableChannels.pricing.billingModeToken')
    expect(wrapper.find('table').classes()).toContain('table-fixed')
    expect(wrapper.find('thead tr').classes()).toEqual(
      expect.arrayContaining(['font-semibold', 'text-gray-700', 'dark:text-gray-200']),
    )
    expect(wrapper.findAll('thead th')).toHaveLength(6)
    expect(wrapper.findAll('thead th').map((header) => header.text())).toEqual([
      '模型名称',
      '计费方式',
      '输入',
      '输出',
      '缓存读',
      '折扣',
    ])
    expect(wrapper.text()).not.toContain('缓存写')
    expect(wrapper.text()).not.toContain('图片')
    expect(wrapper.text()).toContain('0.6折')
    expect(wrapper.find('col.model-name-width').classes()).toContain('w-[18%]')
    expect(wrapper.find('col.model-billing-mode-width').classes()).toContain('w-[7%]')
    expect(wrapper.find('th.model-billing-mode-column').classes()).toContain('px-2')
    expect(wrapper.find('td.model-billing-mode-cell').classes()).toContain('px-2')
    expect(wrapper.findAll('td.model-price-cell')).toHaveLength(3)
    expect(wrapper.find('td.model-price-cell').classes()).toContain('text-left')
    expect(wrapper.find('td.model-price-cell .model-site-price').classes()).not.toContain('text-primary-700')
    expect(wrapper.find('td.model-price-cell .model-site-price').text()).toContain('本站¥0.80')
    expect(wrapper.find('.model-discount-badge').text()).toBe('0.6折')
    expect(wrapper.find('[data-testid="currency-cny"]').exists()).toBe(true)
    expect(wrapper.get('[data-testid="model-plaza-actions"]').element.children[0]).toBe(
      wrapper.get('[data-testid="models-refresh"]').element,
    )
    const currencyButtons = wrapper.findAll('[data-testid^="currency-"]')
    expect(currencyButtons.map((button) => button.text())).toEqual(['CNY', 'USD'])
    expect(wrapper.text()).toContain('本站¥0.80')
    expect(wrapper.text()).toContain('官方¥14.00')
    expect(wrapper.text()).not.toContain('省')

    await wrapper.get('[data-testid="currency-usd"]').trigger('click')

    expect(wrapper.text()).toContain('本站$0.11')
    expect(wrapper.text()).toContain('官方$2.00')
    expect(wrapper.text()).not.toContain('省')
    expect(wrapper.text()).not.toContain('claude-sonnet-4')
  })

  it('uses the lowest visible group multiplier when showing model plaza site prices', async () => {
    getUserGroupRates.mockResolvedValue({ 3: 0.4 })
    getAvailable.mockResolvedValue([
      {
        name: 'OpenAI 渠道',
        description: '多分组渠道',
        platforms: [
          {
            platform: 'openai',
            groups: [
              {
                id: 1,
                name: '默认组',
                platform: 'openai',
                subscription_type: 'standard',
                rate_multiplier: 0.8,
                is_exclusive: false,
              },
              {
                id: 2,
                name: '低价组',
                platform: 'openai',
                subscription_type: 'standard',
                rate_multiplier: 0.5,
                is_exclusive: false,
              },
              {
                id: 3,
                name: '用户专属组',
                platform: 'openai',
                subscription_type: 'standard',
                rate_multiplier: 0.9,
                is_exclusive: true,
              },
            ],
            supported_models: [
              {
                name: 'gpt-4.1',
                platform: 'openai',
                pricing: {
                  billing_mode: 'token',
                  input_price: 0.000001,
                  output_price: 0.000004,
                  cache_write_price: null,
                  cache_read_price: 0.0000001,
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
    getModelPricingBatch.mockResolvedValue({
      prices: {
        'gpt-4.1': {
          found: true,
          billing_mode: 'token',
          input_price: 0.000002,
          output_price: 0.000008,
          cache_write_price: null,
          cache_read_price: 0.0000002,
          image_output_price: null,
          per_request_price: null,
        },
      },
    })

    const wrapper = mount(ModelsView, {
      global: {
        stubs: {
          AppLayout: { template: '<div><slot /></div>' },
          Icon: { template: '<span />' },
          PlatformIcon: { template: '<span />' },
        },
      },
    })
    await flushPromises()

    expect(wrapper.find('.model-channel-group-badges').text()).toContain('分组倍率x0.4')
    expect(wrapper.text()).toContain('本站¥0.40')
    expect(wrapper.text()).toContain('本站¥1.60')
    expect(wrapper.text()).toContain('本站¥0.04')
    expect(wrapper.find('.model-discount-badge').text()).toBe('0.3折')

    await wrapper.get('[data-testid="currency-usd"]').trigger('click')

    expect(wrapper.text()).toContain('本站$0.06')
    expect(wrapper.text()).toContain('本站$0.23')
    expect(wrapper.text()).toContain('本站$0.01')
  })
})
