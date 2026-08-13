import { mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'
import OpenCodeGoUsageCell from '../OpenCodeGoUsageCell.vue'
import UsageProgressBar from '../UsageProgressBar.vue'
import type { Account, OpenCodeGoUsageState } from '@/types'

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string) => {
        const short: Record<string, string> = {
          'admin.accounts.opencodeGo.rollingShort': '5h',
          'admin.accounts.opencodeGo.weeklyShort': 'W',
          'admin.accounts.opencodeGo.monthlyShort': 'M',
          'admin.accounts.opencodeGo.unauthorized': 'unauthorized',
          'admin.accounts.opencodeGo.failed': 'failed',
          'admin.accounts.opencodeGo.ok': 'ok'
        }
        return short[key] ?? key
      }
    })
  }
})

const usageState = (overrides: Partial<OpenCodeGoUsageState> = {}): OpenCodeGoUsageState => ({
  account_id: 7,
  eligible: true,
  auto_refresh_enabled: false,
  snapshot: {
    status: 'ok',
    fetched_at: '2026-07-22T12:00:00Z',
    last_attempt_at: '2026-07-22T12:00:00Z',
    next_refresh_at: '2026-07-22T13:00:00Z',
    data: {
      rolling: { percent: 5.6, resets_at: '2026-07-23T03:00:00Z' },
      weekly: { percent: 14.2, resets_at: '2026-07-29T00:00:00Z' },
      monthly: { percent: 33.3, resets_at: '2026-08-01T00:00:00Z' }
    }
  },
  ...overrides
})

const account = (state = usageState()): Account => ({
  id: 7,
  name: 'opencode',
  platform: 'openai',
  type: 'apikey',
  opencode_go_usage: state,
  proxy_id: null,
  concurrency: 1,
  priority: 1,
  status: 'active',
  error_message: null,
  last_used_at: null,
  expires_at: null,
  auto_pause_on_expired: false,
  created_at: '2026-07-22T00:00:00Z',
  updated_at: '2026-07-22T00:00:00Z',
  schedulable: true,
  rate_limited_at: null,
  rate_limit_reset_at: null,
  overload_until: null,
  temp_unschedulable_until: null,
  temp_unschedulable_reason: null,
  session_window_start: null,
  session_window_end: null,
  session_window_status: null
})

describe('OpenCodeGoUsageCell', () => {
  it('renders rolling, weekly and monthly windows in a shrinkable mobile-safe cell', () => {
    const wrapper = mount(OpenCodeGoUsageCell, { props: { account: account() } })
    const cell = wrapper.get('[data-testid="opencode-go-usage-cell"]')
    expect(cell.classes()).toEqual(expect.arrayContaining(['min-w-0', 'max-w-full']))
    expect(cell.classes()).not.toContain('min-w-[12rem]')

    const bars = wrapper.findAllComponents(UsageProgressBar)
    expect(bars).toHaveLength(3)
    expect(bars[0].props()).toMatchObject({
      label: '5h',
      utilization: 5.6,
      resetsAt: '2026-07-23T03:00:00Z',
      color: 'indigo'
    })
    expect(bars[1].props()).toMatchObject({
      label: 'W',
      utilization: 14.2,
      resetsAt: '2026-07-29T00:00:00Z',
      color: 'emerald'
    })
    expect(bars[2].props()).toMatchObject({
      label: 'M',
      utilization: 33.3,
      resetsAt: '2026-08-01T00:00:00Z',
      color: 'amber'
    })

    expect(wrapper.find('[data-testid="opencode-go-status-badge"]').exists()).toBe(false)
    expect(wrapper.findAll('button')).toHaveLength(0)
  })

  it('shows a status badge when the snapshot status is not ok', () => {
    const wrapper = mount(OpenCodeGoUsageCell, {
      props: { account: account(usageState({ snapshot: { status: 'unauthorized', last_attempt_at: '2026-07-22T12:00:00Z', next_refresh_at: '2026-07-22T13:00:00Z' } })) }
    })
    const badge = wrapper.get('[data-testid="opencode-go-status-badge"]')
    expect(badge.text()).toBe('unauthorized')
    expect(badge.classes()).toEqual(expect.arrayContaining(['bg-amber-100', 'text-amber-700']))
  })

  it('shows a red badge for failed snapshots', () => {
    const wrapper = mount(OpenCodeGoUsageCell, {
      props: { account: account(usageState({ snapshot: { status: 'failed', last_attempt_at: '2026-07-22T12:00:00Z', next_refresh_at: '2026-07-22T13:00:00Z', last_error: 'http_error' } })) }
    })
    const badge = wrapper.get('[data-testid="opencode-go-status-badge"]')
    expect(badge.text()).toBe('failed')
    expect(badge.classes()).toEqual(expect.arrayContaining(['bg-red-100', 'text-red-700']))
  })

  it('renders a dash when the account is not eligible or has no state', () => {
    const wrapper = mount(OpenCodeGoUsageCell, { props: { account: account(usageState({ eligible: false })) } })
    expect(wrapper.find('[data-testid="opencode-go-usage-cell"]').exists()).toBe(false)
    expect(wrapper.text()).toBe('-')
  })

  it('reacts to an account snapshot update', async () => {
    const wrapper = mount(OpenCodeGoUsageCell, { props: { account: account() } })
    const next = usageState()
    next.snapshot!.data!.rolling!.percent = 43

    await wrapper.setProps({ account: account(next) })

    expect(wrapper.findAllComponents(UsageProgressBar)[0].props('utilization')).toBe(43)
  })
})
