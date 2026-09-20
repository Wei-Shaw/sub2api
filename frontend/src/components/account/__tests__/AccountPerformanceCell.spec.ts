import { mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'
import AccountPerformanceCell from '../AccountPerformanceCell.vue'
import type { AccountPerformanceStats } from '@/api/admin/accounts'
import en from '@/i18n/locales/en/admin/accounts'

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      locale: { value: 'en' },
      t: (key: string, params: Record<string, string | number> = {}) => {
        const messages: Record<string, string> = en.accounts.performance
        return (messages[key.replace('admin.accounts.performance.', '')] ?? key)
          .replace(/\{(\w+)\}/g, (_, name: string) => String(params[name]))
      }
    })
  }
})

const stats: AccountPerformanceStats = {
  request_count: 12,
  ttft_ms: 820,
  tps: 46.32,
  cache_rate: 0.725,
  ttft_samples: 8,
  tps_samples: 7,
  cache_samples: 12,
  last_request_at: '2026-09-16T02:59:00Z'
}

function mountCell(props: Partial<InstanceType<typeof AccountPerformanceCell>['$props']> = {}) {
  return mount(AccountPerformanceCell, { props })
}

describe('AccountPerformanceCell', () => {
  it('formats all three metrics and exposes independent sample counts and snapshot times', () => {
    const wrapper = mountCell({
      stats,
      windowStart: '2026-09-16T02:00:00Z',
      windowEnd: '2026-09-16T03:00:00Z'
    })
    expect(wrapper.text()).toContain('0.82 s')
    expect(wrapper.text()).toContain('46.3 tok/s')
    expect(wrapper.text()).toContain('72.5%')
    const details = wrapper.attributes('title')
    expect(details).toContain('TTFT valid samples: 8')
    expect(details).toContain('Output valid samples: 7')
    expect(details).toContain('Cache valid samples: 12')
    expect(details).toContain('Window:')
    expect(details).toContain('Latest record:')
    expect(details).toContain('not active probes')
    expect(details).toContain('Forced cache billing')
    wrapper.unmount()
  })

  it('distinguishes unknown metrics from genuine zero values', async () => {
    const wrapper = mountCell({ stats: { ...stats, ttft_ms: null, tps: null, cache_rate: null } })
    expect(wrapper.findAll('span.font-medium').map(value => value.text())).toEqual(['-', '-', '-'])
    await wrapper.setProps({ stats: { ...stats, ttft_ms: 0, cache_rate: 0 } })
    expect(wrapper.text()).toContain('0 s')
    expect(wrapper.text()).toContain('0%')
    wrapper.unmount()
  })

  it('shows no usage only for an empty window, not for ineligible requests', async () => {
    const empty = { ...stats, request_count: 0, ttft_ms: null, tps: null, cache_rate: null }
    const wrapper = mountCell({ stats: empty })
    expect(wrapper.text()).toContain('No usage in the last hour')
    await wrapper.setProps({ stats: { ...empty, request_count: 2 } })
    expect(wrapper.text()).not.toContain('No usage in the last hour')
    expect(wrapper.findAll('span.font-medium').map(value => value.text())).toEqual(['-', '-', '-'])
    wrapper.unmount()
  })

  it('warns about small nonzero samples without labelling the account unhealthy', () => {
    const wrapper = mountCell({ stats: { ...stats, tps_samples: 2 } })
    expect(wrapper.text()).toContain('Few samples')
    expect(wrapper.text()).toContain('46.3 tok/s')
    wrapper.unmount()
  })

  it('distinguishes initial loading, errors, and stale previously loaded statistics', async () => {
    const wrapper = mountCell({ loading: true })
    expect(wrapper.findAll('.animate-pulse')).toHaveLength(3)
    expect(wrapper.attributes('aria-busy')).toBe('true')
    await wrapper.setProps({ loading: false, error: true })
    expect(wrapper.text()).toContain('Statistics unavailable')
    expect(wrapper.text()).not.toContain('0%')
    await wrapper.setProps({ stats })
    expect(wrapper.text()).toContain('Refresh failed; showing old data')
    expect(wrapper.text()).toContain('72.5%')
    wrapper.unmount()
  })
})
