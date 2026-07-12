import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import AccountTodayStatsCell from '../AccountTodayStatsCell.vue'

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string, params?: Record<string, unknown>) => {
        const messages: Record<string, string> = {
          'admin.accounts.stats.requests': '请求',
          'admin.accounts.stats.tokens': 'Token',
          'admin.accounts.stats.kiroCredits': '消费Credits',
          'admin.accounts.stats.approxCost': `约${params?.amount ?? ''}`,
          'usage.accountBilled': '账号计费',
          'usage.userBilled': '用户扣费'
        }
        return messages[key] ?? key
      }
    })
  }
})

vi.mock('@/i18n', () => ({
  i18n: {
    global: {
      t: (key: string, params?: Record<string, unknown>) => key + JSON.stringify(params ?? {})
    }
  },
  getLocale: () => 'en-US'
}))

const stats = {
  requests: 2,
  tokens: 140600,
  cost: 0.77,
  user_cost: 0.7,
  kiro_credits: 0.17
}

describe('AccountTodayStatsCell', () => {
  it('shows Kiro credits with estimated cost when unit price is configured', () => {
    const wrapper = mount(AccountTodayStatsCell, {
      props: {
        stats,
        platform: 'kiro',
        kiroCreditUnitPriceUsd: 0.071
      }
    })

    expect(wrapper.text()).toContain('消费Credits:')
    expect(wrapper.text()).toContain('0.17')
    expect(wrapper.text()).toContain('约$0.01')
  })

  it('highlights the full credits row when estimated credit cost exceeds user cost', () => {
    const wrapper = mount(AccountTodayStatsCell, {
      props: {
        stats: {
          ...stats,
          user_cost: 0.5,
          kiro_credits: 10
        },
        platform: 'kiro',
        kiroCreditUnitPriceUsd: 0.1
      }
    })

    const row = wrapper.get('[data-testid="kiro-credits-row"]')
    expect(row.classes()).toContain('text-red-600')
    expect(row.classes()).toContain('dark:text-red-400')
    expect(wrapper.text()).toContain('消费Credits:')
    expect(wrapper.text()).toContain('10')
    expect(wrapper.text()).toContain('约$1.00')
  })

  it('does not highlight credits when estimated credit cost is covered by user cost', () => {
    const wrapper = mount(AccountTodayStatsCell, {
      props: {
        stats: {
          ...stats,
          user_cost: 1,
          kiro_credits: 10
        },
        platform: 'kiro',
        kiroCreditUnitPriceUsd: 0.1
      }
    })

    expect(wrapper.get('[data-testid="kiro-credits-row"]').classes()).not.toContain('text-red-600')
  })

  it('shows Kiro credits without estimated cost when unit price is zero', () => {
    const wrapper = mount(AccountTodayStatsCell, {
      props: {
        stats,
        platform: 'kiro',
        kiroCreditUnitPriceUsd: 0
      }
    })

    expect(wrapper.text()).toContain('消费Credits:')
    expect(wrapper.text()).toContain('0.17')
    expect(wrapper.text()).not.toContain('约$')
    expect(wrapper.get('[data-testid="kiro-credits-row"]').classes()).not.toContain('text-red-600')
  })

  it('does not show credits for non-Kiro accounts', () => {
    const wrapper = mount(AccountTodayStatsCell, {
      props: {
        stats,
        platform: 'openai',
        kiroCreditUnitPriceUsd: 0.071
      }
    })

    expect(wrapper.text()).not.toContain('消费Credits')
  })
})
