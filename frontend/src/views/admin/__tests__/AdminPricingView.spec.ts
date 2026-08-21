import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import AdminPricingView from '../AdminPricingView.vue'

const { getAllIncludingInactive, listChannels, showError } = vi.hoisted(() => ({
  getAllIncludingInactive: vi.fn(),
  listChannels: vi.fn(),
  showError: vi.fn(),
}))

vi.mock('@/api/admin', () => ({
  default: {
    groups: {
      getAllIncludingInactive,
    },
    channels: {
      list: listChannels,
    },
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
          'admin.pricing.discountLabel': '{discount}折',
          'admin.pricing.savePercent': '省 {percent}%',
          'admin.pricing.groupRateHint': '{rate} 倍率 · 按分组倍率折算',
        }
        const template = messages[key] || key
        if (!params) return key
        return Object.entries(params).reduce(
          (text, [name, value]) => text.replace(`{${name}}`, String(value)),
          template,
        )
      },
    }),
  }
})

describe('AdminPricingView', () => {
  beforeEach(() => {
    getAllIncludingInactive.mockReset()
    listChannels.mockReset()
    showError.mockReset()

    getAllIncludingInactive.mockResolvedValue([
      {
        id: 1,
        name: 'Claude lite',
        description: 'Fast low-cost group',
        platform: 'anthropic',
        rate_multiplier: 0.5,
        is_exclusive: false,
        status: 'active',
        subscription_type: 'standard',
        daily_limit_usd: null,
        weekly_limit_usd: null,
        monthly_limit_usd: null,
        allow_image_generation: false,
        image_rate_independent: false,
        image_rate_multiplier: 1,
        image_price_1k: null,
        image_price_2k: null,
        image_price_4k: null,
        peak_rate_enabled: false,
        peak_start: '',
        peak_end: '',
        peak_rate_multiplier: 1,
        claude_code_only: false,
        fallback_group_id: null,
        fallback_group_id_on_invalid_request: null,
        require_oauth_only: false,
        require_privacy_set: false,
        created_at: '',
        updated_at: '',
        model_routing: null,
        model_routing_enabled: false,
        mcp_xml_inject: true,
        sort_order: 10,
      },
    ])

    listChannels.mockResolvedValue({
      items: [
        {
          id: 7,
          name: 'Primary Channel',
          description: 'Default channel',
          status: 'active',
          billing_model_source: 'requested',
          restrict_models: false,
          group_ids: [1],
          model_pricing: [
            {
              platform: 'anthropic',
              models: ['claude-sonnet-4-5'],
              billing_mode: 'token',
              input_price: 0.000003,
              output_price: 0.000015,
              cache_write_price: null,
              cache_read_price: null,
              image_output_price: null,
              per_request_price: null,
              intervals: [],
            },
          ],
          model_mapping: {},
          apply_pricing_to_account_stats: false,
          account_stats_pricing_rules: [],
          created_at: '',
          updated_at: '',
        },
      ],
      total: 1,
    })
  })

  it('renders platform tabs, group cards, and prices calculated by group multiplier', async () => {
    const wrapper = mount(AdminPricingView, {
      global: {
        stubs: {
          AppLayout: { template: '<div><slot /></div>' },
          Icon: { props: ['name'], template: '<span />' },
          PlatformIcon: { props: ['platform'], template: '<span>{{ platform }}</span>' },
        },
      },
    })

    await flushPromises()

    expect(getAllIncludingInactive).toHaveBeenCalledTimes(1)
    expect(listChannels).toHaveBeenCalledWith(1, 100)
    expect(wrapper.text()).toContain('Claude Code')
    expect(wrapper.text()).toContain('Claude lite')
    expect(wrapper.text()).toContain('5折')
    expect(wrapper.text()).toContain('claude-sonnet-4-5')
    expect(wrapper.text()).toContain('¥10.50')
    expect(wrapper.text()).toContain('¥21')
    expect(wrapper.text()).toContain('省 50%')
    expect(wrapper.text()).not.toContain('Primary Channel')
    expect(wrapper.text()).not.toContain('admin.pricing.model.channel')
    expect(wrapper.text()).not.toContain('admin.pricing.model.groups')
  })
})
