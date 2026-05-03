import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'

import TokenUsageTrend from '../TokenUsageTrend.vue'

vi.mock('vue-chartjs', () => ({
  Line: {
    props: ['data', 'options'],
    template: '<pre data-testid="chart">{{ JSON.stringify(data) }}</pre>',
  },
}))

vi.mock('vue-i18n', () => ({
  useI18n: () => ({
    t: (key: string) =>
      ({
        'dashboard.tokenInput': '输入 Token',
        'dashboard.tokenOutput': '输出 Token',
        'dashboard.tokenCacheWrite': '缓存写入 Token',
        'dashboard.tokenCacheRead': '缓存读取 Token',
        'dashboard.cacheHitRate': '缓存命中率',
        'dashboard.actualCost': '实际费用',
        'dashboard.standardCost': '标准费用',
        'admin.dashboard.tokenUsageTrend': 'Token 使用趋势',
        'admin.dashboard.noDataAvailable': '暂无数据',
      })[key] || key,
  }),
}))

describe('TokenUsageTrend', () => {
  it('uses localized Chinese labels instead of English dataset names', () => {
    const wrapper = mount(TokenUsageTrend, {
      props: {
        trendData: [
          {
            date: '2026-05-01',
            input_tokens: 10,
            output_tokens: 5,
            cache_creation_tokens: 3,
            cache_read_tokens: 2,
            actual_cost: 0.1,
            cost: 0.2,
          },
        ],
      },
      global: {
        stubs: {
          LoadingSpinner: true,
        },
      },
    })

    const text = wrapper.get('[data-testid="chart"]').text()
    expect(text).toContain('输入 Token')
    expect(text).toContain('输出 Token')
    expect(text).toContain('缓存写入 Token')
    expect(text).toContain('缓存读取 Token')
    expect(text).toContain('缓存命中率')
    expect(text).not.toContain('Input')
    expect(text).not.toContain('Output')
    expect(text).not.toContain('Cache Creation')
    expect(text).not.toContain('Cache Read')
    expect(text).not.toContain('Cache Hit Rate')
  })
})
