import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import ProfilePasswordForm from '@/components/user/profile/ProfilePasswordForm.vue'

const {
  f210ChangePasswordMock,
  f210ShowSuccessMock,
  f210ShowErrorMock,
  f210LogoutMock,
  f210PushMock
} = vi.hoisted(() => ({
  f210ChangePasswordMock: vi.fn(),
  f210ShowSuccessMock: vi.fn(),
  f210ShowErrorMock: vi.fn(),
  f210LogoutMock: vi.fn(),
  f210PushMock: vi.fn()
}))

vi.mock('@/api', () => ({
  userAPI: {
    changePassword: f210ChangePasswordMock
  }
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({
    showSuccess: f210ShowSuccessMock,
    showError: f210ShowErrorMock
  })
}))

vi.mock('@/stores/auth', () => ({
  useAuthStore: () => ({
    logout: f210LogoutMock
  })
}))

vi.mock('vue-router', () => ({
  useRouter: () => ({ push: f210PushMock })
}))

vi.mock('vue-i18n', async (importOriginal) => {
  const actual = await importOriginal<typeof import('vue-i18n')>()
  return {
    ...actual,
    useI18n: () => ({ t: (key: string) => key })
  }
})

async function f210Submit(wrapper: ReturnType<typeof mount>) {
  await wrapper.get('#old_password').setValue('old-password')
  await wrapper.get('#new_password').setValue('new-password')
  await wrapper.get('#confirm_password').setValue('new-password')
  await wrapper.get('form').trigger('submit.prevent')
  await flushPromises()
}

describe('ProfilePasswordForm after a password change (F2-10)', () => {
  beforeEach(() => {
    f210ChangePasswordMock.mockReset()
    f210ShowSuccessMock.mockReset()
    f210ShowErrorMock.mockReset()
    f210LogoutMock.mockReset()
    f210PushMock.mockReset()
    f210LogoutMock.mockResolvedValue(undefined)
    f210PushMock.mockResolvedValue(undefined)
  })

  it('signs out and returns to the login page, because the backend revoked every token', async () => {
    f210ChangePasswordMock.mockResolvedValue({ message: 'ok' })

    const wrapper = mount(ProfilePasswordForm)
    await f210Submit(wrapper)

    expect(f210ChangePasswordMock).toHaveBeenCalledWith('old-password', 'new-password')
    expect(f210ShowSuccessMock).toHaveBeenCalledWith('profile.passwordChangeSuccess')
    expect(f210ShowErrorMock).not.toHaveBeenCalled()
    expect(f210LogoutMock).toHaveBeenCalledTimes(1)
    expect(f210PushMock).toHaveBeenCalledWith('/login')
    expect(f210LogoutMock.mock.invocationCallOrder[0]).toBeLessThan(
      f210PushMock.mock.invocationCallOrder[0]
    )
  })

  it('keeps the session when the password change fails', async () => {
    f210ChangePasswordMock.mockRejectedValue({ status: 400, message: 'current password is incorrect' })

    const wrapper = mount(ProfilePasswordForm)
    await f210Submit(wrapper)

    expect(f210ShowErrorMock).toHaveBeenCalledWith('current password is incorrect')
    expect(f210LogoutMock).not.toHaveBeenCalled()
    expect(f210PushMock).not.toHaveBeenCalled()
  })
})
