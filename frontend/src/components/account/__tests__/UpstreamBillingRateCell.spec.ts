import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import UpstreamBillingRateCell from '../UpstreamBillingRateCell.vue'
import HelpTooltip from '@/components/common/HelpTooltip.vue'
import Icon from '@/components/icons/Icon.vue'
import type { Account, UpstreamQuotaInfo, UpstreamQuotaQueryResult } from '@/types'

const { localeRef } = vi.hoisted(() => ({ localeRef: { value: 'zh' } }))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string, params?: Record<string, unknown>) =>
        params ? `${key}:${Object.values(params).join(',')}` : key,
      locale: localeRef
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

const makeQuotaResult = (overrides: Partial<UpstreamQuotaInfo> = {}): UpstreamQuotaQueryResult => ({
  account_id: 1,
  observed_at: '2026-07-13T00:30:00Z',
  quota: {
    provider: 'sub2api',
    mode: 'balance',
    unit: 'USD',
    remaining: 80,
    ...overrides
  }
})

describe('UpstreamBillingRateCell', () => {
  beforeEach(() => {
    localeRef.value = 'zh'
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

    const rateValue = wrapper.get('[data-testid="upstream-billing-rate"]')
    expect(rateValue.element.tagName).toBe('BUTTON')
    expect(rateValue.classes()).toContain('text-[11px]')
    expect(rateValue.classes()).toContain('text-sky-500')
    expect(rateValue.attributes('aria-label')).toBe('admin.accounts.upstreamBilling.rateValueDetails:0.6x')
    await wrapper.setProps({
      rateError: true,
      rateErrorAt: '2026-07-13T00:29:00Z'
    })
    expect(rateValue.text()).toBe('0.6x')
    expect(rateValue.classes()).toContain('text-[11px]')
    expect(rateValue.classes()).toContain('text-red-600')
    expect(wrapper.get('[data-testid="upstream-rate-row"]').text()).not.toContain(
      'admin.accounts.upstreamBilling.failed'
    )
    await rateValue.trigger('focusin')
    await flushPromises()
    const rateDetails = document.body.querySelectorAll('[data-testid="upstream-rate-details"]')
    const rateTooltip = rateDetails[rateDetails.length - 1]?.closest('[role="tooltip"]') as HTMLElement
    expect(rateTooltip.style.display).not.toBe('none')
    expect(rateTooltip.textContent).not.toContain('admin.accounts.upstreamBilling.sortUsesRate')
    const errorDetails = rateTooltip.querySelector('[data-testid="upstream-billing-error-details"]')
    expect(errorDetails?.textContent).toContain('admin.accounts.upstreamBilling.probeErrorTitle')
    expect(errorDetails?.textContent).toContain('admin.accounts.upstreamBilling.probeErrorDetail:')
    expect(errorDetails?.textContent).toContain('admin.accounts.upstreamBilling.probeFailedReason')
    expect(errorDetails?.parentElement?.lastElementChild).toBe(errorDetails)
    wrapper.unmount()
  })

  it('opens history only from a numeric rate', async () => {
    const wrapper = mount(UpstreamBillingRateCell, {
      props: {
        account: makeAccount({
          extra: {
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
        now: Date.now()
      }
    })

    await wrapper.get('[data-testid="upstream-billing-rate"]').trigger('click')
    expect(wrapper.emitted('open-history')).toHaveLength(1)

    await wrapper.get('[data-testid="upstream-billing-probe"]').trigger('click')
    expect(wrapper.emitted('probe')).toHaveLength(1)
    expect(wrapper.emitted('open-history')).toHaveLength(1)

    await wrapper.setProps({
      account: makeAccount({
        extra: {
          upstream_billing_probe: {
            status: 'unsupported',
            last_attempt_at: '2026-07-13T00:00:00Z',
            next_probe_at: '2026-07-13T00:30:00Z'
          }
        }
      })
    })
    expect(wrapper.get('[data-testid="upstream-billing-rate"]').element.tagName).toBe('SPAN')
    await wrapper.get('[data-testid="upstream-billing-rate"]').trigger('click')
    expect(wrapper.emitted('open-history')).toHaveLength(1)
  })

  it('renders stable rate and quota rows with independent accessible actions', async () => {
    const wrapper = mount(UpstreamBillingRateCell, {
      props: { account: makeAccount(), now: Date.now() }
    })

    const information = wrapper.get('[data-testid="upstream-information"]')
    const rateRow = wrapper.get('[data-testid="upstream-rate-row"]')
    const quotaRow = wrapper.get('[data-testid="upstream-quota-row"]')
    expect(information.classes()).toContain('w-fit')
    expect(information.classes()).toContain('gap-2')
    expect(information.classes()).not.toContain('gap-1')
    expect(information.classes()).not.toContain('border')
    expect(rateRow.classes()).toContain('bg-gray-100/70')
    expect(rateRow.classes()).toContain('gap-[10px]')
    expect(rateRow.classes()).not.toContain('border')
    expect(rateRow.classes()).toContain('w-fit')
    expect(rateRow.classes()).toContain('max-w-[13rem]')
    expect(rateRow.classes()).toContain('h-6')
    expect(rateRow.classes()).not.toContain('min-h-8')
    expect(rateRow.classes()).toContain('px-1')
    expect(rateRow.classes()).not.toContain('py-0.5')
    expect(rateRow.find('span').classes()).toContain('w-5')
    expect(rateRow.find('span').classes()).toContain('text-left')
    expect(quotaRow.classes()).toContain('bg-emerald-50/50')
    expect(quotaRow.classes()).toContain('gap-[10px]')
    expect(quotaRow.classes()).not.toContain('border')
    expect(quotaRow.classes()).toContain('w-fit')
    expect(quotaRow.classes()).toContain('max-w-[13rem]')
    expect(quotaRow.classes()).toContain('h-6')
    expect(quotaRow.classes()).not.toContain('min-h-8')
    expect(quotaRow.classes()).toContain('px-1')
    expect(quotaRow.classes()).not.toContain('py-0.5')
    expect(quotaRow.find('span').classes()).toContain('w-5')
    expect(quotaRow.find('span').classes()).toContain('text-left')
    expect(wrapper.find('[data-testid="upstream-rate-mark"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="upstream-quota-mark"]').exists()).toBe(false)
    expect(wrapper.get('[data-testid="upstream-billing-details"]').classes()).toContain('!ml-0')
    expect(wrapper.get('[data-testid="upstream-billing-details"]').classes()).toContain('w-[46px]')
    expect(wrapper.get('[data-testid="upstream-billing-details"]').classes()).toContain('shrink-0')
    expect(wrapper.get('[data-testid="upstream-billing-rate"]').classes()).toContain('text-[10px]')
    expect(wrapper.get('[data-testid="upstream-billing-rate"]').classes()).toContain('text-gray-600')
    expect(wrapper.get('[data-testid="upstream-billing-rate"]').classes()).toContain('text-left')
    expect(wrapper.get('[data-testid="upstream-quota-value"]').text()).toBe(
      'admin.accounts.upstreamBilling.notQueried'
    )
    expect(wrapper.get('[data-testid="upstream-quota-value"]').classes()).toContain('text-[10px]')
    expect(wrapper.get('[data-testid="upstream-quota-value"]').classes()).toContain('text-gray-600')
    expect(wrapper.get('[data-testid="upstream-quota-value"]').classes()).toContain('w-[46px]')
    expect(wrapper.get('[data-testid="upstream-quota-value"]').classes()).toContain('text-left')
    expect(wrapper.get('[data-testid="upstream-quota-value"]').classes()).toContain('shrink-0')

    const rateButton = wrapper.get('[data-testid="upstream-billing-probe"]')
    const quotaButton = wrapper.get('[data-testid="upstream-quota-query"]')
    expect(rateButton.attributes('aria-label')).toBe('admin.accounts.upstreamBilling.manualProbe')
    expect(rateButton.attributes('title')).toBe('admin.accounts.upstreamBilling.manualProbe')
    expect(rateButton.classes()).toContain('w-6')
    expect(rateButton.classes()).toContain('hover:bg-white/80')
    expect(rateButton.classes()).not.toContain('hover:bg-gray-100')
    expect(quotaButton.attributes('aria-label')).toBe('admin.accounts.upstreamBilling.queryQuota')
    expect(quotaButton.attributes('title')).toBe('admin.accounts.upstreamBilling.queryQuota')
    expect(quotaButton.classes()).toContain('w-6')
    expect(quotaButton.classes()).toContain('hover:bg-white/80')
    expect(quotaButton.classes()).not.toContain('hover:bg-gray-100')
    expect(quotaButton.getComponent(Icon).props('name')).toBe('search')

    await quotaButton.trigger('click')
    expect(wrapper.emitted('query-quota')).toHaveLength(1)
    expect(wrapper.emitted('probe')).toBeUndefined()

    await wrapper.setProps({ probing: true, quotaLoading: false })
    expect(rateButton.attributes('disabled')).toBeDefined()
    expect(quotaButton.attributes('disabled')).toBeUndefined()
    expect(rateButton.getComponent(Icon).props('name')).toBe('refresh')
    expect(rateButton.find('svg').classes()).toContain('animate-spin')
    expect(rateButton.classes()).toContain('text-primary-600')
    expect(quotaButton.find('svg').classes()).not.toContain('animate-spin')

    await wrapper.setProps({ probing: false, quotaLoading: true })
    expect(rateButton.attributes('disabled')).toBeUndefined()
    expect(quotaButton.attributes('disabled')).toBeDefined()
    expect(quotaButton.getComponent(Icon).props('name')).toBe('refresh')
    expect(quotaButton.find('svg').classes()).toContain('animate-spin')
    expect(wrapper.get('[data-testid="upstream-quota-value"]').classes()).toContain('w-[46px]')

    await wrapper.setProps({
      probing: false,
      quotaLoading: false,
      rateFeedback: 'success',
      quotaFeedback: 'error'
    })
    expect(rateButton.getComponent(Icon).props('name')).toBe('check')
    expect(rateButton.classes()).toContain('text-emerald-600')
    expect(quotaButton.getComponent(Icon).props('name')).toBe('x')
    expect(quotaButton.classes()).toContain('text-red-600')

    await wrapper.setProps({ rateFeedback: null, quotaFeedback: null })
    expect(rateButton.getComponent(Icon).props('name')).toBe('refresh')
    expect(quotaButton.getComponent(Icon).props('name')).toBe('search')
  })

  it('keeps wider metric tracks for English labels and states', () => {
    localeRef.value = 'en'
    const wrapper = mount(UpstreamBillingRateCell, {
      props: { account: makeAccount(), now: Date.now() }
    })

    expect(wrapper.get('[data-testid="upstream-rate-row"] > span').classes()).toContain('w-10')
    expect(wrapper.get('[data-testid="upstream-billing-details"]').classes()).toContain('w-16')
    expect(wrapper.get('[data-testid="upstream-quota-value"]').classes()).toContain('w-16')
  })

  it('formats scalar balances and summarizes subscriptions in one row', async () => {
    const wrapper = mount(UpstreamBillingRateCell, {
      props: {
        account: makeAccount(),
        now: Date.now(),
        quotaResult: makeQuotaResult({
          used: 20,
          total: 100
        })
      }
    })

    const quotaValue = () => wrapper.get('[data-testid="upstream-quota-value"]')
    expect(quotaValue().element.tagName).toBe('SPAN')
    expect(quotaValue().text()).toBe('$80.00')
    expect(quotaValue().classes()).toContain('text-[11px]')
    expect(quotaValue().classes()).toContain('text-emerald-600')
    expect(wrapper.findAllComponents(HelpTooltip)).toHaveLength(2)
    const quotaTooltip = wrapper.findAllComponents(HelpTooltip)
      .find(component => component.attributes('data-testid') === 'upstream-quota-details')
    expect(quotaTooltip?.props('content')).toBe('$80.00')
    expect(quotaTooltip?.props('openDelayMs')).toBe(500)
    expect(quotaTooltip?.props('tooltipClass')).toBe('!py-2')

    await wrapper.setProps({
      quotaResult: makeQuotaResult({ unit: 'CNY' })
    })
    expect(quotaValue().text()).toBe('¥80.00')

    await wrapper.setProps({
      quotaResult: makeQuotaResult({ unit: 'TOKENS', remaining: 2000 })
    })
    expect(quotaValue().text()).toBe('2,000 TOKENS')

    await wrapper.setProps({
      quotaResult: makeQuotaResult({ unit: undefined, remaining: 12.5 })
    })
    expect(quotaValue().text()).toBe('12.5')

    await wrapper.setProps({ quotaResult: makeQuotaResult({ remaining: 8.49895155 }) })
    expect(quotaValue().text()).toBe('$8.50')

    await wrapper.setProps({ quotaResult: makeQuotaResult({ remaining: 0.0045 }) })
    expect(quotaValue().text()).toBe('$0.0045')
    expect(quotaValue().classes()).toContain('text-emerald-600')

    await wrapper.setProps({ quotaResult: makeQuotaResult({ remaining: 0 }) })
    expect(quotaValue().text()).toBe('$0.00')
    expect(quotaValue().classes()).toContain('text-red-600')

    await wrapper.setProps({ quotaResult: makeQuotaResult({ remaining: -2.5 }) })
    expect(quotaValue().text()).toBe('-$2.50')
    expect(quotaValue().classes()).toContain('text-red-600')

    await wrapper.setProps({
      quotaResult: makeQuotaResult({
        mode: 'rate_limits',
        unit: undefined,
        remaining: undefined,
        windows: [{ name: '5h', used: 2, limit: 10, remaining: 8 }]
      })
    })
    expect(quotaValue().text()).toBe('admin.accounts.upstreamBilling.quotaModeRateLimits')
    expect(quotaValue().classes()).toContain('text-[10px]')

    await wrapper.setProps({
      quotaResult: makeQuotaResult({
        mode: 'subscription',
        remaining: undefined,
        subscription: {
          plan_name: 'Unlimited',
          unlimited: true,
          expires_at: '2026-08-01T00:00:00Z'
        }
      })
    })
    expect(wrapper.findAll('[data-testid="upstream-quota-row"]')).toHaveLength(1)
    expect(wrapper.get('[data-testid="upstream-quota-row"] > span').text())
      .toBe('admin.accounts.upstreamBilling.quotaLabel')
    expect(wrapper.get('[data-testid="upstream-quota-value"]').text())
      .toBe('admin.accounts.upstreamBilling.subscriptionDetails')
    expect(wrapper.findAll('[data-testid="upstream-quota-query"]')).toHaveLength(1)

    await wrapper.setProps({
      quotaResult: makeQuotaResult({
        remaining: 80,
        subscription: {
          plan_name: 'Pro',
          remaining: 8,
          expires_at: '2026-08-01T00:00:00Z',
          windows: [
            { name: 'daily', used: 2, limit: 10, remaining: 8 },
            { name: 'weekly', used: 20, limit: 20, remaining: 0 },
            { name: 'monthly', used: 102.5, limit: 100, remaining: -2.5 }
          ]
        }
      })
    })
    expect(wrapper.findAll('[data-testid="upstream-quota-row"]')).toHaveLength(1)
    expect(wrapper.get('[data-testid="upstream-quota-value"]').text())
      .toBe('admin.accounts.upstreamBilling.subscriptionDetails')
    expect(wrapper.findAllComponents(HelpTooltip)).toHaveLength(1)
    wrapper.unmount()
  })

  it('retains the last successful quota and exposes a later refresh error independently', async () => {
    const wrapper = mount(UpstreamBillingRateCell, {
      attachTo: document.body,
      props: {
        account: makeAccount({
          extra: {
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
        now: Date.now(),
        quotaResult: makeQuotaResult(),
        quotaError: 'upstream timeout'
      }
    })

    expect(wrapper.get('[data-testid="upstream-billing-rate"]').text()).toBe('0.6x')
    expect(wrapper.get('[data-testid="upstream-quota-value"]').text()).toBe('$80.00')
    expect(wrapper.get('[data-testid="upstream-quota-value"]').classes()).toContain('text-[11px]')
    expect(wrapper.get('[data-testid="upstream-quota-value"]').classes()).toContain('text-red-600')
    expect(wrapper.get('[data-testid="upstream-quota-row"]').text()).toContain(
      'admin.accounts.upstreamBilling.quotaFailed'
    )
    const quotaTooltip = wrapper.findAllComponents(HelpTooltip)
      .find(component => component.attributes('data-testid') === 'upstream-quota-details')
    expect(quotaTooltip?.props('content')).toBe('$80.00')
    expect(quotaTooltip?.props('openDelayMs')).toBe(500)

    await wrapper.setProps({
      account: makeAccount({
        extra: {
          upstream_billing_probe: {
            status: 'failed',
            last_attempt_at: '2026-07-13T00:00:00Z',
            next_probe_at: '2026-07-13T01:00:00Z',
            last_error: 'rate timeout'
          }
        }
      })
    })
    expect(wrapper.get('[data-testid="upstream-billing-rate"]').text()).toBe(
      'admin.accounts.upstreamBilling.failed'
    )
    expect(wrapper.get('[data-testid="upstream-billing-rate"]').classes()).toContain('text-red-600')
    expect(wrapper.get('[data-testid="upstream-quota-value"]').text()).toBe('$80.00')
    wrapper.unmount()
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
    const wrapper = mount(UpstreamBillingRateCell, {
      attachTo: document.body,
      props: { account, now: Date.now() }
    })
    expect(wrapper.get('[data-testid="upstream-billing-rate"]').text()).toBe('0.9x')
    expect(wrapper.get('[data-testid="upstream-billing-rate"]').classes()).toContain('text-amber-600')

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
    expect(wrapper.get('[data-testid="upstream-rate-row"]').text()).not.toContain(
      'admin.accounts.upstreamBilling.failed'
    )

    await wrapper.setProps({ now: Date.parse('2026-07-13T01:00:00Z') })
    expect(wrapper.text()).toContain('0.9x')
    expect(wrapper.text()).not.toContain('admin.accounts.upstreamBilling.stale')

    await wrapper.setProps({ now: Date.parse('2026-07-13T01:00:00.001Z') })
    expect(wrapper.get('[data-testid="upstream-billing-rate"]').text()).toBe('0.9x')
    expect(wrapper.get('[data-testid="upstream-billing-rate"]').classes()).toContain('text-red-600')

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
    expect(wrapper.get('[data-testid="upstream-billing-rate"]').text()).toBe('0.9x')
    expect(wrapper.get('[data-testid="upstream-billing-rate"]').classes()).toContain('text-red-600')

    await wrapper.get('[data-testid="upstream-billing-details"]').trigger('mouseenter')
    await flushPromises()
    const tooltips = document.body.querySelectorAll('[role="tooltip"]')
    const tooltip = tooltips[tooltips.length - 1] as HTMLElement
    expect(tooltip.querySelector('[data-testid="upstream-billing-stale-notice"]')).toBeNull()
    const errorDetails = tooltip.querySelector('[data-testid="upstream-billing-error-details"]')
    const errorGuidance = tooltip.querySelector('[data-testid="upstream-billing-error-guidance"]')
    expect(errorDetails?.children).toHaveLength(3)
    expect(errorDetails?.lastElementChild).toBe(errorGuidance)
    expect(errorGuidance?.textContent).toContain(
      'admin.accounts.upstreamBilling.manualProbeNotice:'
    )
    wrapper.unmount()
  })

  it('shows stale snapshot details, local next probe time, and the account probe state', async () => {
    const wrapper = mount(UpstreamBillingRateCell, {
      attachTo: document.body,
      props: {
        account: makeAccount({
          extra: {
            upstream_billing_probe_enabled: true,
            upstream_billing_probe: {
              status: 'ok',
              data: billingData,
              received_at: '2026-07-12T22:00:00Z',
              fresh_until: '2026-07-12T23:00:00Z',
              last_attempt_at: '2026-07-12T22:00:00Z',
              next_probe_at: '2026-07-13T01:00:00Z'
            }
          }
        }),
        now: Date.now()
      }
    })

    expect(wrapper.getComponent(HelpTooltip).props('widthClass')).toBe('w-max max-w-[calc(100vw-2rem)]')
    await wrapper.get('[data-testid="upstream-billing-details"]').trigger('mouseenter')
    await flushPromises()

    const rateDetails = document.body.querySelectorAll('[data-testid="upstream-rate-details"]')
    const tooltip = rateDetails[rateDetails.length - 1].closest('[role="tooltip"]') as HTMLElement
    expect(tooltip.textContent).toContain('admin.accounts.upstreamBilling.lastDetectedRate:0.9')
    expect(tooltip.textContent).toContain('admin.accounts.upstreamBilling.lastDetectedAt:')
    expect(tooltip.textContent).toContain('admin.accounts.upstreamBilling.elapsedSince:admin.accounts.upstreamBilling.hoursMinutesAgo:2,30')
    expect(tooltip.textContent).toContain('admin.accounts.upstreamBilling.nextProbeAt:')
    expect(wrapper.get('[data-testid="upstream-billing-rate"]').text()).toBe('0.9x')
    expect(wrapper.get('[data-testid="upstream-billing-rate"]').classes()).toContain('text-amber-600')
    const staleNotice = tooltip.querySelector('[data-testid="upstream-billing-stale-notice"]')
    expect(staleNotice?.textContent).toContain(
      'admin.accounts.upstreamBilling.staleCacheNotice:admin.accounts.upstreamBilling.hoursMinutesAgo:2,30'
    )
    expect(staleNotice?.className).toContain('text-amber-300')
    expect(tooltip.querySelector('[data-testid="upstream-billing-probe-state"] span')?.className).toContain('text-emerald-400')

    await wrapper.setProps({ rateError: true })
    expect(wrapper.get('[data-testid="upstream-billing-rate"]').classes()).toContain('text-red-600')
    expect(tooltip.querySelector('[data-testid="upstream-billing-stale-notice"]')).toBeNull()
    expect(tooltip.querySelector('[data-testid="upstream-billing-error-guidance"]')?.textContent).toContain(
      'admin.accounts.upstreamBilling.manualProbeNotice:'
    )

    await wrapper.setProps({
      rateError: false,
      account: makeAccount({
        extra: {
          upstream_billing_probe_enabled: false,
          upstream_billing_probe: {
            status: 'unsupported',
            last_attempt_at: '2026-07-13T00:00:00Z',
            next_probe_at: '2026-07-13T01:00:00Z'
          }
        }
      })
    })
    expect(tooltip.querySelector('[data-testid="upstream-billing-next-probe"]')).toBeNull()
    expect(tooltip.querySelector('[data-testid="upstream-billing-probe-state"] span')?.className).toContain('text-red-400')
    wrapper.unmount()
  })

  it('directs an overdue automatic probe to the row action after a page refresh', async () => {
    const wrapper = mount(UpstreamBillingRateCell, {
      attachTo: document.body,
      props: {
        account: makeAccount({
          extra: {
            upstream_billing_probe_enabled: true,
            upstream_billing_probe: {
              status: 'ok',
              data: billingData,
              received_at: '2026-07-12T22:00:00Z',
              fresh_until: '2026-07-12T23:00:00Z',
              last_attempt_at: '2026-07-12T22:00:00Z',
              next_probe_at: '2026-07-13T00:00:00Z'
            }
          }
        }),
        now: Date.now()
      }
    })

    await wrapper.get('[data-testid="upstream-billing-details"]').trigger('mouseenter')
    await flushPromises()

    const notices = document.body.querySelectorAll('[data-testid="upstream-billing-stale-notice"]')
    expect(notices[notices.length - 1]?.textContent).toContain(
      'admin.accounts.upstreamBilling.staleProbeOverdueNotice:'
    )
    wrapper.unmount()
  })

  it.each([
    ['account probe is disabled', false, true],
    ['global probe is disabled', true, false]
  ])('directs stale data to the row action when %s', async (_state, accountEnabled, globalEnabled) => {
    const wrapper = mount(UpstreamBillingRateCell, {
      attachTo: document.body,
      props: {
        account: makeAccount({
          extra: {
            upstream_billing_probe_enabled: accountEnabled,
            upstream_billing_probe: {
              status: 'ok',
              data: billingData,
              received_at: '2026-07-12T22:00:00Z',
              fresh_until: '2026-07-12T23:00:00Z',
              last_attempt_at: '2026-07-12T22:00:00Z',
              next_probe_at: '2026-07-13T00:00:00Z'
            }
          }
        }),
        globalProbeEnabled: globalEnabled,
        now: Date.now()
      }
    })

    await wrapper.get('[data-testid="upstream-billing-details"]').trigger('mouseenter')
    await flushPromises()

    const notices = document.body.querySelectorAll('[data-testid="upstream-billing-stale-notice"]')
    const notice = notices[notices.length - 1]
    expect(notice?.textContent).toContain('admin.accounts.upstreamBilling.manualProbeNotice:')
    expect(notice?.textContent).not.toContain('admin.accounts.upstreamBilling.staleCacheNotice:')
    expect(document.body.querySelector('[data-testid="upstream-billing-next-probe"]')).toBeNull()
    wrapper.unmount()
  })

  it('stacks the global-off state below the account state and hides it when globally enabled', async () => {
    const wrapper = mount(UpstreamBillingRateCell, {
      attachTo: document.body,
      props: {
        account: makeAccount({
          extra: {
            upstream_billing_probe_enabled: true,
            upstream_billing_probe: {
              status: 'unsupported',
              last_attempt_at: '2026-07-13T00:00:00Z',
              next_probe_at: '2026-07-13T01:00:00Z'
            }
          }
        }),
        globalProbeEnabled: false,
        now: Date.now()
      }
    })

    await wrapper.get('[data-testid="upstream-billing-details"]').trigger('mouseenter')
    await flushPromises()

    const rateDetails = document.body.querySelectorAll('[data-testid="upstream-rate-details"]')
    const tooltip = rateDetails[rateDetails.length - 1].closest('[role="tooltip"]') as HTMLElement
    const accountState = tooltip.querySelector('[data-testid="upstream-billing-probe-state"]')
    const globalState = tooltip.querySelector('[data-testid="upstream-billing-global-probe-state"]')
    expect(accountState?.querySelector('span')?.className).toContain('text-emerald-400')
    expect(globalState?.textContent).toContain('admin.accounts.upstreamBilling.globalProbeState')
    expect(globalState?.querySelector('span')?.className).toContain('text-red-400')
    expect(tooltip.querySelector('[data-testid="upstream-billing-next-probe"]')).toBeNull()

    await wrapper.setProps({ globalProbeEnabled: true })
    expect(tooltip.querySelector('[data-testid="upstream-billing-global-probe-state"]')).toBeNull()
    expect(tooltip.querySelector('[data-testid="upstream-billing-next-probe"]')).not.toBeNull()

    await wrapper.setProps({
      globalProbeEnabled: false,
      account: makeAccount({
        extra: { upstream_billing_probe_enabled: false }
      })
    })
    expect(accountState?.querySelector('span')?.className).toContain('text-red-400')
    expect(tooltip.querySelector('[data-testid="upstream-billing-global-probe-state"]')).not.toBeNull()
    expect(tooltip.querySelector('[data-testid="upstream-billing-next-probe"]')).toBeNull()
    wrapper.unmount()
  })

  it('emits manual probe commands only for eligible accounts', async () => {
    const wrapper = mount(UpstreamBillingRateCell, {
      props: { account: makeAccount(), now: Date.now() }
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
        now: Date.now()
      }
    })

    expect(wrapper.get('[data-testid="upstream-billing-rate"]').text()).toBe('-')
    await wrapper.setProps({ account: malformedAccount({ billing_scope: 'request' as 'token' }) })
    expect(wrapper.get('[data-testid="upstream-billing-rate"]').text()).toBe('-')
    await wrapper.setProps({ account: malformedAccount({}, { received_at: 'not-a-time' }) })
    expect(wrapper.get('[data-testid="upstream-billing-rate"]').text()).toBe('admin.accounts.upstreamBilling.stale')
    await wrapper.setProps({ account: malformedAccount({}, { received_at: '2026-07-13T00:31:00Z' }) })
    expect(wrapper.get('[data-testid="upstream-billing-rate"]').text()).toBe('0.6x')
    await wrapper.setProps({ account: malformedAccount({}, { received_at: '2026-07-13T00:36:00Z' }) })
    expect(wrapper.get('[data-testid="upstream-billing-rate"]').text()).toBe('admin.accounts.upstreamBilling.stale')
    await wrapper.setProps({ account: malformedAccount({}, { fresh_until: '2026-07-12T23:59:00Z' }) })
    expect(wrapper.get('[data-testid="upstream-billing-rate"]').text()).toBe('admin.accounts.upstreamBilling.stale')

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
    expect(wrapper.get('[data-testid="upstream-billing-rate"]').text()).toBe('admin.accounts.upstreamBilling.failed')
    expect(wrapper.get('[data-testid="upstream-billing-rate"]').classes()).toContain('text-red-600')
    expect(wrapper.text()).toContain('admin.accounts.upstreamBilling.failed')
    expect(wrapper.text()).not.toContain('admin.accounts.upstreamBilling.stale')
  })

  it('uses unsupported as the primary tooltip trigger without a dash', () => {
    const wrapper = mount(UpstreamBillingRateCell, {
      props: {
        account: makeAccount({
          extra: {
            upstream_billing_probe: {
              status: 'unsupported',
              last_attempt_at: '2026-07-13T00:00:00Z',
              next_probe_at: '2026-07-13T00:30:00Z',
              last_error: 'unsupported'
            }
          }
        }),
        now: Date.now()
      }
    })

    expect(wrapper.get('[data-testid="upstream-billing-rate"]').text()).toBe(
      'admin.accounts.upstreamBilling.unsupported'
    )
    expect(wrapper.get('[data-testid="upstream-billing-rate"]').classes()).toContain('text-gray-600')
    expect(wrapper.text()).not.toContain('-admin.accounts.upstreamBilling.unsupported')
  })

  it('keeps unsupported and failure reasons in the tooltip', async () => {
    const wrapper = mount(UpstreamBillingRateCell, {
      attachTo: document.body,
      props: {
        account: makeAccount({
          extra: {
            upstream_billing_probe: {
              status: 'unsupported',
              http_status: 404,
              last_attempt_at: '2026-07-13T00:00:00Z',
              next_probe_at: '2026-07-13T00:30:00Z'
            }
          }
        }),
        now: Date.now()
      }
    })

    expect(wrapper.get('[data-testid="upstream-billing-rate"]').text()).toBe(
      'admin.accounts.upstreamBilling.unsupported'
    )
    await wrapper.get('[data-testid="upstream-billing-details"]').trigger('mouseenter')
    await flushPromises()
    let tooltips = document.body.querySelectorAll('[role="tooltip"]')
    let tooltip = tooltips[tooltips.length - 1]
    expect(tooltip.textContent).toContain('admin.accounts.upstreamBilling.unsupportedNotFound')
    expect(wrapper.get('[data-testid="upstream-billing-rate"]').text()).not.toContain('unsupportedNotFound')

    await wrapper.setProps({
      account: makeAccount({
        extra: {
          upstream_billing_probe: {
            status: 'failed',
            http_status: 429,
            last_attempt_at: '2026-07-13T00:00:00Z',
            next_probe_at: '2026-07-13T00:30:00Z',
            last_error: 'http_error'
          }
        }
      })
    })
    await wrapper.get('[data-testid="upstream-billing-details"]').trigger('mouseenter')
    await flushPromises()
    tooltips = document.body.querySelectorAll('[role="tooltip"]')
    tooltip = tooltips[tooltips.length - 1]
    expect(tooltip.textContent).toContain('admin.accounts.upstreamBilling.probeRateLimited')
    const errorDetails = tooltip.querySelector('[data-testid="upstream-billing-error-details"]')
    const errorGuidance = tooltip.querySelector('[data-testid="upstream-billing-error-guidance"]')
    expect(errorDetails?.textContent).toContain('admin.accounts.upstreamBilling.probeErrorTitle')
    expect(errorDetails?.textContent).toContain('admin.accounts.upstreamBilling.probeErrorDetail:')
    expect(errorDetails?.children).toHaveLength(3)
    expect(errorDetails?.lastElementChild).toBe(errorGuidance)
    expect(errorGuidance?.textContent).toContain(
      'admin.accounts.upstreamBilling.failedNoRateNotice'
    )
    expect(errorDetails?.parentElement?.lastElementChild).toBe(errorDetails)
    expect(tooltip.querySelector('[data-testid="upstream-billing-probe-reason"]')).toBeNull()
    expect(wrapper.get('[data-testid="upstream-billing-rate"]').text()).toBe(
      'admin.accounts.upstreamBilling.failed'
    )
    wrapper.unmount()
  })

  it('maps known probe failures to safe tooltip reasons while preserving primary labels', async () => {
    const cases = [
      {
        snapshot: { status: 'unsupported' as const, http_status: 405, last_error: 'unsupported' },
        reason: 'admin.accounts.upstreamBilling.unsupportedMethod',
        primary: 'admin.accounts.upstreamBilling.unsupported'
      },
      {
        snapshot: { status: 'failed' as const, http_status: 401, last_error: 'http_error' },
        reason: 'admin.accounts.upstreamBilling.probeAuthFailed',
        primary: 'admin.accounts.upstreamBilling.failed'
      },
      {
        snapshot: { status: 'failed' as const, http_status: 500, last_error: 'http_error' },
        reason: 'admin.accounts.upstreamBilling.probeServerError',
        primary: 'admin.accounts.upstreamBilling.failed'
      },
      {
        snapshot: { status: 'failed' as const, last_error: 'invalid_response' },
        reason: 'admin.accounts.upstreamBilling.probeInvalidResponse',
        primary: 'admin.accounts.upstreamBilling.failed'
      },
      {
        snapshot: { status: 'failed' as const, last_error: 'request_failed' },
        reason: 'admin.accounts.upstreamBilling.probeRequestFailed',
        primary: 'admin.accounts.upstreamBilling.failed'
      },
      {
        snapshot: { status: 'failed' as const, last_error: 'upstream-secret-error' },
        reason: 'admin.accounts.upstreamBilling.probeFailedReason',
        primary: 'admin.accounts.upstreamBilling.failed'
      }
    ]

    for (const testCase of cases) {
      const wrapper = mount(UpstreamBillingRateCell, {
        attachTo: document.body,
        props: {
          account: makeAccount({
            extra: {
              upstream_billing_probe: {
                ...testCase.snapshot,
                last_attempt_at: '2026-07-13T00:00:00Z',
                next_probe_at: '2026-07-13T00:30:00Z'
              }
            }
          }),
          now: Date.now()
        }
      })

      expect(wrapper.get('[data-testid="upstream-billing-rate"]').text()).toBe(testCase.primary)
      await wrapper.get('[data-testid="upstream-billing-details"]').trigger('mouseenter')
      await flushPromises()
      const tooltips = document.body.querySelectorAll('[role="tooltip"]')
      const tooltip = tooltips[tooltips.length - 1]
      expect(tooltip.textContent).toContain(testCase.reason)
      expect(tooltip.textContent).not.toContain('upstream-secret-error')
      wrapper.unmount()
    }
  })
})
