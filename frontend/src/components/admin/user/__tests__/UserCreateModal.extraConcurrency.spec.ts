import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'

import UserCreateModal from '../UserCreateModal.vue'

const { createUser, showError, showSuccess } = vi.hoisted(() => ({
  createUser: vi.fn(),
  showError: vi.fn(),
  showSuccess: vi.fn()
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    users: {
      create: createUser
    }
  }
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({
    showError,
    showSuccess
  })
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string) => key
    })
  }
})

function mountModal() {
  return mount(UserCreateModal, {
    props: { show: true },
    global: {
      stubs: {
        BaseDialog: {
          template: '<div><slot /><slot name="footer" /></div>'
        },
        Icon: true
      }
    }
  })
}

describe('UserCreateModal extra concurrency', () => {
  beforeEach(() => {
    createUser.mockReset()
    showError.mockReset()
    showSuccess.mockReset()
    createUser.mockResolvedValue({})
  })

  it('omits extra_concurrency when the administrator leaves it blank', async () => {
    const wrapper = mountModal()

    await wrapper.get('input[type="email"]').setValue('new-user@example.com')
    await wrapper.get('input[required][type="text"]').setValue('Password123!')

    expect(wrapper.get('[data-testid="create-user-extra-concurrency"]').element).toHaveProperty('value', '')

    await wrapper.get('#create-user-form').trigger('submit')
    await flushPromises()

    expect(createUser).toHaveBeenCalledTimes(1)
    expect(createUser.mock.calls[0]?.[0]).not.toHaveProperty('extra_concurrency')
  })

  it('preserves an explicit zero extra concurrency override', async () => {
    const wrapper = mountModal()

    await wrapper.get('input[type="email"]').setValue('zero-extra@example.com')
    await wrapper.get('input[required][type="text"]').setValue('Password123!')
    await wrapper.get('[data-testid="create-user-extra-concurrency"]').setValue('0')
    await wrapper.get('#create-user-form').trigger('submit')
    await flushPromises()

    expect(createUser).toHaveBeenCalledWith(expect.objectContaining({ extra_concurrency: 0 }))
  })

  it('rejects a negative extra concurrency override before calling the API', async () => {
    const wrapper = mountModal()

    await wrapper.get('input[type="email"]').setValue('invalid-extra@example.com')
    await wrapper.get('input[required][type="text"]').setValue('Password123!')
    await wrapper.get('[data-testid="create-user-extra-concurrency"]').setValue('-1')
    await wrapper.get('#create-user-form').trigger('submit')
    await flushPromises()

    expect(createUser).not.toHaveBeenCalled()
    expect(showError).toHaveBeenCalledWith('admin.users.extraConcurrencyMin')
  })
})
