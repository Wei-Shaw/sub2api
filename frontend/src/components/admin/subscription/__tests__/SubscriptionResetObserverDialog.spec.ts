import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import SubscriptionResetObserverDialog from '../SubscriptionResetObserverDialog.vue'
import type { AccountListItem } from '@/types'
import type { SubscriptionResetAccountState, SubscriptionResetEvent, SubscriptionResetPolicy, SubscriptionResetStatus } from '@/types/subscriptionResetObserver'

const { getStatus, updatePolicy, listAccounts } = vi.hoisted(() => ({ getStatus: vi.fn(), updatePolicy: vi.fn(), listAccounts: vi.fn() }))
vi.mock('@/api/admin', () => ({ adminAPI: { groups: { getSubscriptionResetStatus: getStatus, updateSubscriptionResetPolicy: updatePolicy }, accounts: { list: listAccounts } } }))
vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  const { default: messages } = await import('@/i18n/locales/en')
  function lookup(key: string): unknown {
    return key.split('.').reduce<unknown>((message, part) => (message as Record<string, unknown>)?.[part], messages)
  }
  return { ...actual, useI18n: () => ({
    te: (key: string) => typeof lookup(key) === 'string',
    t: (key: string, values: Record<string, unknown> = {}) => {
      const message = lookup(key)
      return typeof message === 'string' ? message.replace(/\{(\w+)\}/g, (_, name: string) => String(values[name] ?? `{${name}}`)) : key
    }
  }) }
})

function policy(overrides: Partial<SubscriptionResetPolicy> = {}): SubscriptionResetPolicy {
  return { group_id: 1, mode: 'off', source: '7d', account_ids: [], quorum_percent: 80, aggregation_minutes: 10, reset_dimensions: ['daily', 'weekly', 'monthly'], allow_single_subject: false, allow_early_resets: false, version: 0, updated_at: '2026-09-12T08:00:00Z', ...overrides }
}
function state(overrides: Partial<SubscriptionResetStatus> = {}): SubscriptionResetStatus {
  return { policy: policy(), accounts: [], subject_count: 0, verified_subject_count: 0, ready: false, active_event: null, events: [], last_observed_at: null, observation_only: true, ...overrides }
}
function account(id: number, overrides: Partial<AccountListItem> = {}): AccountListItem {
  return { id, name: `Fixture Pro ${id}`, platform: 'openai', type: 'oauth', parent_account_id: null, quota_dimension: 'global', credentials: { plan_type: 'pro' }, group_ids: [1], status: 'active', ...overrides } as AccountListItem
}
function accountState(id: number, overrides: Partial<SubscriptionResetAccountState> = {}): SubscriptionResetAccountState {
  return { account_id: id, subject_key: `private-subject-${id}`, duplicate_of: null, last_observed_at: '2026-09-12T08:00:00Z', used_percent: 10, reset_at: '2026-09-19T08:00:00Z', window_minutes: 10080, plan_type: 'pro', state: 'observing', reason: 'baseline_ready', last_error: '', ...overrides }
}
function batch(): SubscriptionResetEvent {
  return { id: 'fixture-batch', group_id: 1, policy_version: 4, source: '7d', kind: 'natural_reset', status: 'pending', reason: 'quorum_not_met', opened_at: '2026-09-12T08:00:00Z', deadline_at: '2026-09-12T08:10:00Z', confirmed_at: null, updated_at: '2026-09-12T08:02:00Z', source_reset_at: '2026-09-12T08:00:00Z', reset_dimensions: ['daily', 'weekly', 'monthly'], denominator: 5, confirmed_count: 3, required_count: 4,
    members: [1, 2, 3, 4, 5].map(id => ({ subject_key: `private-subject-${id}`, account_ids: [id], confirmed: id <= 3, confirmed_at: id <= 3 ? '2026-09-12T08:01:00Z' : null, transition_id: `private-transition-${id}`, state: id <= 3 ? 'confirmed' : 'unknown', reason: id <= 3 ? 'natural_window_advanced' : 'stale_sample', confirmation_samples: 1 })) }
}
function dialog() {
  return mount(SubscriptionResetObserverDialog, { props: { show: true, groupId: 1, groupName: 'Fixture group' }, global: { stubs: { BaseDialog: { props: ['show'], template: '<div v-if="show"><slot /><slot name="footer" /></div>' }, LoadingSpinner: true } } })
}

describe('SubscriptionResetObserverDialog', () => {
  beforeEach(() => {
    getStatus.mockReset().mockResolvedValue(state())
    listAccounts.mockReset().mockResolvedValue({ items: [account(1), account(2)], pages: 1, total: 2 })
    updatePolicy.mockReset().mockImplementation(async (_id, input) => policy({ ...input, version: input.version + 1 }))
  })

  it('defaults to off, 7d, 80%, 10 minutes and all preview dimensions with early resets disabled', async () => {
    const wrapper = dialog()
    await flushPromises()
    expect(wrapper.findAll('[data-test="mode"] option').map(o => o.attributes('value'))).toEqual(['off', 'observe', 'auto'])
    expect((wrapper.get('[data-test="mode"]').element as HTMLSelectElement).value).toBe('off')
    expect((wrapper.get('[data-test="allow-early-resets"]').element as HTMLInputElement).checked).toBe(false)
    expect((wrapper.get('[data-test="quorum"]').element as HTMLInputElement).value).toBe('80')
    expect((wrapper.get('[data-test="aggregation"]').element as HTMLInputElement).value).toBe('10')
    for (const dimension of ['daily', 'weekly', 'monthly']) expect((wrapper.get(`[data-test="dimension-${dimension}"]`).element as HTMLInputElement).checked).toBe(true)
    expect(wrapper.text()).toContain('does not refill subscription quotas')
    expect(wrapper.get('[data-test="monthly-preview"]').text()).toContain('every upstream week')
    expect(updatePolicy).not.toHaveBeenCalled()
    wrapper.unmount()
  })

  it('loads every account page, excludes non-global/auth variants, and filters by plan without deselecting hidden choices', async () => {
    listAccounts.mockResolvedValueOnce({ items: [account(1), account(8, { parent_account_id: 1 }), account(9, { quota_dimension: 'agent' }), account(10, { credentials: { auth_mode: ' Agent_Identity ' } })], pages: 2 })
      .mockResolvedValueOnce({ items: [account(2, { name: 'Workspace beta', credentials: { plan_type: 'team' } }), account(11, { credentials: { openai_auth_mode: 'PersonalAccessToken' } }), account(12, { type: 'apikey' })], pages: 2 })
    const wrapper = dialog()
    await flushPromises()
    expect(listAccounts).toHaveBeenCalledWith(2, 100, { group: '1', platform: 'openai', type: 'oauth', lite: '1' }, { signal: expect.any(AbortSignal) })
    for (const id of [8, 9, 10, 11, 12]) expect(wrapper.find(`[data-test="account-${id}"]`).exists()).toBe(false)
    await wrapper.get('[data-test="account-1"]').setValue(true)
    await wrapper.get('[data-test="account-search"]').setValue('Team')
    expect(wrapper.find('[data-test="account-1"]').exists()).toBe(false)
    expect(wrapper.find('[data-test="account-2"]').exists()).toBe(true)
    expect(wrapper.text()).toContain('1 accounts selected')
    wrapper.unmount()
  })

  it('saves explicit IDs, dimensions and policy version, without a refill action', async () => {
    getStatus.mockResolvedValue(state({ policy: policy({ version: 4 }) }))
    const wrapper = dialog()
    await flushPromises()
    await wrapper.get('[data-test="mode"]').setValue('observe')
    await wrapper.get('[data-test="account-1"]').setValue(true)
    await wrapper.get('[data-test="account-2"]').setValue(true)
    await wrapper.get('[data-test="dimension-monthly"]').setValue(false)
    await wrapper.get('form').trigger('submit')
    await flushPromises()
    expect(updatePolicy).toHaveBeenCalledWith(1, { mode: 'observe', source: '7d', account_ids: [1, 2], quorum_percent: 80, aggregation_minutes: 10, reset_dimensions: ['daily', 'weekly'], allow_single_subject: false, allow_early_resets: false, version: 4 })
    expect(wrapper.text()).toContain('No subscription quota was reset')
    wrapper.unmount()
  })

  it('validates the minimum reference set but allows disabling with no references', async () => {
    const wrapper = dialog()
    await flushPromises()
    await wrapper.get('[data-test="mode"]').setValue('observe')
    await wrapper.get('[data-test="account-1"]').setValue(true)
    await wrapper.get('form').trigger('submit')
    expect(updatePolicy).not.toHaveBeenCalled()
    expect(wrapper.text()).toContain('at least 2 eligible reference accounts')
    await wrapper.get('[data-test="mode"]').setValue('off')
    await wrapper.get('[data-test="account-1"]').setValue(false)
    await wrapper.get('form').trigger('submit')
    await flushPromises()
    expect(updatePolicy).toHaveBeenCalledWith(1, expect.objectContaining({ mode: 'off', account_ids: [] }))
    wrapper.unmount()
  })

  it('keeps offline and unknown members in the server-provided frozen denominator and hides identity keys', async () => {
    const event = batch()
    getStatus.mockResolvedValue(state({ policy: policy({ mode: 'observe', account_ids: [1, 2, 3, 4, 5] }), subject_count: 5, verified_subject_count: 3, active_event: event, events: [event], accounts: [accountState(4, { state: 'unknown', reason: 'stale_sample' }), accountState(5, { state: 'unknown', last_error: 'query_failed', reason: 'unverified_subject' })] }))
    const wrapper = dialog()
    await flushPromises()
    await wrapper.get('[data-test="tab-observations"]').trigger('click')
    expect(wrapper.findAll('[data-test="event"]')).toHaveLength(1)
    expect(wrapper.get('[data-test="event"]').text()).toContain('3 / 5 subjects confirmed')
    expect(wrapper.get('[data-test="event"]').text()).toContain('Requires 4 subjects')
    expect(wrapper.text()).toContain('observation is too old')
    expect(wrapper.text()).toContain('upstream query failed')
    expect(wrapper.text()).not.toContain('private-subject')
    expect(wrapper.text()).not.toContain('private-transition')
    wrapper.unmount()
  })

  it('shows no-confirmation reasons for early drops and has no apply button', async () => {
    getStatus.mockResolvedValue(state({ events: [{ ...batch(), kind: 'early_drop', status: 'needs_review', reason: 'early_reset_unverified' }] }))
    const wrapper = dialog()
    await flushPromises()
    await wrapper.get('[data-test="tab-observations"]').trigger('click')
    expect(wrapper.text()).toContain('Usage fell early; this cannot confirm a natural reset')
    expect(wrapper.findAll('button').some(b => /apply|refill|reset quota/i.test(b.text()))).toBe(false)
    wrapper.unmount()
  })

  it('retains unavailable saved references and requires explicit deselection before re-enabling', async () => {
    getStatus.mockResolvedValue(state({ policy: policy({ mode: 'observe', account_ids: [1, 2, 99] }) }))
    const wrapper = dialog()
    await flushPromises()
    expect((wrapper.get('[data-test="missing-account-99"]').element as HTMLInputElement).checked).toBe(true)
    await wrapper.get('form').trigger('submit')
    expect(updatePolicy).not.toHaveBeenCalled()
    await wrapper.get('[data-test="missing-account-99"]').setValue(false)
    await wrapper.get('form').trigger('submit')
    await flushPromises()
    expect(updatePolicy).toHaveBeenCalledWith(1, expect.objectContaining({ account_ids: [1, 2] }))
    wrapper.unmount()
  })

  it('retries initial failures and preserves edits on a version conflict', async () => {
    getStatus.mockRejectedValueOnce(new Error('offline'))
    const wrapper = dialog()
    await flushPromises()
    expect(wrapper.text()).toContain('Could not load')
    await wrapper.get('[data-test="retry"]').trigger('click')
    await flushPromises()
    updatePolicy.mockRejectedValue({ status: 409, code: 'SUBSCRIPTION_RESET_POLICY_CONFLICT' })
    await wrapper.get('[data-test="quorum"]').setValue(90)
    await wrapper.get('form').trigger('submit')
    await flushPromises()
    expect(wrapper.text()).toContain('policy changed elsewhere')
    expect((wrapper.get('[data-test="quorum"]').element as HTMLInputElement).value).toBe('90')
    wrapper.unmount()
  })

  it('ignores a stale policy response when switching groups', async () => {
    let resolveFirst!: (value: SubscriptionResetStatus) => void
    getStatus.mockImplementationOnce(() => new Promise(resolve => { resolveFirst = resolve }))
      .mockResolvedValueOnce(state({ policy: policy({ group_id: 2, quorum_percent: 95, version: 8 }) }))
    const wrapper = dialog()
    await wrapper.setProps({ groupId: 2, groupName: 'Second group' })
    await flushPromises()
    resolveFirst(state({ policy: policy({ quorum_percent: 20 }) }))
    await flushPromises()
    expect((wrapper.get('[data-test="quorum"]').element as HTMLInputElement).value).toBe('95')
    await wrapper.get('form').trigger('submit')
    await flushPromises()
    expect(updatePolicy).toHaveBeenCalledWith(2, expect.objectContaining({ version: 8, quorum_percent: 95 }))
    wrapper.unmount()
  })

  it('requires explicit auto selection and explains monthly refills without applying historical review batches', async () => {
    getStatus.mockResolvedValue(state({ policy: policy({ account_ids: [1, 2], version: 4 }), events: [{ ...batch(), kind: 'early_drop', status: 'needs_review' }] }))
    const wrapper = dialog()
    await flushPromises()
    await wrapper.get('[data-test="mode"]').setValue('auto')
    expect(wrapper.text()).toContain('only future reset batches confirmed under this policy')
    expect(wrapper.text()).toContain('does not apply historical batches')
    expect(wrapper.text()).toContain('upgrade every instance and wait for in-flight requests and pending settlements to finish')
    expect(wrapper.text()).toContain('Turning following off retains cycle accounting')
    expect(wrapper.text()).toContain('effective execution time')
    expect(wrapper.text()).toContain('expired, revoked, suspended, and future subscriptions are not reactivated')
    expect(wrapper.get('[data-test="monthly-preview"]').text()).toContain('every upstream week')
    expect(wrapper.text()).not.toContain('No quota is refilled in observation mode')
    await wrapper.get('[data-test="allow-early-resets"]').setValue(true)
    expect(wrapper.text()).toContain('at least 20% usage to at most 5%')
    expect(wrapper.text()).toContain('at least 30 seconds apart')
    await wrapper.get('form').trigger('submit')
    await flushPromises()
    expect(updatePolicy).toHaveBeenCalledWith(1, expect.objectContaining({ mode: 'auto', allow_early_resets: true, version: 4, reset_dimensions: ['daily', 'weekly', 'monthly'] }))
    expect(wrapper.text()).toContain('A fresh baseline is required')
    await wrapper.get('[data-test="tab-observations"]').trigger('click')
    expect(wrapper.get('[data-test="event"]').text()).toContain('Needs review')
    expect(wrapper.find('[data-test="execution-result"]').exists()).toBe(false)
    expect(wrapper.findAll('button').some(b => /apply|refill|reset quota/i.test(b.text()))).toBe(false)
    wrapper.unmount()
  })

  it('validates reference accounts for auto mode too', async () => {
    const wrapper = dialog()
    await flushPromises()
    await wrapper.get('[data-test="mode"]').setValue('auto')
    await wrapper.get('form').trigger('submit')
    expect(updatePolicy).not.toHaveBeenCalled()
    expect(wrapper.text()).toContain('at least 2 eligible reference accounts')
    wrapper.unmount()
  })

  it('shows applied execution facts and zero affected subscriptions without claiming an expired event was applied', async () => {
    getStatus.mockResolvedValue(state({ policy: policy({ mode: 'auto' }), observation_only: false, events: [
      { ...batch(), status: 'applied', reason: 'quota_replenished', confirmed_kind: 'early', executed_at: '2026-09-12T08:05:00Z', affected_subscriptions: 0, group_revision: 7, members: [{ ...batch().members[0], old_used_percent: 45, new_used_percent: 2, confirmation_samples: 2 }] },
      { ...batch(), id: 'expired-batch', status: 'execution_expired', reason: 'confirmation_no_longer_fresh', confirmed_kind: 'natural' }
    ] }))
    const wrapper = dialog()
    await flushPromises()
    await wrapper.get('[data-test="tab-observations"]').trigger('click')
    expect(wrapper.findAll('[data-test="execution-result"]')).toHaveLength(1)
    expect(wrapper.get('[data-test="execution-result"]').text()).toContain('0 subscriptions affected')
    expect(wrapper.get('[data-test="execution-result"]').text()).toContain('Group quota revision: 7')
    expect(wrapper.text()).toContain('Confirmed source: Early bulk reset')
    expect(wrapper.text()).toContain('Usage 45.0% → 2.0% · 2 confirming samples')
    expect(wrapper.text()).toContain('not applied because its execution window expired')
    expect(wrapper.text()).toContain('The selected subscription quotas were refilled')
    expect(wrapper.text()).toContain('The confirmation is no longer fresh enough to apply')
    wrapper.unmount()
  })
})
