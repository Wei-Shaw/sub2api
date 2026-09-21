import { describe, it, expect, vi } from 'vitest'
import { mount } from '@vue/test-utils'

vi.mock('vue-i18n', () => ({
  useI18n: () => ({
    t: (key: string) => key,
  }),
}))

import UserDashboardStats from '../UserDashboardStats.vue'
import type { UserDashboardStats as UserStatsType } from '@/api/usage'

function makeStats(over: Partial<UserStatsType> = {}): UserStatsType {
  return {
    total_api_keys: 1,
    active_api_keys: 1,
    total_requests: 10,
    total_input_tokens: 100,
    total_output_tokens: 50,
    total_cache_creation_tokens: 0,
    total_cache_read_tokens: 0,
    total_tokens: 150,
    total_cost: 0.2,
    total_actual_cost: 0.1,
    today_requests: 2,
    today_input_tokens: 20,
    today_output_tokens: 10,
    today_cache_creation_tokens: 0,
    today_cache_read_tokens: 0,
    today_tokens: 30,
    today_cost: 0.04,
    today_actual_cost: 0.02,
    average_duration_ms: 420,
    rpm: 1,
    tpm: 10,
    by_platform: [],
    ...over,
  }
}

function mountStats(stats: UserStatsType, isSimple = false) {
  return mount(UserDashboardStats, {
    props: { stats, balance: 12.5, isSimple },
    global: { stubs: { Icon: true } },
  })
}

describe('UserDashboardStats R3 仪表盘统计', () => {
  it('标准模式保留 8 张统计卡', () => {
    const wrapper = mountStats(makeStats())

    expect(wrapper.findAll('.card')).toHaveLength(8)
  })

  it('移除平台拆分区域', () => {
    const wrapper = mountStats(makeStats({
      by_platform: [{
        platform: 'openai',
        total_requests: 1,
        total_tokens: 10,
        total_actual_cost: 0.1,
        today_requests: 1,
        today_tokens: 10,
        today_actual_cost: 0.1,
      }],
    }))

    expect(wrapper.html()).not.toContain('platformBreakdown')
    expect(wrapper.html()).not.toContain('data-testid="platform-card"')
  })

  it('余额和消费使用人民币符号', () => {
    const wrapper = mountStats(makeStats())

    expect(wrapper.text()).toContain('¥12.50')
    expect(wrapper.text()).toContain('¥0.0200')
    expect(wrapper.text()).toContain('¥0.0400')
  })
})
