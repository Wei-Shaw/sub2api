import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'

import UserEditModal from '../UserEditModal.vue'
import type { AdminUser } from '@/types'

const { updateUser, updateUserAttributeValues, showError, showSuccess } = vi.hoisted(() => ({
  updateUser: vi.fn(),
  updateUserAttributeValues: vi.fn(),
  showError: vi.fn(),
  showSuccess: vi.fn()
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    users: { update: updateUser },
    userAttributes: { updateUserAttributeValues }
  }
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({ showError, showSuccess })
}))

vi.mock('@/composables/useClipboard', () => ({
  useClipboard: () => ({ copyToClipboard: vi.fn() })
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({ t: (key: string) => key })
  }
})

const user: AdminUser = {
  id: 7,
  username: 'alice',
  email: 'alice@example.com',
  role: 'user',
  balance: 0,
  concurrency: 4,
  extra_concurrency: 2,
  status: 'active',
  allowed_groups: null,
  balance_notify_enabled: false,
  balance_notify_threshold: null,
  balance_notify_extra_emails: [],
  created_at: '2026-07-01T00:00:00Z',
  updated_at: '2026-07-01T00:00:00Z',
  notes: ''
}

function mountModal() {
  return mount(UserEditModal, {
    props: { show: true, user },
    global: {
      stubs: {
        BaseDialog: { template: '<div><slot /><slot name="footer" /></div>' },
        UserAttributeForm: true,
        Icon: true
      }
    }
  })
}

describe('UserEditModal concurrency allowances', () => {
  beforeEach(() => {
    updateUser.mockReset()
    updateUserAttributeValues.mockReset()
    showError.mockReset()
    showSuccess.mockReset()
    updateUser.mockResolvedValue(user)
  })

  it('shows and submits standard and extra concurrency separately', async () => {
    const wrapper = mountModal()
    const standardInput = wrapper.get('[data-testid="edit-user-standard-concurrency"]')
    const extraInput = wrapper.get('[data-testid="edit-user-extra-concurrency"]')

    expect((standardInput.element as HTMLInputElement).value).toBe('4')
    expect((extraInput.element as HTMLInputElement).value).toBe('2')

    await standardInput.setValue('5')
    await extraInput.setValue('3')
    await wrapper.get('#edit-user-form').trigger('submit')
    await flushPromises()

    expect(updateUser).toHaveBeenCalledWith(
      7,
      expect.objectContaining({ concurrency: 5, extra_concurrency: 3 })
    )
  })
})
