import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import ModelPricingView from '../ModelPricingView.vue'

const { getPricing, showError } = vi.hoisted(() => ({
  getPricing: vi.fn(),
  showError: vi.fn(),
}))

vi.mock('@/api/channels', () => ({
  default: {
    getPricing,
  },
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({
    showError,
  }),
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string, params?: Record<string, unknown>) => {
        const messages: Record<string, string> = {
          'modelPricing.title': '模型定价',
          'modelPricing.description': '查看各分组倍率下的模型计费价格',
          'modelPricing.discountLabel': '{discount}折',
          'modelPricing.savePercent': '省 {percent}%',
          'modelPricing.groupRateHint': '{rate} 倍率 · 按分组倍率折算',
          'modelPricing.billingModel': '计费模型',
          'modelPricing.officialLabel': '官方价 ',
          'modelPricing.pricing.billingModeToken': '按 Token',
          'modelPricing.pricing.inputPrice': '输入',
          'modelPricing.pricing.outputPrice': '输出',
          'modelPricing.pricing.unitPerMillion': '/ 1M token',
        }
        const template = messages[key] || key
        if (!params) return template
        return Object.entries(params).reduce(
          (text, [name, value]) => text.replace(`{${name}}`, String(value)),
          template,
        )
      },
    }),
  }
})

describe('ModelPricingView', () => {
  beforeEach(() => {
    getPricing.mockReset()
    showError.mockReset()

    getPricing.mockResolvedValue([
      {
        name: 'Primary Channel',
        description: 'Default channel',
        platforms: [
          {
            platform: 'anthropic',
            groups: [
              {
                id: 1,
                name: 'Claude lite',
                platform: 'anthropic',
                subscription_type: 'standard',
                rate_multiplier: 0.8,
                peak_rate_enabled: false,
                peak_start: '',
                peak_end: '',
                peak_rate_multiplier: 1,
                is_exclusive: false,
              },
            ],
            supported_models: [
              {
                name: 'claude-sonnet-4-5',
                platform: 'anthropic',
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
  })

  it('renders model pricing calculated by the group multiplier', async () => {
    const wrapper = mount(ModelPricingView, {
      global: {
        stubs: {
          AppLayout: { template: '<div><slot /></div>' },
          Icon: { props: ['name'], template: '<span />' },
          PlatformIcon: { props: ['platform'], template: '<span>{{ platform }}</span>' },
        },
      },
    })

    await flushPromises()

    expect(getPricing).toHaveBeenCalledTimes(1)
    expect(wrapper.text()).toContain('Claude Code')
    expect(wrapper.text()).toContain('Claude lite')
    expect(wrapper.text()).toContain('8折')
    expect(wrapper.text()).toContain('计费模型')
    expect(wrapper.text()).toContain('claude-sonnet-4-5')
    expect(wrapper.text()).toContain('¥16.80')
    expect(wrapper.text()).toContain('¥21')
    expect(wrapper.text()).toContain('省 20%')
    expect(wrapper.text()).not.toContain('Primary Channel')
  })
})
