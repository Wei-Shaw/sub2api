import { mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'

const localeState = vi.hoisted(() => ({ value: 'en' as 'en' | 'zh' }))

vi.mock('vue-i18n', async (importOriginal) => {
  const actual = await importOriginal<typeof import('vue-i18n')>()
  const translations: Record<'en' | 'zh', Record<string, string>> = {
    en: {
      'common.retry': 'Retry',
      'admin.accounts.quotaWindows.title': 'Standard Quota Windows',
      'admin.accounts.quotaWindows.localScope': 'Local outcomes',
      'admin.accounts.quotaWindows.loadFailed': 'Load failed',
      'admin.accounts.quotaWindows.noSupportedWindows': 'No windows',
      'admin.accounts.quotaWindows.used': 'Used',
      'admin.accounts.quotaWindows.resetsAt': 'Resets {time}',
      'admin.accounts.quotaWindows.resetUnknown': 'Unknown reset',
      'admin.accounts.quotaWindows.window.five_hour': '5-Hour Quota',
      'admin.accounts.quotaWindows.column.previous': 'Previous Window Usage',
      'admin.accounts.quotaWindows.column.current': 'Current Window Used',
      'admin.accounts.quotaWindows.column.forecast': 'Current Window Forecast',
      'admin.accounts.quotaWindows.metric.requests': 'Requests',
      'admin.accounts.quotaWindows.metric.tokens': 'Tokens',
      'admin.accounts.quotaWindows.metric.cost': 'Account Cost',
      'admin.accounts.quotaWindows.metric.successRate': 'Success Rate',
      'admin.accounts.quotaWindows.boundaryStatus.stale_snapshot': 'Stale snapshot',
      'admin.accounts.quotaWindows.successRateStatus.available': 'Available',
      'admin.accounts.quotaWindows.successRateStatus.monitoring_disabled': 'Monitoring off',
      'admin.accounts.quotaWindows.forecastByQuota': 'Quota basis'
    },
    zh: {
      'common.retry': '重试',
      'admin.accounts.quotaWindows.title': '标准额度窗口',
      'admin.accounts.quotaWindows.localScope': '本地记录口径',
      'admin.accounts.quotaWindows.loadFailed': '加载失败',
      'admin.accounts.quotaWindows.noSupportedWindows': '暂无窗口',
      'admin.accounts.quotaWindows.used': '已用',
      'admin.accounts.quotaWindows.resetsAt': '重置于 {time}',
      'admin.accounts.quotaWindows.resetUnknown': '暂无重置时间',
      'admin.accounts.quotaWindows.window.five_hour': '5 小时限额',
      'admin.accounts.quotaWindows.column.previous': '上个窗口用量',
      'admin.accounts.quotaWindows.column.current': '当前窗口已用',
      'admin.accounts.quotaWindows.column.forecast': '当前窗口预测',
      'admin.accounts.quotaWindows.metric.requests': '请求数',
      'admin.accounts.quotaWindows.metric.tokens': 'Token',
      'admin.accounts.quotaWindows.metric.cost': '账号成本',
      'admin.accounts.quotaWindows.metric.successRate': '成功率',
      'admin.accounts.quotaWindows.boundaryStatus.stale_snapshot': '快照过期',
      'admin.accounts.quotaWindows.successRateStatus.available': '可用',
      'admin.accounts.quotaWindows.successRateStatus.monitoring_disabled': '监控未开启',
      'admin.accounts.quotaWindows.forecastByQuota': '额度比例推算'
    }
  }
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string, params?: Record<string, string>) => {
        let value = translations[localeState.value][key] ?? key
        for (const [name, replacement] of Object.entries(params ?? {})) {
          value = value.replace(`{${name}}`, replacement)
        }
        return value
      }
    })
  }
})

import AccountQuotaWindowSection from '../AccountQuotaWindowSection.vue'
import type { AccountQuotaWindowModel } from '@/features/account-window-usage/accountWindowUsage'

const readyWindow = (): AccountQuotaWindowModel => ({
  key: 'five_hour',
  utilization: 82.5,
  resetAt: '2026-08-11T13:00:00.000Z',
  boundaryStatus: 'ready',
  currentRange: { startTime: '2026-08-11T08:00:00.000Z', endTime: '2026-08-11T10:30:00.000Z' },
  previousRange: { startTime: '2026-08-11T03:00:00.000Z', endTime: '2026-08-11T08:00:00.000Z' },
  current: {
    window_key: 'five_hour', period: 'current', start_time: '2026-08-11T08:00:00.000Z', end_time: '2026-08-11T10:30:00.000Z',
    matched: true, total_requests: 12, success_calls: 10, failure_calls: 2, total_tokens: 2400,
    account_cost: 1.5, standard_cost: 1.25, user_cost: 2, success_rate: null,
    success_rate_status: 'monitoring_disabled'
  },
  previous: {
    window_key: 'five_hour', period: 'previous', start_time: '2026-08-11T03:00:00.000Z', end_time: '2026-08-11T08:00:00.000Z',
    matched: true, total_requests: 8, success_calls: 8, failure_calls: 0, total_tokens: 1600,
    account_cost: 1, standard_cost: 0.8, user_cost: 1.2, success_rate: 100,
    success_rate_status: 'available'
  },
  forecast: { total_requests: 15, total_tokens: 2909, account_cost: 1.82, basis: 'quota' }
})

function mountSection(
  locale: 'en' | 'zh',
  windows: AccountQuotaWindowModel[],
  error: string | null = null,
  loading = false
) {
  localeState.value = locale
  return mount(AccountQuotaWindowSection, {
    props: { windows, loading, error },
    global: {
      stubs: { Icon: true, LoadingSpinner: true }
    }
  })
}

describe('AccountQuotaWindowSection', () => {
  it('renders three responsive columns, risk progress, and unavailable success-rate status', () => {
    const wrapper = mountSection('zh', [readyWindow()])
    expect(wrapper.text()).toContain('标准额度窗口')
    expect(wrapper.text()).toContain('上个窗口用量')
    expect(wrapper.text()).toContain('当前窗口已用')
    expect(wrapper.text()).toContain('当前窗口预测')
    expect(wrapper.text()).toContain('监控未开启')
    expect(wrapper.findAll('[data-column]')).toHaveLength(3)
    expect(wrapper.find('.grid.grid-cols-1[class~="md:grid-cols-3"]').exists()).toBe(true)
    expect(wrapper.find('[data-column="forecast"]').classes()).toContain('bg-blue-50/60')
    expect(wrapper.find('.bg-amber-500').exists()).toBe(true)
  })

  it('keeps the display percentage rounded independently from forecast precision', () => {
    const window = readyWindow()
    window.utilization = 15.121
    const wrapper = mountSection('zh', [window])

    expect(wrapper.text()).toContain('已用15.1%')
    expect(wrapper.text()).not.toContain('15.121%')
  })

  it('renders English labels and a boundary-specific unavailable state', () => {
    const window = readyWindow()
    window.boundaryStatus = 'stale_snapshot'
    window.currentRange = null
    window.previousRange = null
    const wrapper = mountSection('en', [window])
    expect(wrapper.text()).toContain('Standard Quota Windows')
    expect(wrapper.text()).toContain('Stale snapshot')
    expect(wrapper.find('[data-testid="quota-window-boundary-error"]').exists()).toBe(true)
    expect(wrapper.findAll('[data-column]')).toHaveLength(0)
  })

  it('keeps the error independent and emits retry', async () => {
    const wrapper = mountSection('en', [readyWindow()], 'load_failed')
    expect(wrapper.find('[data-testid="quota-window-error"]').exists()).toBe(true)
    await wrapper.find('[data-testid="quota-window-error"] button').trigger('click')
    expect(wrapper.emitted('retry')).toHaveLength(1)
    expect(wrapper.find('[data-window-key="five_hour"]').exists()).toBe(true)
  })

  it('reserves responsive card height while uncached windows load', () => {
    const wrapper = mountSection('en', [], null, true)
    const skeletons = wrapper.findAll('[data-testid="quota-window-skeleton"]')

    expect(skeletons).toHaveLength(2)
    expect(wrapper.findAll('[data-testid="quota-window-skeleton-column"]')).toHaveLength(6)
    for (const skeleton of skeletons) {
      expect(skeleton.find('.grid.grid-cols-1[class~="md:grid-cols-3"]').exists()).toBe(true)
      expect(skeleton.findAll('[data-testid="quota-window-skeleton-column"]')).toHaveLength(3)
    }
    expect(wrapper.find('[data-testid="quota-window-empty"]').exists()).toBe(false)
  })
})
