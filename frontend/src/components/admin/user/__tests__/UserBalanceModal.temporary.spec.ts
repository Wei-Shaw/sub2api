import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import type { AdminUser } from '@/types'

const updateBalance = vi.hoisted(() => vi.fn())
const setTemporaryBalance = vi.hoisted(() => vi.fn())
const showError = vi.hoisted(() => vi.fn())
const showSuccess = vi.hoisted(() => vi.fn())

vi.mock('@/api/admin', () => ({
  adminAPI: { users: { updateBalance, setTemporaryBalance } }
}))
vi.mock('@/stores/app', () => ({ useAppStore: () => ({ showError, showSuccess }) }))
vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))

const user: AdminUser = {
  id: 7, username: 'alice', email: 'alice@example.com', role: 'user', balance: 2,
  concurrency: 1, status: 'active', allowed_groups: null,
  balance_notify_enabled: false, balance_notify_threshold: null,
  balance_notify_extra_emails: [], notes: '',
  created_at: '2026-09-01T00:00:00Z', updated_at: '2026-09-01T00:00:00Z'
}

describe('UserBalanceModal temporary grant mode', () => {
  beforeEach(() => {
    updateBalance.mockReset()
    setTemporaryBalance.mockReset()
    showError.mockReset()
    showSuccess.mockReset()
    setTemporaryBalance.mockResolvedValue(user)
  })

  it('lets a deposit choose temporary balance and submits expiry metadata', async () => {
    const wrapper = mount((await import('../UserBalanceModal.vue')).default, {
      props: { show: true, user, operation: 'add' },
      global: {
        stubs: {
          BaseDialog: { props: ['show'], template: '<div v-if="show"><slot /><slot name="footer" /></div>' },
          Icon: true
        }
      }
    })

    await wrapper.get('[data-test="balance-type-temporary"]').trigger('click')
    await wrapper.get('[data-test="balance-amount"]').setValue('18')
    await wrapper.get('[data-test="temporary-expiry-tomorrow"]').trigger('click')
    await wrapper.get('#balance-form').trigger('submit')
    await flushPromises()

    expect(setTemporaryBalance).toHaveBeenCalledWith(7, expect.objectContaining({
      amount: 18,
      expires_at: expect.any(String)
    }))
    expect(updateBalance).not.toHaveBeenCalled()
  })

  it('surfaces plain interceptor errors from a failed temporary grant', async () => {
    setTemporaryBalance.mockRejectedValueOnce({ status: 400, message: 'expiry is required' })
    const wrapper = mount((await import('../UserBalanceModal.vue')).default, {
      props: { show: true, user, operation: 'add' },
      global: {
        stubs: {
          BaseDialog: { props: ['show'], template: '<div v-if="show"><slot /><slot name="footer" /></div>' },
          Icon: true
        }
      }
    })

    await wrapper.get('[data-test="balance-type-temporary"]').trigger('click')
    await wrapper.get('[data-test="balance-amount"]').setValue('4')
    await wrapper.get('[data-test="temporary-expiry-tomorrow"]').trigger('click')
    await wrapper.get('#balance-form').trigger('submit')
    await flushPromises()

    expect(showError).toHaveBeenCalledWith('expiry is required')
  })
})
