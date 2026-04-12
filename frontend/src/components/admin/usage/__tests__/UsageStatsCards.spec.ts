import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'

import UsageStatsCards from '../UsageStatsCards.vue'

const messages: Record<string, string> = {
  'usage.totalRequests': 'Total Requests',
  'usage.inSelectedRange': 'In Selected Range',
  'usage.totalTokens': 'Total Tokens',
  'usage.grossInputTokens': 'Total Input Tokens',
  'usage.netInputTokens': 'Net Input Tokens',
  'usage.out': 'Out',
  'usage.totalCost': 'Total Cost',
  'usage.userBilled': 'User billed',
  'usage.standardCost': 'Standard Cost',
  'usage.avgDuration': 'Avg Duration',
}

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string) => messages[key] ?? key,
    }),
  }
})

describe('UsageStatsCards', () => {
  it('shows gross input as primary summary and net input as secondary text', () => {
    const wrapper = mount(UsageStatsCards, {
      props: {
        stats: {
          total_requests: 10,
          total_input_tokens: 100,
          total_output_tokens: 20,
          total_cache_tokens: 40,
          total_tokens: 160,
          total_cost: 1,
          total_actual_cost: 1,
          average_duration_ms: 100,
        },
      },
      global: {
        stubs: {
          Icon: true,
        },
      },
    })

    const text = wrapper.text()
    expect(text).toContain('160')
    expect(text).toContain('Total Input Tokens: 140 / Out: 20')
    expect(text).toContain('Net Input Tokens: 100')
  })
})
