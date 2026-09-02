import { mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'

import UserDashboardStats from '../UserDashboardStats.vue'

vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))

describe('UserDashboardStats temporary balance', () => {
  it('renders active temporary balance and expiry beside permanent balance', () => {
    const wrapper = mount(UserDashboardStats, {
      props: {
        stats: {
          total_api_keys: 0, active_api_keys: 0, total_requests: 0, total_input_tokens: 0,
          total_output_tokens: 0, total_cache_creation_tokens: 0, total_cache_read_tokens: 0,
          total_tokens: 0, total_cost: 0, total_actual_cost: 0, today_requests: 0,
          today_input_tokens: 0, today_output_tokens: 0, today_cache_creation_tokens: 0,
          today_cache_read_tokens: 0, today_tokens: 0, today_cost: 0, today_actual_cost: 0,
          average_duration_ms: 0, rpm: 0, tpm: 0,
          active_temporary_balance: 8,
          temporary_balance_expires_at: '2026-09-03T00:00:00Z'
        },
        balance: 10,
        isSimple: false,
        platformQuotas: []
      },
      global: {
        stubs: {
          Icon: true,
          TemporaryBalanceCard: {
            props: ['amount', 'expiresAt'],
            template: '<div data-testid="dashboard-temporary-balance">{{ amount }} {{ expiresAt }}</div>'
          }
        }
      }
    })

    expect(wrapper.get('[data-testid="dashboard-temporary-balance"]').text()).toContain('8')
    expect(wrapper.get('[data-testid="dashboard-temporary-balance"]').text()).toContain('2026-09-03')
  })
})
