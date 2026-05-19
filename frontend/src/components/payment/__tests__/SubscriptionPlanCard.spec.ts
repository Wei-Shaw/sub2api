import { mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'
import SubscriptionPlanCard from '../SubscriptionPlanCard.vue'
import type { SubscriptionPlan } from '@/types/payment'

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string) => key,
    }),
  }
})

function planFixture(overrides: Partial<SubscriptionPlan> = {}): SubscriptionPlan {
  return {
    id: 1,
    group_id: 10,
    group_platform: 'openai',
    group_name: 'OpenAI',
    rate_multiplier: 1,
    daily_limit_usd: null,
    weekly_limit_usd: null,
    monthly_limit_usd: null,
    supported_model_scopes: ['claude', 'gemini_text', 'gemini_image'],
    name: 'Starter',
    description: '',
    price: 20,
    original_price: 0,
    validity_days: 30,
    validity_unit: 'day',
    features: [],
    for_sale: true,
    sort_order: 1,
    ...overrides,
  }
}

describe('SubscriptionPlanCard', () => {
  it('hides model scope badges for non-Antigravity plans', () => {
    const wrapper = mount(SubscriptionPlanCard, {
      props: {
        plan: planFixture({ group_platform: 'openai' }),
      },
    })

    expect(wrapper.text()).not.toContain('payment.planCard.models')
    expect(wrapper.text()).not.toContain('Claude')
    expect(wrapper.text()).not.toContain('Gemini')
    expect(wrapper.text()).not.toContain('Imagen')
  })

  it('shows model scope badges for Antigravity plans', () => {
    const wrapper = mount(SubscriptionPlanCard, {
      props: {
        plan: planFixture({ group_platform: 'antigravity' }),
      },
    })

    expect(wrapper.text()).toContain('payment.planCard.models')
    expect(wrapper.text()).toContain('Claude')
    expect(wrapper.text()).toContain('Gemini')
    expect(wrapper.text()).toContain('Imagen')
  })
})
