import { mount, flushPromises } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import ClaudeResetCreditsCell from '../ClaudeResetCreditsCell.vue'
import type { Account } from '@/types'
const getCredits = vi.hoisted(() => vi.fn())
const redeem = vi.hoisted(() => vi.fn())
vi.mock('@/api/admin/claudeResetCredits', () => ({ getClaudeResetCredits: getCredits, redeemClaudeResetCredit: redeem }))
const t = vi.hoisted(() => vi.fn((key: string, _params?: Record<string, unknown>) => key))
vi.mock('vue-i18n', () => ({ useI18n: () => ({ t }) }))
// Minimal stand-in exposing the dialog's show state and confirm/cancel events.
vi.mock('@/components/common/ConfirmDialog.vue', () => ({
  default: {
    props: { show: Boolean, title: String, message: String, confirmText: String, cancelText: String, danger: Boolean },
    emits: ['confirm', 'cancel'],
    template: `<div v-if="show" data-testid="confirm-dialog" :data-danger="String(danger)">{{ message }}
      <button data-testid="confirm-ok" @click="$emit('confirm')">ok</button>
      <button data-testid="confirm-cancel" @click="$emit('cancel')">cancel</button></div>`
  }
}))
const account = { id: 1, platform: 'anthropic', type: 'oauth' } as Account
const credit = {
  label: 'Launch reset', resets_left: 1,
  expires_at: '2026-10-22T16:00:00Z', clears: ['five_hour', 'seven_day'],
  percent_used: {}, blocking: [], use_requires_limit: false, redeemable: true
}
const snapshot = { eligible: true, available_count: 1, credits: [credit], fetched_at: '2026-09-25T00:00:00Z' }
const countButton = (wrapper: ReturnType<typeof mount>) => wrapper.find('[data-testid="claude-reset-count"]')
const redeemButton = (wrapper: ReturnType<typeof mount>) => wrapper.get('[data-testid="claude-reset-redeem"]')
async function queried(result: unknown = snapshot) {
  getCredits.mockResolvedValue(result)
  const wrapper = mount(ClaudeResetCreditsCell, { props: { account } })
  await countButton(wrapper).trigger('click')
  await flushPromises()
  return wrapper
}
async function confirmReset(wrapper: ReturnType<typeof mount>) {
  await redeemButton(wrapper).trigger('click')
  await wrapper.get('[data-testid="confirm-ok"]').trigger('click')
  await flushPromises()
}

describe('Claude reset credit status', () => {
  beforeEach(() => { getCredits.mockReset(); redeem.mockReset() })

  it('queries only on explicit request and shows count and expiry', async () => {
    getCredits.mockResolvedValue(snapshot)
    const wrapper = mount(ClaudeResetCreditsCell, { props: { account } })
    expect(getCredits).not.toHaveBeenCalled()
    expect(countButton(wrapper).text()).toBe('admin.accounts.claudeResetCredits.count')
    await countButton(wrapper).trigger('click')
    await flushPromises()
    expect(getCredits).toHaveBeenCalledWith(1)
    expect(countButton(wrapper).text()).toBe('admin.accounts.claudeResetCredits.count1')
    const expiry = wrapper.get('[data-testid="claude-reset-expiry"]')
    expect(expiry.attributes('title')).toContain('Launch reset')
  })

  it('counts held resets and prefers the next redeemable grant', async () => {
    const later = { ...credit, label: 'Later', expires_at: '2026-12-01T00:00:00Z', redeemable: false }
    const next = { ...credit, label: 'Next', expires_at: '2026-11-01T00:00:00Z', redeemable: true }
    getCredits.mockResolvedValue({ ...snapshot, available_count: 1, credits: [later, next] })
    const wrapper = mount(ClaudeResetCreditsCell, { props: { account } })
    await countButton(wrapper).trigger('click')
    await flushPromises()
    expect(countButton(wrapper).text()).toBe('admin.accounts.claudeResetCredits.count2')
    expect(wrapper.get('[data-testid="claude-reset-expiry"]').attributes('title')).toContain('Next')
    expect(wrapper.find('[data-testid="claude-reset-not-usable"]').exists()).toBe(false)
  })

  it('flags held resets that cannot be used yet', async () => {
    const waiting = { ...credit, use_requires_limit: true, redeemable: false }
    getCredits.mockResolvedValue({ ...snapshot, available_count: 0, credits: [waiting] })
    const wrapper = mount(ClaudeResetCreditsCell, { props: { account } })
    await countButton(wrapper).trigger('click')
    await flushPromises()
    expect(countButton(wrapper).text()).toBe('admin.accounts.claudeResetCredits.count1')
    expect(wrapper.find('[data-testid="claude-reset-not-usable"]').exists()).toBe(true)
  })

  it('disables reset until a query shows a redeemable credit', async () => {
    const wrapper = mount(ClaudeResetCreditsCell, { props: { account } })
    expect(redeemButton(wrapper).attributes('disabled')).toBeDefined()
    await redeemButton(wrapper).trigger('click')
    expect(wrapper.find('[data-testid="confirm-dialog"]').exists()).toBe(false)

    const waiting = await queried({ ...snapshot, available_count: 0, credits: [{ ...credit, redeemable: false }] })
    expect(redeemButton(waiting).attributes('disabled')).toBeDefined()

    let resolve!: (value: typeof snapshot) => void
    getCredits.mockReturnValue(new Promise(r => { resolve = r }))
    const loading = mount(ClaudeResetCreditsCell, { props: { account } })
    await countButton(loading).trigger('click')
    expect(redeemButton(loading).attributes('disabled')).toBeDefined()
    resolve(snapshot)
    await flushPromises()
    expect(redeemButton(loading).attributes('disabled')).toBeUndefined()
    expect(redeem).not.toHaveBeenCalled()
  })

  it('never redeems without confirmation, and the count button never redeems', async () => {
    const wrapper = await queried()
    await countButton(wrapper).trigger('click')
    await flushPromises()
    await redeemButton(wrapper).trigger('click')
    const dialog = wrapper.get('[data-testid="confirm-dialog"]')
    expect(dialog.attributes('data-danger')).toBe('true')
    expect(dialog.text()).toContain('admin.accounts.claudeResetCredits.confirmMessage')
    // Known windows render as readable labels; count is what remains after this use.
    expect(t).toHaveBeenCalledWith('admin.accounts.claudeResetCredits.confirmMessage', {
      windows: 'admin.accounts.claudeResetCredits.windows.fiveHour, admin.accounts.claudeResetCredits.windows.sevenDay',
      count: 0
    })
    await wrapper.get('[data-testid="confirm-cancel"]').trigger('click')
    await flushPromises()
    expect(wrapper.find('[data-testid="confirm-dialog"]').exists()).toBe(false)
    expect(redeem).not.toHaveBeenCalled()
  })

  it('redeems after confirmation, reports success, refreshes and notifies the parent', async () => {
    const wrapper = await queried()
    redeem.mockResolvedValue({ outcome: 'reset', cleared: ['five_hour'], replayed: false })
    getCredits.mockResolvedValue({ ...snapshot, available_count: 0, credits: [] })
    await confirmReset(wrapper)
    expect(redeem).toHaveBeenCalledTimes(1)
    expect(redeem.mock.calls[0][0]).toBe(1)
    expect(redeem.mock.calls[0][1]).toMatch(/^claude-reset-1-/)
    expect(wrapper.get('[data-testid="claude-reset-feedback"]').text()).toBe('admin.accounts.claudeResetCredits.outcome.reset')
    expect(getCredits).toHaveBeenCalledTimes(2)
    expect(wrapper.emitted('redeemed')).toHaveLength(1)
    expect(redeemButton(wrapper).attributes('disabled')).toBeDefined()
  })

  it('reuses the key when retrying a failed confirmation and rotates it after an answer', async () => {
    const wrapper = await queried()
    redeem.mockRejectedValueOnce({ status: 0, message: 'Network Error' })
    await confirmReset(wrapper)
    expect(wrapper.get('[data-testid="claude-reset-feedback"]').text()).toBe('Network Error')
    redeem.mockResolvedValueOnce({ outcome: 'not_limited', replayed: false })
    await confirmReset(wrapper)
    expect(redeem.mock.calls[1][1]).toBe(redeem.mock.calls[0][1])
    redeem.mockResolvedValueOnce({ outcome: 'not_limited', replayed: false })
    await confirmReset(wrapper)
    expect(redeem.mock.calls[2][1]).not.toBe(redeem.mock.calls[0][1])
  })

  it.each([
    ['already_used', 'alreadyUsed'],
    ['cooldown', 'cooldown'],
    ['not_limited', 'notLimited'],
    ['ineligible', 'ineligible'],
    ['unknown', 'unknown']
  ])('maps outcome %s to a clear message', async (outcome, key) => {
    const wrapper = await queried()
    redeem.mockResolvedValue({ outcome, replayed: false })
    await confirmReset(wrapper)
    expect(wrapper.get('[data-testid="claude-reset-feedback"]').text()).toBe(`admin.accounts.claudeResetCredits.outcome.${key}`)
    expect(getCredits).toHaveBeenCalledTimes(2)
  })

  it('maps server refusals to clear messages', async () => {
    const wrapper = await queried()
    redeem.mockRejectedValueOnce({ status: 409, reason: 'CLAUDE_RESET_UNRESOLVED', message: 'x' })
    await confirmReset(wrapper)
    expect(wrapper.get('[data-testid="claude-reset-feedback"]').text()).toBe('admin.accounts.claudeResetCredits.outcome.unknown')
    redeem.mockRejectedValueOnce({ status: 409, reason: 'CLAUDE_RESET_BUSY', message: 'x' })
    await confirmReset(wrapper)
    expect(wrapper.get('[data-testid="claude-reset-feedback"]').text()).toBe('admin.accounts.claudeResetCredits.outcome.busy')
  })

  it.each(['CLAUDE_RESET_BUSY', 'CLAUDE_RESET_NOT_AVAILABLE', 'CLAUDE_RESET_UNRESOLVED', 'CLAUDE_RESET_UPSTREAM_UNAVAILABLE'])(
    'rotates the key after pre-claim refusal %s',
    async reason => {
      const wrapper = await queried()
      redeem.mockRejectedValueOnce({ status: 409, reason, message: 'x' })
      await confirmReset(wrapper)
      redeem.mockResolvedValueOnce({ outcome: 'not_limited', replayed: false })
      await confirmReset(wrapper)
      expect(redeem.mock.calls[1][1]).not.toBe(redeem.mock.calls[0][1])
    }
  )

  it.each([
    ['IDEMPOTENCY_IN_PROGRESS', 'inProgress'],
    ['IDEMPOTENCY_RETRY_BACKOFF', 'retryBackoff'],
    ['CLAUDE_RESET_UPSTREAM_UNAVAILABLE', 'unavailable']
  ])('maps %s to a clear message', async (reason, key) => {
    const wrapper = await queried()
    redeem.mockRejectedValueOnce({ status: 409, reason, message: 'x' })
    await confirmReset(wrapper)
    expect(wrapper.get('[data-testid="claude-reset-feedback"]').text()).toBe(`admin.accounts.claudeResetCredits.outcome.${key}`)
  })

  it('reuses the key after an idempotency conflict', async () => {
    const wrapper = await queried()
    redeem.mockRejectedValueOnce({ status: 409, reason: 'IDEMPOTENCY_IN_PROGRESS', message: 'x' })
    await confirmReset(wrapper)
    redeem.mockResolvedValueOnce({ outcome: 'not_limited', replayed: false })
    await confirmReset(wrapper)
    expect(redeem.mock.calls[1][1]).toBe(redeem.mock.calls[0][1])
  })

  it('labels cleared windows and falls back to the raw key for unknown ones', async () => {
    const wrapper = await queried()
    redeem.mockResolvedValue({ outcome: 'reset', cleared: ['seven_day_overage_included', 'future_window'], replayed: false })
    await confirmReset(wrapper)
    expect(t).toHaveBeenCalledWith('admin.accounts.claudeResetCredits.outcome.reset', {
      windows: 'admin.accounts.claudeResetCredits.windows.sevenDayOverage, future_window'
    })
  })

  it('tells the operator to retry later after an explicit unavailable answer', async () => {
    const wrapper = await queried()
    redeem.mockResolvedValue({ outcome: 'unknown', reason: 'upstream_unavailable', replayed: false })
    await confirmReset(wrapper)
    expect(wrapper.get('[data-testid="claude-reset-feedback"]').text()).toBe('admin.accounts.claudeResetCredits.outcome.unavailable')
  })

  it('keeps slot content for setup tokens and discards responses after account changes', async () => {
    let resolve!: (value: typeof snapshot) => void
    getCredits.mockReturnValue(new Promise(r => { resolve = r }))
    const wrapper = mount(ClaudeResetCreditsCell, {
      props: { account },
      slots: { 'pre-actions': '<button data-testid="local-query">q</button>' }
    })
    await countButton(wrapper).trigger('click')
    await wrapper.setProps({ account: { ...account, id: 2, type: 'setup-token' } })
    resolve(snapshot)
    await flushPromises()
    expect(countButton(wrapper).exists()).toBe(false)
    expect(wrapper.find('[data-testid="local-query"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="claude-reset-expiry"]').exists()).toBe(false)
  })

  it('falls back to the earliest parsed expiry across offsets, invalid last', async () => {
    const a = { ...credit, label: 'Tokyo', expires_at: '2026-11-01T08:00:00+09:00', redeemable: false }
    const b = { ...credit, label: 'Chicago', expires_at: '2026-10-31T20:00:00-05:00', redeemable: false }
    const bad = { ...credit, label: 'Bad', expires_at: 'not-a-date', redeemable: false }
    const none = { ...credit, label: 'None', expires_at: undefined, redeemable: false }
    getCredits.mockResolvedValue({ ...snapshot, available_count: 0, credits: [none, bad, b, a] })
    const wrapper = mount(ClaudeResetCreditsCell, { props: { account } })
    await countButton(wrapper).trigger('click')
    await flushPromises()
    expect(wrapper.get('[data-testid="claude-reset-expiry"]').attributes('title')).toContain('Tokyo')
    expect(wrapper.get('[data-testid="claude-reset-expiry"]').attributes('title')).not.toContain('Chicago')
  })

  it('hides a cooldown hint that is already in the past', async () => {
    getCredits.mockResolvedValue({ ...snapshot, cooldown_until: '2000-01-01T00:00:00Z' })
    const wrapper = mount(ClaudeResetCreditsCell, { props: { account } })
    await countButton(wrapper).trigger('click')
    await flushPromises()
    expect(wrapper.find('[data-testid="claude-reset-cooldown"]').exists()).toBe(false)
  })

  it('discards in-flight responses when the same account changes type', async () => {
    let resolve!: (value: typeof snapshot) => void
    getCredits.mockReturnValue(new Promise(r => { resolve = r }))
    const wrapper = mount(ClaudeResetCreditsCell, { props: { account } })
    await countButton(wrapper).trigger('click')
    await wrapper.setProps({ account: { ...account, type: 'setup-token' } })
    await wrapper.setProps({ account: { ...account } })
    resolve(snapshot)
    await flushPromises()
    expect(countButton(wrapper).text()).toBe('admin.accounts.claudeResetCredits.count')
    expect(countButton(wrapper).attributes('disabled')).toBeUndefined()
    expect(wrapper.find('[data-testid="claude-reset-expiry"]').exists()).toBe(false)
  })
})
