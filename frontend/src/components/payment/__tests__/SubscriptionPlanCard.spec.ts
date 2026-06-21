import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'
import { createI18n } from 'vue-i18n'
import SubscriptionPlanCard from '../SubscriptionPlanCard.vue'
import type { SubscriptionPlan } from '@/types/payment'

const msg = (text: string) => () => text

const messages = {
  en: {
    payment: {
      days: msg('days'),
      models: msg('Models'),
      planCard: {
        quota: msg('Quota'),
        rate: msg('Rate'),
        unlimited: msg('Unlimited'),
        dailyLimit: msg('Daily'),
        summary: msg('Monthly subscription - duration {days} days, daily limit {limit} USD, refreshes at 24:00'),
      },
      subscribeNow: msg('Subscribe now'),
    },
  },
  zh: {
    payment: {
      days: msg('天'),
      models: msg('模型'),
      planCard: {
        quota: msg('配额'),
        rate: msg('倍率'),
        unlimited: msg('无限制'),
        dailyLimit: msg('日限额'),
        summary: msg('月度订阅-时间 {days}天，日限额 {limit}刀，24点刷新'),
      },
      subscribeNow: msg('立即开通'),
    },
  },
}

const createTestI18n = (locale = 'en') =>
  createI18n({
    legacy: false,
    locale,
    fallbackWarn: false,
    missingWarn: false,
    messages,
  })

const planFixture = (overrides: Partial<SubscriptionPlan> = {}): SubscriptionPlan => ({
  id: 1,
  group_id: 10,
  group_platform: 'openai',
  name: 'Pro',
  description: '',
  price: 10,
  features: [],
  rate_multiplier: 1,
  validity_days: 30,
  validity_unit: 'day',
  supported_model_scopes: ['claude', 'gemini_text', 'gemini_image'],
  is_active: true,
  for_sale: true,
  sort_order: 0,
  ...overrides,
})

const mountPlanCard = (groupPlatform: string) =>
  mount(SubscriptionPlanCard, {
    props: {
      plan: planFixture({ group_platform: groupPlatform }),
    },
    global: { plugins: [createTestI18n()] },
  })

describe('SubscriptionPlanCard', () => {
  it('does not show Antigravity model scopes for OpenAI plans', () => {
    const text = mountPlanCard('openai').text()

    expect(text).not.toContain('Claude')
    expect(text).not.toContain('Gemini')
    expect(text).not.toContain('Imagen')
  })

  it('shows model scopes for Antigravity plans', () => {
    const text = mountPlanCard('antigravity').text()

    expect(text).toContain('Claude')
    expect(text).toContain('Gemini')
    expect(text).toContain('Imagen')
  })

  it('uses simplified RMB price and monthly quota copy for sale plans', () => {
    const wrapper = mount(SubscriptionPlanCard, {
      props: {
        plan: planFixture({
          name: '29 元套餐',
          price: 29,
          daily_limit_usd: 19,
          rate_multiplier: 1,
          features: ['多余说明'],
        }),
      },
      global: { plugins: [createTestI18n('zh')] },
    })
    const text = wrapper.text()

    expect(text).toContain('¥29元')
    expect(text).toContain('月度订阅-时间 30天，日限额 19刀，24点刷新')
    expect(text).toContain('立即开通')
    expect(text).not.toContain('$')
    expect(text).not.toContain('/ 30')
    expect(text).not.toContain('倍率')
    expect(text).not.toContain('模型')
    expect(text).not.toContain('多余说明')
  })
})
