import { mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'

import type { MonitorQuotaSnapshot } from '@/api/admin/channelMonitor'
import MonitorQuotaView from '../MonitorQuotaView.vue'

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    // te() 恒真：已知 token 直接返回 i18n key，便于断言 window/label 映射。
    // 带插值的调用把参数序列化附在 key 后，便于断言账号计数摘要的具体数字。
    useI18n: () => ({
      t: (key: string, params?: Record<string, unknown>) =>
        params ? `${key}:${JSON.stringify(params)}` : key,
      te: () => true,
    }),
  }
})

function makeSnapshot(overrides: Partial<MonitorQuotaSnapshot> = {}): MonitorQuotaSnapshot {
  return {
    source: 'usage',
    success: true,
    fetched_at: '2026-08-18T00:00:00Z',
    ...overrides,
  }
}

describe('MonitorQuotaView', () => {
  it('renders nothing without a snapshot', () => {
    const wrapper = mount(MonitorQuotaView, { props: { snapshot: null } })
    expect(wrapper.find('[data-testid="monitor-quota-view"]').exists()).toBe(false)
    expect(wrapper.text()).toBe('')
  })

  it('maps tier window/label tokens through i18n and colors utilization', () => {
    const wrapper = mount(MonitorQuotaView, {
      props: {
        snapshot: makeSnapshot({
          tiers: [
            { window: '5h', used_percent: 42.4 },
            { window: '7d', label: 'pro', used_percent: 80 },
            { window: 'weekly', label: 'unknown-token', used_percent: 95 },
          ],
        }),
      },
    })

    const rows = wrapper.findAll('[data-testid="monitor-quota-view"] .flex.items-center')
    // tier 行只在 success 且有数据时渲染
    expect(rows.length).toBeGreaterThanOrEqual(3)
    const text = wrapper.text()
    // 已知 window token 走 i18n
    expect(text).toContain('monitorCommon.quota.windows.5h')
    expect(text).toContain('monitorCommon.quota.windows.7d')
    // 已知 label token 拼成 label/window
    expect(text).toContain('monitorCommon.quota.labels.pro/monitorCommon.quota.windows.7d')
    // 未知 label 原样透出（前向兼容）
    expect(text).toContain('unknown-token/monitorCommon.quota.windows.weekly')
    // 百分比取整
    expect(text).toContain('42%')
    expect(text).toContain('80%')
    expect(text).toContain('95%')

    const html = wrapper.html()
    // 阈值配色：≥90 红 / ≥75 黄 / 其余绿（与账号页 CNProviderQuotaCell 对齐）
    expect(html).toContain('bg-emerald-500')
    expect(html).toContain('bg-amber-500')
    expect(html).toContain('bg-red-500')
  })

  it('clamps the tier bar width into 0-100', () => {
    const wrapper = mount(MonitorQuotaView, {
      props: {
        snapshot: makeSnapshot({ tiers: [{ window: '5h', used_percent: 240 }] }),
      },
    })
    expect(wrapper.html()).toContain('width: 100%')
  })

  it('shows the plan level badge and multi-currency balances', () => {
    const wrapper = mount(MonitorQuotaView, {
      props: {
        snapshot: makeSnapshot({
          plan_level: 'Max20',
          balances: [
            { currency: 'CNY', balance: 12.5 },
            { currency: 'USD', balance: 0 },
          ],
        }),
      },
    })

    expect(wrapper.text()).toContain('Max20')
    expect(wrapper.text()).toContain('12.50 CNY')
    expect(wrapper.text()).toContain('0.00 USD')
    // 余额为 0 用红色警示
    expect(wrapper.html()).toContain('text-red-600')
  })

  it('falls back to the single balance + currency pair', () => {
    const wrapper = mount(MonitorQuotaView, {
      props: { snapshot: makeSnapshot({ balance: 3.2, currency: 'CNY' }) },
    })
    expect(wrapper.text()).toContain('3.20 CNY')
  })

  it('renders a truncated error state when the fetch failed', () => {
    const longError = 'x'.repeat(60)
    const wrapper = mount(MonitorQuotaView, {
      props: {
        snapshot: makeSnapshot({ success: false, error: longError }),
      },
    })

    const error = wrapper.get('[data-testid="monitor-quota-error"]')
    expect(error.text()).toBe(`${'x'.repeat(48)}…`)
    expect(error.attributes('title')).toBe(longError)
  })

  // 组级聚合摘要：accounts_total > 0 才渲染，单账号快照不受影响。
  it('omits the accounts summary for single-account snapshots', () => {
    const wrapper = mount(MonitorQuotaView, {
      props: { snapshot: makeSnapshot({ tiers: [{ window: '5h', used_percent: 10 }] }) },
    })
    expect(wrapper.find('[data-testid="monitor-quota-accounts"]').exists()).toBe(false)
  })

  it('summarises how many group accounts still have quota', () => {
    const wrapper = mount(MonitorQuotaView, {
      props: {
        snapshot: makeSnapshot({
          accounts_total: 5,
          accounts_healthy: 2,
          accounts_exhausted: 2,
        }),
      },
    })

    const summary = wrapper.get('[data-testid="monitor-quota-accounts"]')
    expect(summary.text()).toContain('monitorCommon.quota.accountsHealthy:{"healthy":2,"total":5}')
    expect(summary.text()).toContain('monitorCommon.quota.accountsExhausted:{"count":2}')
    // 未知 = total - healthy - exhausted，与「耗尽」分开展示。
    expect(summary.text()).toContain('monitorCommon.quota.accountsUnknown:{"count":1}')
    // 部分可用 → 琥珀色
    expect(summary.html()).toContain('text-amber-600')
  })

  it('colors the accounts summary red when no account has quota left', () => {
    const wrapper = mount(MonitorQuotaView, {
      props: {
        snapshot: makeSnapshot({
          accounts_total: 3,
          accounts_healthy: 0,
          accounts_exhausted: 3,
        }),
      },
    })

    const summary = wrapper.get('[data-testid="monitor-quota-accounts"]')
    expect(summary.html()).toContain('text-red-600')
    // 全部耗尽时不该再报「未知」。
    expect(summary.text()).not.toContain('accountsUnknown')
  })

  it('colors the accounts summary green when every account has quota', () => {
    const wrapper = mount(MonitorQuotaView, {
      props: {
        snapshot: makeSnapshot({
          accounts_total: 4,
          accounts_healthy: 4,
          accounts_exhausted: 0,
        }),
      },
    })

    const summary = wrapper.get('[data-testid="monitor-quota-accounts"]')
    expect(summary.html()).toContain('text-emerald-600')
    expect(summary.text()).not.toContain('accountsExhausted')
  })

  it('accounts-only mode keeps the count line and hides tiers, plan, and errors', () => {
    const wrapper = mount(MonitorQuotaView, {
      props: {
        accountsOnly: true,
        snapshot: makeSnapshot({
          success: false,
          error: 'internal fetch failed',
          plan_level: 'pro',
          tiers: [{ window: '5h', used_percent: 88 }],
          accounts_total: 8,
          accounts_healthy: 2,
          accounts_exhausted: 5,
        }),
      },
    })

    expect(wrapper.find('[data-testid="monitor-quota-accounts"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="monitor-quota-error"]').exists()).toBe(false)
    expect(wrapper.text()).not.toContain('pro')
    expect(wrapper.text()).not.toContain('88%')
    expect(wrapper.text()).not.toContain('internal fetch failed')
  })

  it('keeps failed snapshots from rendering tier rows', () => {
    const wrapper = mount(MonitorQuotaView, {
      props: {
        snapshot: makeSnapshot({
          success: false,
          tiers: [{ window: '5h', used_percent: 10 }],
        }),
      },
    })
    expect(wrapper.text()).not.toContain('10%')
  })
})
