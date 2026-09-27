import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import AccountStatusIndicator from '../AccountStatusIndicator.vue'
import type { Account } from '@/types'

vi.mock('vue-i18n', async (importOriginal) => ({
  ...(await importOriginal<typeof import('vue-i18n')>()),
  useI18n: () => ({ t: (key: string) => key })
}))
vi.mock('@/utils/format', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/utils/format')>()),
  formatCountdown: () => '1h'
}))

const future = new Date(Date.now() + 3600_000).toISOString()

function makeAccount(overrides: Partial<Account>): Account {
  return {
    id: 1,
    name: 'account',
    platform: 'antigravity',
    type: 'oauth',
    proxy_id: null,
    concurrency: 1,
    priority: 1,
    status: 'active',
    error_message: null,
    last_used_at: null,
    expires_at: null,
    auto_pause_on_expired: true,
    created_at: '2026-03-15T00:00:00Z',
    updated_at: '2026-03-15T00:00:00Z',
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
  }
}

function expectFocusableTooltips(wrapper: ReturnType<typeof mount>, count: number) {
  const tooltips = wrapper.findAll('[role="tooltip"]')
  expect(tooltips).toHaveLength(count)
  for (const tooltip of tooltips) {
    const id = tooltip.attributes('id')
    expect(id).toBeTruthy()
    // The trigger is a real button (tap/keyboard focusable) pointing at its tooltip.
    const trigger = wrapper.get(`button[aria-describedby="${id}"]`)
    expect(trigger.attributes('type')).toBe('button')
    // Focus inside the trigger's group reveals the tooltip, not only hover.
    expect(tooltip.classes().some((c) => c.startsWith('group-focus-within'))).toBe(true)
  }
  return tooltips.map((tooltip) => tooltip.attributes('id'))
}

describe('AccountStatusIndicator tooltips', () => {
  it('opens the error-message tooltip from a labelled, focusable button', () => {
    const wrapper = mount(AccountStatusIndicator, {
      props: { account: makeAccount({ status: 'error', error_message: 'invalid_grant: token revoked' }) }
    })

    expectFocusableTooltips(wrapper, 1)
    const trigger = wrapper.get('button[aria-describedby]')
    expect(trigger.attributes('aria-label')).toBe('admin.accounts.status.error')
    expect(wrapper.get('[role="tooltip"]').text()).toContain('invalid_grant: token revoked')
  })

  it('makes 429, 529 and model rate-limit tooltips focusable with ids unique per instance', () => {
    const account = makeAccount({
      rate_limit_reset_at: future,
      overload_until: future,
      extra: { model_rate_limits: { 'claude-sonnet-5': { rate_limited_at: future, rate_limit_reset_at: future } } }
    })
    // Two rows of the same table live in one app, so their tooltip ids must not collide.
    const wrapper = mount({
      components: { AccountStatusIndicator },
      setup: () => ({ account }),
      template: '<div><AccountStatusIndicator :account="account" /><AccountStatusIndicator :account="account" /></div>'
    })
    const ids = expectFocusableTooltips(wrapper, 6)

    expect(new Set(ids).size).toBe(6)
  })
})
