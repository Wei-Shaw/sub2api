import { mount, flushPromises } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import ClaudeResetCreditsCell from '../ClaudeResetCreditsCell.vue'
import type { Account } from '@/types'
const { getCredits, redeemCredits } = vi.hoisted(() => ({ getCredits: vi.fn(), redeemCredits: vi.fn() }))
vi.mock('@/api/admin/claudeResetCredits', () => ({ getClaudeResetCredits: getCredits, redeemClaudeResetCredit: redeemCredits }))
vi.mock('@/composables/useStepUp', () => ({ useStepUp: () => ({ run: (fn: () => unknown) => fn() }), isStepUpCancelled: () => false, isStepUpBlocked: () => false }))
vi.mock('@/components/auth/TotpStepUpDialog.vue', () => ({ default: { template: '<div data-testid="step-up-dialog" />' } }))
vi.mock('@/components/common/ConfirmDialog.vue', () => ({ default: { props: ['show'], emits: ['confirm'], template: `<button v-if="show" data-testid="confirm" @click="$emit('confirm')">confirm</button>` } }))
vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))
const account = { id: 1, platform: 'anthropic', type: 'oauth' } as Account
const snapshot = { eligible: true, available_count: 1, credits: [], fetched_at: '2026-09-25T00:00:00Z' }
describe('Claude reset credit status', () => {
  beforeEach(() => { getCredits.mockReset(); redeemCredits.mockReset() })
  it('queries only on explicit request and renders fetched data', async () => {
    getCredits.mockResolvedValue(snapshot)
    const wrapper = mount(ClaudeResetCreditsCell, { props: { account } })
    expect(getCredits).not.toHaveBeenCalled()
    expect(wrapper.find('[data-testid="step-up-dialog"]').exists()).toBe(false)
    await wrapper.get('button').trigger('click')
    await flushPromises()
    expect(getCredits).toHaveBeenCalledWith(1)
    expect(wrapper.text()).toContain('claudeResetCredits.count')
  })
  it('hides setup tokens and discards responses after account changes', async () => {
    let resolve!: (value: typeof snapshot) => void
    getCredits.mockReturnValue(new Promise(r => { resolve = r }))
    const wrapper = mount(ClaudeResetCreditsCell, { props: { account } })
    await wrapper.get('button').trigger('click')
    await wrapper.setProps({ account: { ...account, id: 2, type: 'setup-token' } })
    resolve(snapshot)
    await flushPromises()
    expect(wrapper.find('button').exists()).toBe(false)
    expect(wrapper.text()).not.toContain('claudeResetCredits.count')
  })
  it('requires confirmation and reuses the operation on an ambiguous response', async () => {
    getCredits.mockResolvedValue({ ...snapshot, credits: [{ selection_token: 'grant-use-1', label: 'Credit', resets_left: 1, clears: ['weekly'], percent_used: { weekly: 100 }, blocking: [], redeemable: true }] })
    redeemCredits.mockRejectedValueOnce(new Error('timeout')).mockResolvedValueOnce({ outcome: 'reset', replayed: true })
    const wrapper = mount(ClaudeResetCreditsCell, { props: { account: { ...account, id: 42 } } })
    await wrapper.get('button').trigger('click')
    await flushPromises()
    await wrapper.findAll('button').find(b => b.text().endsWith('.redeem'))!.trigger('click')
    expect(redeemCredits).not.toHaveBeenCalled()
    expect(wrapper.find('[data-testid="step-up-dialog"]').exists()).toBe(true)
    await wrapper.get('[data-testid="confirm"]').trigger('click')
    await flushPromises()
    expect(wrapper.text()).toContain('outcomes.unknown')
    const first = redeemCredits.mock.calls[0]
    await wrapper.findAll('button').find(b => b.text().endsWith('.retry'))!.trigger('click')
    await flushPromises()
    expect(redeemCredits.mock.calls[1]).toEqual(first)
    expect(wrapper.text()).toContain('outcomes.reset')
  })

})
