import { describe, expect, it, vi, beforeEach } from 'vitest'
import { mount } from '@vue/test-utils'

import type { AdminUser } from '@/types'
import UserCreateModal from '../UserCreateModal.vue'
import UserEditModal from '../UserEditModal.vue'

const { createUser, updateUser, showError, showSuccess, authState } = vi.hoisted(() => ({
  createUser: vi.fn(),
  updateUser: vi.fn(),
  showError: vi.fn(),
  showSuccess: vi.fn(),
  authState: {
    isAdmin: true,
    isMarketing: false
  }
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    users: {
      create: createUser,
      update: updateUser
    },
    userAttributes: {
      updateUserAttributeValues: vi.fn()
    }
  }
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({
    showError,
    showSuccess
  })
}))

vi.mock('@/stores/auth', () => ({
  useAuthStore: () => ({
    get isAdmin() { return authState.isAdmin },
    get isMarketing() { return authState.isMarketing }
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

const BaseDialogStub = {
  props: ['show', 'title'],
  emits: ['close'],
  template: '<div v-if="show"><slot /><slot name="footer" /></div>'
}

const SelectStub = {
  props: ['modelValue', 'options', 'disabled'],
  emits: ['update:modelValue'],
  template: `
    <select
      :data-test="options.some((option) => option.value === 'marketing') ? 'role-select' : 'status-select'"
      :value="modelValue"
      :disabled="disabled"
      @change="$emit('update:modelValue', $event.target.value)"
    >
      <option v-for="option in options" :key="option.value" :value="option.value">{{ option.label }}</option>
    </select>
  `
}

const UserAttributeFormStub = {
  props: ['modelValue', 'userId'],
  emits: ['update:modelValue'],
  template: '<div data-test="attribute-form" />'
}

const adminUser = (overrides: Partial<AdminUser> = {}): AdminUser => ({
  id: 7,
  username: 'existing',
  email: 'existing@example.com',
  role: 'user',
  balance: 0,
  concurrency: 1,
  rpm_limit: 0,
  status: 'active',
  allowed_groups: [],
  balance_notify_enabled: false,
  balance_notify_threshold: null,
  balance_notify_extra_emails: [],
  created_at: '2026-05-07T00:00:00Z',
  updated_at: '2026-05-07T00:00:00Z',
  notes: '',
  ...overrides
})

describe('admin user role modals', () => {
  beforeEach(() => {
    createUser.mockReset()
    updateUser.mockReset()
    showError.mockReset()
    showSuccess.mockReset()
    authState.isAdmin = true
    authState.isMarketing = false
    createUser.mockResolvedValue(adminUser())
    updateUser.mockResolvedValue(adminUser())
  })

  it('includes selected role when creating a user', async () => {
    const wrapper = mount(UserCreateModal, {
      props: { show: true },
      global: {
        stubs: {
          BaseDialog: BaseDialogStub,
          Select: SelectStub,
          Icon: true
        }
      }
    })

    await wrapper.find('input[type="email"]').setValue('marketer@example.com')
    await wrapper.find('input[type="text"]').setValue('safe-password')
    await wrapper.find('[data-test="role-select"]').setValue('marketing')
    await wrapper.find('form').trigger('submit.prevent')

    expect(createUser).toHaveBeenCalledWith(expect.objectContaining({
      email: 'marketer@example.com',
      password: 'safe-password',
      role: 'marketing'
    }))
  })

  it('preloads and submits role when editing a user', async () => {
    const wrapper = mount(UserEditModal, {
      props: { show: true, user: adminUser({ role: 'marketing' }) },
      global: {
        stubs: {
          BaseDialog: BaseDialogStub,
          Select: SelectStub,
          UserAttributeForm: UserAttributeFormStub,
          Icon: true
        }
      }
    })

    expect((wrapper.find('[data-test="role-select"]').element as HTMLSelectElement).value).toBe('marketing')

    await wrapper.find('[data-test="role-select"]').setValue('admin')
    await wrapper.find('form').trigger('submit.prevent')

    expect(updateUser).toHaveBeenCalledWith(7, expect.objectContaining({
      role: 'admin'
    }))
  })

  it('hides role editing and omits role when marketing updates a managed user', async () => {
    authState.isAdmin = false
    authState.isMarketing = true

    const wrapper = mount(UserEditModal, {
      props: { show: true, user: adminUser({ role: 'user', status: 'active' }) },
      global: {
        stubs: {
          BaseDialog: BaseDialogStub,
          Select: SelectStub,
          UserAttributeForm: UserAttributeFormStub,
          Icon: true
        }
      }
    })

    expect(wrapper.find('[data-test="role-select"]').exists()).toBe(false)
    await wrapper.find('input[type="email"]').setValue('edited@example.com')
    await wrapper.find('[data-test="status-select"]').setValue('blocked')
    await wrapper.find('form').trigger('submit.prevent')

    expect(updateUser).toHaveBeenCalledWith(7, expect.objectContaining({
      email: 'edited@example.com',
      status: 'blocked'
    }))
    expect(updateUser.mock.calls[0][1]).not.toHaveProperty('role')
  })
})
