import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import OpenAIReferralCell from '../OpenAIReferralCell.vue'
import type { Account } from '@/types'
import type { OpenAIReferralEligibility, OpenAIReferralRefreshResult } from '@/types/openaiReferrals'
import { refreshOpenAIReferrals, sendOpenAIReferralInvite } from '@/api/admin/accounts'

vi.mock('@/api/admin/accounts', () => ({ refreshOpenAIReferrals: vi.fn(), sendOpenAIReferralInvite: vi.fn() }))
vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string, params?: Record<string, unknown>) => params ? `${key}:${Object.values(params).join(',')}` : key }) }))

const eligibility: OpenAIReferralEligibility = {
  should_show: true, remaining_send_capacity: 3, remaining_reward_capacity: 2,
  requires_explicit_confirmation: true, program_id: 'codex_referral_consumer', available_invites: 2,
  fetched_at: 1770000000, title: 'Test offer', rules: ['Test eligibility rule'],
}
const account = (overrides: Partial<Account> = {}) => ({
  id: 1, name: 'Test account', platform: 'openai', type: 'oauth',
  ...overrides,
}) as Account
const mountCell = (value = account()) => mount(OpenAIReferralCell, {
  props: { account: value }, global: { stubs: { teleport: true } },
})
const open = async (wrapper: ReturnType<typeof mountCell>) => {
  await wrapper.get('[data-testid="referral-open"]').trigger('click')
  await flushPromises()
}
const completeForm = async (wrapper: ReturnType<typeof mountCell>) => {
  await wrapper.get('[data-testid="referral-email"]').setValue('friend@example.com')
  await wrapper.get('[data-testid="referral-consent"]').setValue(true)
}

beforeEach(() => {
  vi.mocked(refreshOpenAIReferrals).mockReset().mockResolvedValue({ eligibility: { ...eligibility }, cache_persisted: true })
  vi.mocked(sendOpenAIReferralInvite).mockReset()
})

describe('OpenAIReferralCell', () => {
  it('shows cached capacity without sending requests, then refreshes when opening the form', async () => {
    const wrapper = mountCell(account({ extra: { codex_referral_snapshot: eligibility } }))
    expect(wrapper.get('[data-testid="referral-count"]').text()).toContain('2')
    expect(refreshOpenAIReferrals).not.toHaveBeenCalled()
    await open(wrapper)
    expect(refreshOpenAIReferrals).toHaveBeenCalledWith(1)
    expect(wrapper.text()).toContain('Test account')
    expect(wrapper.text()).toContain('Test eligibility rule')
    expect(wrapper.get('[data-testid="referral-send"]').attributes('disabled')).toBeDefined()
    expect(sendOpenAIReferralInvite).not.toHaveBeenCalled()
    wrapper.unmount()
  })

  it('requires a valid email and consent, and clears consent when the recipient changes', async () => {
    const wrapper = mountCell()
    await open(wrapper)
    await wrapper.get('[data-testid="referral-email"]').setValue('invalid')
    await wrapper.get('[data-testid="referral-consent"]').setValue(true)
    await wrapper.get('form').trigger('submit')
    expect(sendOpenAIReferralInvite).not.toHaveBeenCalled()
    await wrapper.get('[data-testid="referral-email"]').setValue('friend@example.com')
    expect((wrapper.get('[data-testid="referral-consent"]').element as HTMLInputElement).checked).toBe(false)
    expect(wrapper.get('[data-testid="referral-send"]').attributes('disabled')).toBeDefined()
    await wrapper.get('[data-testid="referral-consent"]').setValue(true)
    expect(wrapper.get('[data-testid="referral-send"]').attributes('disabled')).toBeUndefined()
    wrapper.unmount()
  })

  it.each([0, null])('blocks sends when remaining capacity is %s', async (capacity) => {
    vi.mocked(refreshOpenAIReferrals).mockResolvedValue({ eligibility: { ...eligibility, available_invites: capacity }, cache_persisted: true })
    const wrapper = mountCell()
    await open(wrapper)
    await completeForm(wrapper)
    await wrapper.get('form').trigger('submit')
    expect(sendOpenAIReferralInvite).not.toHaveBeenCalled()
    expect(wrapper.text()).toContain('admin.accounts.openaiReferral.unavailable')
    wrapper.unmount()
  })

  it('sends once and displays the recipient and refreshed count', async () => {
    vi.mocked(sendOpenAIReferralInvite).mockResolvedValue({
      sent: true, email: 'friend@example.com', eligibility: { ...eligibility, available_invites: 1 }, cache_persisted: true, refresh_failed: false,
    })
    const wrapper = mountCell()
    await open(wrapper)
    await completeForm(wrapper)
    await wrapper.get('form').trigger('submit')
    await wrapper.get('form').trigger('submit')
    await flushPromises()
    expect(sendOpenAIReferralInvite).toHaveBeenCalledTimes(1)
    expect(sendOpenAIReferralInvite).toHaveBeenCalledWith(1, { email: 'friend@example.com', program_id: 'codex_referral_consumer', confirmed: true })
    expect(wrapper.text()).toContain('admin.accounts.openaiReferral.sent:friend@example.com')
    expect(wrapper.get('[data-testid="referral-count"]').text()).toContain('1')
    expect((wrapper.get('[data-testid="referral-email"]').element as HTMLInputElement).value).toBe('')
    wrapper.unmount()
  })

  it('keeps send success when the capacity refresh fails and disables another send', async () => {
    vi.mocked(sendOpenAIReferralInvite).mockResolvedValue({ sent: true, email: 'friend@example.com', eligibility: null, cache_persisted: true, refresh_failed: true })
    const wrapper = mountCell()
    await open(wrapper)
    await completeForm(wrapper)
    await wrapper.get('form').trigger('submit')
    await flushPromises()
    expect(wrapper.text()).toContain('admin.accounts.openaiReferral.sent:friend@example.com')
    expect(wrapper.text()).toContain('admin.accounts.openaiReferral.refreshFailed')
    expect(wrapper.get('[data-testid="referral-count"]').text()).toContain('—')
    await completeForm(wrapper)
    expect(wrapper.get('[data-testid="referral-send"]').attributes('disabled')).toBeDefined()
    wrapper.unmount()
  })

  it('does not automatically retry an uncertain send', async () => {
    vi.mocked(sendOpenAIReferralInvite).mockRejectedValue({ reason: 'OPENAI_REFERRAL_SEND_UNKNOWN' })
    const wrapper = mountCell()
    await open(wrapper)
    await completeForm(wrapper)
    await wrapper.get('form').trigger('submit')
    await flushPromises()
    expect(wrapper.text()).toContain('admin.accounts.openaiReferral.sendUnknown')
    expect(sendOpenAIReferralInvite).toHaveBeenCalledTimes(1)
    expect(wrapper.get('[data-testid="referral-send"]').attributes('disabled')).toBeDefined()
    wrapper.unmount()
  })

  it('prevents sending from a shadow account', () => {
    const wrapper = mountCell(account({ parent_account_id: 100 }))
    expect(wrapper.get('[data-testid="referral-open"]').attributes('disabled')).toBeDefined()
    expect(wrapper.get('[data-testid="referral-open"]').attributes('title')).toContain('shadowHint')
    wrapper.unmount()
  })

  it('does not apply a pending query to a different account', async () => {
    let resolve!: (value: OpenAIReferralRefreshResult) => void
    vi.mocked(refreshOpenAIReferrals).mockReturnValue(new Promise((done) => { resolve = done }))
    const wrapper = mountCell()
    await wrapper.get('[data-testid="referral-count"]').trigger('click')
    await wrapper.setProps({ account: account({ id: 2 }) })
    resolve({ eligibility, cache_persisted: true })
    await flushPromises()
    expect(wrapper.get('[data-testid="referral-count"]').text()).toContain('—')
    wrapper.unmount()
  })
})
