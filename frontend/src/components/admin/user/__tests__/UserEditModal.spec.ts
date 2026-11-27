import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'

import UserEditModal from '../UserEditModal.vue'
import type { AdminUser } from '@/types'

const { update, updateUserAttributeValues, showSuccess, showError } = vi.hoisted(() => ({
  update: vi.fn(),
  updateUserAttributeValues: vi.fn(),
  showSuccess: vi.fn(),
  showError: vi.fn()
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    users: { update },
    userAttributes: { updateUserAttributeValues }
  }
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({ showSuccess, showError })
}))

vi.mock('@/composables/useClipboard', () => ({
  useClipboard: () => ({ copyToClipboard: vi.fn() })
}))

// useStepUp pulls in the API client, which needs the real i18n instance.
vi.mock('vue-i18n', async (importOriginal) => ({
  ...(await importOriginal<typeof import('vue-i18n')>()),
  useI18n: () => ({
    t: (key: string, params?: Record<string, unknown>) =>
      params ? `${key}:${JSON.stringify(params)}` : key
  })
}))

const mountModal = (concurrency: number, override: Partial<Pick<AdminUser, 'role' | 'moderation_penalty_scope' | 'moderation_key_lock_minutes'>> = {}) => mount(UserEditModal, {
  props: {
    show: true,
    user: { id: 7, email: 'user@example.test', username: 'user', notes: '', role: 'user', concurrency, rpm_limit: 0, ...override } as AdminUser
  },
  global: {
    stubs: {
      BaseDialog: {
        props: ['show', 'title'],
        template: '<div v-if="show"><slot /><slot name="footer" /></div>'
      },
      Select: true,
      Icon: true,
      UserAttributeForm: true,
      TotpStepUpDialog: true
    }
  }
})

describe('UserEditModal', () => {
  beforeEach(() => {
    update.mockReset()
    updateUserAttributeValues.mockReset()
    showSuccess.mockReset()
    showError.mockReset()
    update.mockResolvedValue({})
  })

  it.each([undefined, 'user', 'api_key'] as const)('preserves %s scope when saving other user details', async (scope) => {
    const wrapper = mountModal(3, { moderation_penalty_scope: scope })
    const selected = wrapper.get<HTMLInputElement>(`input[name="moderation-scope"][value="${scope ?? 'user'}"]`)
    expect(selected.element.checked).toBe(true)
    await wrapper.get('form').trigger('submit')
    await flushPromises()
    expect(update).toHaveBeenCalledOnce()
    expect(update.mock.calls[0][1]).not.toHaveProperty('moderation_penalty_scope')
  })

  it.each([['user', 'api_key'], ['api_key', 'user']] as const)('sends the selected scope when changing from %s to %s', async (current, next) => {
    const wrapper = mountModal(3, { moderation_penalty_scope: current })
    await wrapper.get(`input[name="moderation-scope"][value="${next}"]`).setValue()
    expect(wrapper.get('[role="status"]').text()).toContain('admin.users.penaltyChangeHint')
    await wrapper.get('form').trigger('submit')
    await flushPromises()
    expect(update).toHaveBeenCalledWith(7, expect.objectContaining({ moderation_penalty_scope: next }))
  })

  it('defaults to an indefinite per-key lock and suggests 10 minutes when timed is selected', async () => {
    const wrapper = mountModal(3)
    expect(wrapper.find('input[name="key-lock-duration"]').exists()).toBe(false)
    await wrapper.get('input[value="api_key"]').setValue()
    expect(wrapper.get<HTMLInputElement>('input[value="permanent"]').element.checked).toBe(true)
    expect(wrapper.find('#key-lock-minutes').exists()).toBe(false)
    await wrapper.get('input[value="timed"]').setValue()
    expect(wrapper.get<HTMLInputElement>('#key-lock-minutes').element.value).toBe('10')
    await wrapper.get('form').trigger('submit')
    await flushPromises()
    expect(update).toHaveBeenCalledWith(7, expect.objectContaining({ moderation_penalty_scope: 'api_key', moderation_key_lock_minutes: 10 }))
  })

  it('preserves a saved custom duration on unrelated edits and only sends minutes when changed', async () => {
    const wrapper = mountModal(3, { moderation_penalty_scope: 'api_key', moderation_key_lock_minutes: 37 })
    expect(wrapper.get<HTMLInputElement>('input[value="timed"]').element.checked).toBe(true)
    expect(wrapper.get<HTMLInputElement>('#key-lock-minutes').element.value).toBe('37')
    await wrapper.get('form').trigger('submit')
    await flushPromises()
    expect(update.mock.calls[0][1]).not.toHaveProperty('moderation_key_lock_minutes')
    await wrapper.get('#key-lock-minutes').setValue('12')
    await wrapper.get('form').trigger('submit')
    await flushPromises()
    expect(update.mock.calls[1][1]).toHaveProperty('moderation_key_lock_minutes', 12)
    expect(update.mock.calls[1][1]).not.toHaveProperty('moderation_penalty_scope')
    expect(wrapper.find('[role="status"]').exists()).toBe(false)
    await wrapper.get('input[value="permanent"]').setValue()
    await wrapper.get('form').trigger('submit')
    await flushPromises()
    expect(update.mock.calls[2][1]).toHaveProperty('moderation_key_lock_minutes', 0)
  })

  it.each(['', '0', '-1', '1.5', '2147483648'])('rejects invalid timed duration %j', async (value) => {
    const wrapper = mountModal(3, { moderation_penalty_scope: 'api_key' })
    await wrapper.get('input[value="timed"]').setValue()
    await wrapper.get('#key-lock-minutes').setValue(value)
    await wrapper.get('form').trigger('submit')
    await flushPromises()
    expect(showError).toHaveBeenCalledWith('admin.users.keyLockMinutesInvalid')
    expect(update).not.toHaveBeenCalled()
  })

  it('discards unsaved edits when reopened and omits hidden minutes when switching back to account scope', async () => {
    const wrapper = mountModal(3, { moderation_penalty_scope: 'api_key', moderation_key_lock_minutes: 37 })
    await wrapper.get('#key-lock-minutes').setValue('55')
    await wrapper.setProps({ show: false })
    await wrapper.setProps({ show: true })
    expect(wrapper.get<HTMLInputElement>('#key-lock-minutes').element.value).toBe('37')
    await wrapper.get('#key-lock-minutes').setValue('55')
    await wrapper.get('input[value="user"]').setValue()
    expect(wrapper.find('input[name="key-lock-duration"]').exists()).toBe(false)
    await wrapper.get('form').trigger('submit')
    await flushPromises()
    expect(update.mock.calls[0][1]).toHaveProperty('moderation_penalty_scope', 'user')
    expect(update.mock.calls[0][1]).not.toHaveProperty('moderation_key_lock_minutes')
  })

  it('omits the scope when an administrator reverts the selection before saving', async () => {
    const wrapper = mountModal(3)
    await wrapper.get('input[value="api_key"]').setValue()
    await wrapper.get('input[value="user"]').setValue()
    await wrapper.get('form').trigger('submit')
    await flushPromises()
    expect(update.mock.calls[0][1]).not.toHaveProperty('moderation_penalty_scope')
  })

  it('does not offer a scope change when editing an administrator', async () => {
    const wrapper = mountModal(3, { role: 'admin' })
    expect(wrapper.find('input[name="moderation-scope"]').exists()).toBe(false)
    await wrapper.get('form').trigger('submit')
    await flushPromises()
    expect(update.mock.calls[0][1]).not.toHaveProperty('moderation_penalty_scope')
  })

  // Regression coverage for issue #5977: the gateway treats concurrency <= 0 as
  // unlimited (AcquireUserSlot) and both the batch limits endpoint and the bulk
  // edit modal accept 0, so this dialog must not be the only place that rejects
  // it — doing so blocked every other edit on such a user.
  it('saves an unlimited (0) concurrency instead of blocking the whole form', async () => {
    const wrapper = mountModal(0)

    await wrapper.get('form').trigger('submit')
    await flushPromises()

    expect(showError).not.toHaveBeenCalled()
    expect(update).toHaveBeenCalledWith(7, expect.objectContaining({ concurrency: 0 }))
    expect(wrapper.emitted('success')).toBeTruthy()
  })

  it('still rejects a negative concurrency', async () => {
    const wrapper = mountModal(3)

    await wrapper.get('[data-test="concurrency-input"]').setValue('-1')
    await wrapper.get('form').trigger('submit')
    await flushPromises()

    expect(showError).toHaveBeenCalledWith('admin.users.concurrencyNonNegative')
    expect(update).not.toHaveBeenCalled()
  })
})
