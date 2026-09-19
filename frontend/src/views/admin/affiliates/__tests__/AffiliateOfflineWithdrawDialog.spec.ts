import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import AffiliateOfflineWithdrawDialog from '../AffiliateOfflineWithdrawDialog.vue'

const { lookupUsers, getUserOverview, withdrawUserQuota, showError, showSuccess } = vi.hoisted(() => ({
  lookupUsers: vi.fn(),
  getUserOverview: vi.fn(),
  withdrawUserQuota: vi.fn(),
  showError: vi.fn(),
  showSuccess: vi.fn(),
}))

vi.mock('@/api/admin/affiliates', () => ({
  affiliatesAPI: { lookupUsers, getUserOverview, withdrawUserQuota },
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({ showError, showSuccess }),
}))

vi.mock('vue-i18n', async (importOriginal) => {
  const actual = await importOriginal<typeof import('vue-i18n')>()
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string, params?: Record<string, unknown>) =>
        params ? `${key} ${JSON.stringify(params)}` : key,
    }),
  }
})

const BaseDialogStub = {
  props: ['show', 'title'],
  emits: ['close'],
  template: '<div v-if="show"><slot /><slot name="footer" /></div>',
}

function mountDialog() {
  return mount(AffiliateOfflineWithdrawDialog, {
    props: { show: true },
    global: { stubs: { BaseDialog: BaseDialogStub } },
  })
}

async function pickUser(wrapper: VueWrapper, availableQuota: number) {
  lookupUsers.mockResolvedValue([{ id: 42, email: 'inviter@example.com', username: 'inviter' }])
  getUserOverview.mockResolvedValue({
    user_id: 42,
    email: 'inviter@example.com',
    username: 'inviter',
    aff_code: 'INVITER',
    rebate_rate_percent: 20,
    invited_count: 3,
    rebated_invitee_count: 2,
    available_quota: availableQuota,
    history_quota: 50,
  })
  await wrapper.get('[data-test="withdraw-user-search"]').setValue('inviter')
  vi.advanceTimersByTime(300)
  await flushPromises()
  await wrapper.get('[data-test="withdraw-user-option"]').trigger('click')
  await flushPromises()
}

describe('AffiliateOfflineWithdrawDialog', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    vi.clearAllMocks()
  })

  afterEach(() => {
    vi.useRealTimers()
  })

  it('records the exact available quota as the withdrawal amount', async () => {
    const wrapper = mountDialog()
    await pickUser(wrapper, 12.34567891)

    expect(lookupUsers).toHaveBeenCalledWith('inviter')
    expect(getUserOverview).toHaveBeenCalledWith(42)
    expect(wrapper.get('[data-test="withdraw-available-quota"]').text()).toBe('$12.34567891')

    await wrapper.get('[data-test="withdraw-fill-all"]').trigger('click')
    expect((wrapper.get('[data-test="withdraw-amount"]').element as HTMLInputElement).value).toBe('12.34567891')

    withdrawUserQuota.mockResolvedValue({
      ledger_id: 9,
      user_id: 42,
      amount: 12.34567891,
      available_quota_after: 0,
      frozen_quota_after: 0,
      history_quota_after: 50,
    })
    await wrapper.get('[data-test="withdraw-submit"]').trigger('click')
    await flushPromises()

    expect(withdrawUserQuota).toHaveBeenCalledWith(42, { amount: 12.34567891 })
    expect(showSuccess).toHaveBeenCalledWith(
      'admin.affiliates.withdraw.success {"amount":"$12.34567891","remaining":"$0.00"}',
    )
    expect(wrapper.emitted('success')?.[0]?.[0]).toMatchObject({ ledger_id: 9 })
  })

  it('blocks amounts above the available quota', async () => {
    const wrapper = mountDialog()
    await pickUser(wrapper, 10)

    await wrapper.get('[data-test="withdraw-amount"]').setValue('10.00000001')

    expect(wrapper.get('[data-test="withdraw-amount-error"]').text()).toBe('admin.affiliates.withdraw.amountExceeds')
    expect(wrapper.get('[data-test="withdraw-submit"]').attributes('disabled')).toBeDefined()
    await wrapper.get('[data-test="withdraw-submit"]').trigger('click')
    await flushPromises()
    expect(withdrawUserQuota).not.toHaveBeenCalled()
  })

  it('treats a user without an affiliate profile as having no available quota', async () => {
    lookupUsers.mockResolvedValue([{ id: 7, email: 'fresh@example.com', username: 'fresh' }])
    getUserOverview.mockRejectedValue({ reason: 'USER_NOT_FOUND', message: 'user not found' })
    const wrapper = mountDialog()

    await wrapper.get('[data-test="withdraw-user-search"]').setValue('fresh')
    vi.advanceTimersByTime(300)
    await flushPromises()
    await wrapper.get('[data-test="withdraw-user-option"]').trigger('click')
    await flushPromises()

    expect(showError).not.toHaveBeenCalled()
    expect(wrapper.get('[data-test="withdraw-selected-user"]').text()).toContain('fresh@example.com')
    expect(wrapper.get('[data-test="withdraw-available-quota"]').text()).toBe('$0.00')
    expect(wrapper.get('[data-test="withdraw-fill-all"]').attributes('disabled')).toBeDefined()

    await wrapper.get('[data-test="withdraw-amount"]').setValue('1')
    expect(wrapper.get('[data-test="withdraw-amount-error"]').text()).toBe('admin.affiliates.withdraw.amountExceeds')
    expect(wrapper.get('[data-test="withdraw-submit"]').attributes('disabled')).toBeDefined()
  })

  it('shows the API error and reloads the available quota when recording fails', async () => {
    const wrapper = mountDialog()
    await pickUser(wrapper, 10)

    withdrawUserQuota.mockRejectedValue({ reason: 'AFFILIATE_QUOTA_INSUFFICIENT', message: 'insufficient' })
    await wrapper.get('[data-test="withdraw-amount"]').setValue('5')
    await wrapper.get('[data-test="withdraw-submit"]').trigger('click')
    await flushPromises()

    expect(showError).toHaveBeenCalledWith('admin.affiliates.errors.AFFILIATE_QUOTA_INSUFFICIENT {}')
    expect(getUserOverview).toHaveBeenCalledTimes(2)
    expect(wrapper.emitted('success')).toBeUndefined()
  })
})
