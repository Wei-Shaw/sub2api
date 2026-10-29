import { mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'
import ProfileEditForm from '@/components/user/profile/ProfileEditForm.vue'

const { updateProfileMock, showErrorMock, authState } = vi.hoisted(() => ({
  updateProfileMock: vi.fn(),
  showErrorMock: vi.fn(),
  authState: { user: { username: 'alice' } },
}))

vi.mock('@/api', () => ({
  userAPI: { updateProfile: updateProfileMock },
}))
vi.mock('@/stores/auth', () => ({
  useAuthStore: () => authState,
}))
vi.mock('@/stores/app', () => ({
  useAppStore: () => ({ showError: showErrorMock }),
}))
vi.mock('vue-i18n', () => ({
  useI18n: () => ({ t: (key: string) => key }),
}))

describe('ProfileEditForm', () => {
  it.each([
    [{ status: 400, code: 'VALIDATION_ERROR', message: 'username is too long' }, 'username is too long'],
    [{ response: { data: { detail: 'backend failure' } } }, 'backend failure'],
    [{}, 'profile.updateFailed'],
  ])('shows API failure %j without changing the saved profile', async (error, expectedMessage) => {
    updateProfileMock.mockRejectedValue(error)
    const wrapper = mount(ProfileEditForm, { props: { initialUsername: 'alice' } })

    await wrapper.get('#username').setValue('new-name')
    await wrapper.get('form').trigger('submit.prevent')

    expect(updateProfileMock).toHaveBeenCalledWith({ username: 'new-name' })
    expect(showErrorMock).toHaveBeenLastCalledWith(expectedMessage)
    expect(authState.user.username).toBe('alice')
    expect(wrapper.get('button[type="submit"]').attributes('disabled')).toBeUndefined()
  })
})
