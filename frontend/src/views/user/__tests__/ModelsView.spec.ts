import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import ModelsView from '../ModelsView.vue'

const getAvailable = vi.hoisted(() => vi.fn())
const getUserGroupRates = vi.hoisted(() => vi.fn())
const showError = vi.hoisted(() => vi.fn())

vi.mock('@/api/channels', () => ({
  default: { getAvailable },
}))

vi.mock('@/api/groups', () => ({
  default: { getUserGroupRates },
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({ showError }),
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
        'models.searchPlaceholder': '搜索渠道、平台或模型',
        'models.noImageSupport': '文生图暂不支持',
        'models.readonlyHint': '此页面仅用于查看',
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
    getAvailable.mockReset()
    getUserGroupRates.mockReset()
    showError.mockReset()
    getUserGroupRates.mockResolvedValue({})
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
    expect(wrapper.text()).toContain('模型广场')
    expect(wrapper.text()).toContain('Claude 渠道')
    expect(wrapper.text()).toContain('claude-sonnet-4')
    expect(wrapper.text()).toContain('文生图暂不支持')
  })
})
