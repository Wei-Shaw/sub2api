import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import UpstreamBillingRateCell from '../UpstreamBillingRateCell.vue'
import type { Account } from '@/types'

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string, params?: Record<string, unknown>) =>
        params ? `${key}:${Object.values(params).join(',')}` : key
    })
  }
})

const makeAccount = (overrides: Partial<Account> = {}): Account => ({
  id: 1,
  name: 'upstream',
  platform: 'openai',
  type: 'apikey',
  proxy_id: null,
  concurrency: 1,
  priority: 1,
  status: 'active',
  error_message: null,
  last_used_at: null,
  expires_at: null,
  auto_pause_on_expired: false,
  created_at: '2026-07-13T00:00:00Z',
  updated_at: '2026-07-13T00:00:00Z',
  schedulable: true,
  rate_limited_at: null,
  rate_limit_reset_at: null,
  overload_until: null,
  temp_unschedulable_until: null,
  temp_unschedulable_reason: null,
  session_window_start: null,
  session_window_end: null,
  session_window_status: null,
  ...overrides
})

const billingData = {
  object: 'sub2api.key_billing' as const,
  schema_version: 1 as const,
  billing_scope: 'token' as const,
  group_rate_multiplier: 0.8,
  resolved_rate_multiplier: 0.6,
  peak_rate_enabled: true,
  peak_start: '09:00',
  peak_end: '18:00',
  peak_rate_multiplier: 1.5,
  applied_peak_multiplier: 1.5,
  effective_rate_multiplier: 0.9,
  timezone: 'Asia/Shanghai',
  observed_at: '2026-07-13T00:00:00Z'
}

describe('UpstreamBillingRateCell', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    vi.setSystemTime(new Date('2026-07-13T00:30:00Z'))
  })

  afterEach(() => {
    vi.useRealTimers()
  })

  it('recomputes the current effective rate and keeps the icon-only probe action', async () => {
    const wrapper = mount(UpstreamBillingRateCell, {
      props: {
        account: makeAccount({
          extra: {
            upstream_billing_probe_enabled: true,
            upstream_billing_probe: {
              status: 'ok',
              data: billingData,
              received_at: '2026-07-13T00:00:00Z',
              fresh_until: '2026-07-14T00:00:00Z',
              last_attempt_at: '2026-07-13T00:00:00Z',
              next_probe_at: '2026-07-13T00:30:00Z'
            }
          }
        }),
        intervalMinutes: 30,
        now: Date.now()
      }
    })

    expect(wrapper.text()).toContain('0.6x')
    await wrapper.setProps({ now: Date.parse('2026-07-13T01:00:00Z') })
    expect(wrapper.text()).toContain('0.9x')
    await wrapper.setProps({ now: Date.parse('2026-07-13T10:00:00Z') })
    expect(wrapper.text()).toContain('0.6x')
    expect(wrapper.text()).not.toContain('admin.accounts.upstreamBilling.latest')
    expect(wrapper.get('[data-testid="upstream-billing-probe"]').text()).toBe('')
    expect(wrapper.get('[data-testid="upstream-billing-probe"]').attributes('aria-label')).toBe(
      'admin.accounts.upstreamBilling.manualProbe'
    )
  })

  it('uses retained failed data only while it is still fresh', async () => {
    const account = makeAccount({
      extra: {
        upstream_billing_probe: {
          status: 'ok',
          data: billingData,
          received_at: '2026-07-12T22:00:00Z',
          fresh_until: '2026-07-12T23:00:00Z',
          last_attempt_at: '2026-07-12T22:00:00Z',
          next_probe_at: '2026-07-12T22:30:00Z'
        }
      }
    })
    const wrapper = mount(UpstreamBillingRateCell, { props: { account, intervalMinutes: 30, now: Date.now() } })
    expect(wrapper.text()).toContain('admin.accounts.upstreamBilling.stale')
    expect(wrapper.get('[data-testid="upstream-billing-rate"]').text()).toBe('-')

    await wrapper.setProps({
      account: makeAccount({
        extra: {
          upstream_billing_probe: {
            status: 'failed',
            data: billingData,
            received_at: '2026-07-13T00:00:00Z',
            fresh_until: '2026-07-13T01:00:00Z',
            last_attempt_at: '2026-07-13T00:00:00Z',
            next_probe_at: '2026-07-13T01:00:00Z',
            last_error: 'http_error'
          }
        }
      })
    })
    expect(wrapper.text()).toContain('0.6x')
    expect(wrapper.text()).toContain('admin.accounts.upstreamBilling.failed')

    await wrapper.setProps({ now: Date.parse('2026-07-13T01:00:00Z') })
    expect(wrapper.text()).toContain('0.9x')
    expect(wrapper.text()).not.toContain('admin.accounts.upstreamBilling.stale')

    await wrapper.setProps({ now: Date.parse('2026-07-13T01:00:00.001Z') })
    expect(wrapper.get('[data-testid="upstream-billing-rate"]').text()).toBe('-')
    expect(wrapper.text()).toContain('admin.accounts.upstreamBilling.stale')

    await wrapper.setProps({
      now: Date.now(),
      account: makeAccount({
        extra: {
          upstream_billing_probe: {
            status: 'failed',
            data: billingData,
            received_at: '2026-07-12T22:00:00Z',
            fresh_until: '2026-07-12T23:00:00Z',
            last_attempt_at: '2026-07-13T00:00:00Z',
            next_probe_at: '2026-07-13T01:00:00Z',
            last_error: 'http_error'
          }
        }
      })
    })
    expect(wrapper.get('[data-testid="upstream-billing-rate"]').text()).toBe('-')
    expect(wrapper.text()).toContain('admin.accounts.upstreamBilling.stale')
  })

  it('emits manual probe commands only for eligible accounts', async () => {
    const wrapper = mount(UpstreamBillingRateCell, {
      props: { account: makeAccount(), intervalMinutes: 30, now: Date.now() }
    })
    await wrapper.get('[data-testid="upstream-billing-probe"]').trigger('click')
    expect(wrapper.emitted('probe')).toHaveLength(1)

    await wrapper.setProps({ account: makeAccount({ type: 'oauth' }) })
    expect(wrapper.findAll('button')).toHaveLength(0)
    expect(wrapper.text()).toBe('-')
  })

  it('fails neutral for malformed data and timestamps', async () => {
    const malformedAccount = (
      dataOverrides: Partial<typeof billingData> = {},
      snapshotOverrides: Record<string, unknown> = {}
    ) => makeAccount({
      extra: {
        upstream_billing_probe: {
          status: 'ok',
          data: { ...billingData, ...dataOverrides },
          received_at: '2026-07-13T00:00:00Z',
          fresh_until: '2026-07-13T01:00:00Z',
          last_attempt_at: '2026-07-13T00:00:00Z',
          next_probe_at: '2026-07-13T01:00:00Z',
          ...snapshotOverrides
        }
      }
    })
    const wrapper = mount(UpstreamBillingRateCell, {
      props: {
        account: malformedAccount({
          resolved_rate_multiplier: -1,
          peak_rate_enabled: false,
          effective_rate_multiplier: -1
        }),
        intervalMinutes: 30,
        now: Date.now()
      }
    })

    expect(wrapper.get('[data-testid="upstream-billing-rate"]').text()).toBe('-')
    await wrapper.setProps({ account: malformedAccount({ billing_scope: 'request' as 'token' }) })
    expect(wrapper.get('[data-testid="upstream-billing-rate"]').text()).toBe('-')
    await wrapper.setProps({ account: malformedAccount({}, { received_at: 'not-a-time' }) })
    expect(wrapper.get('[data-testid="upstream-billing-rate"]').text()).toBe('-')
    await wrapper.setProps({ account: malformedAccount({}, { received_at: '2026-07-13T00:31:00Z' }) })
    expect(wrapper.get('[data-testid="upstream-billing-rate"]').text()).toBe('-')
    await wrapper.setProps({ account: malformedAccount({}, { fresh_until: '2026-07-12T23:59:00Z' }) })
    expect(wrapper.get('[data-testid="upstream-billing-rate"]').text()).toBe('-')

    await wrapper.setProps({
      account: makeAccount({
        extra: {
          upstream_billing_probe: {
            status: 'failed',
            last_attempt_at: '2026-07-13T00:00:00Z',
            next_probe_at: '2026-07-13T01:00:00Z',
            last_error: 'network_error'
          }
        }
      })
    })
    expect(wrapper.get('[data-testid="upstream-billing-rate"]').text()).toBe('-')
    expect(wrapper.text()).toContain('admin.accounts.upstreamBilling.failed')
    expect(wrapper.text()).not.toContain('admin.accounts.upstreamBilling.stale')
  })
})
