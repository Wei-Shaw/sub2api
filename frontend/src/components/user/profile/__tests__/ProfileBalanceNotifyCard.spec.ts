import { flushPromises, mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'
import ProfileBalanceNotifyCard from '@/components/user/profile/ProfileBalanceNotifyCard.vue'

const { updateProfileMock, showSuccessMock, showErrorMock, authState } = vi.hoisted(() => ({
  updateProfileMock: vi.fn(),
  showSuccessMock: vi.fn(),
  showErrorMock: vi.fn(),
  authState: { user: null as Record<string, unknown> | null }
}))

vi.mock('@/api', () => ({
  userAPI: {
    updateProfile: updateProfileMock,
    toggleNotifyEmail: vi.fn(),
    sendNotifyEmailCode: vi.fn(),
    verifyNotifyEmail: vi.fn(),
    removeNotifyEmail: vi.fn(),
    getProfile: vi.fn()
  }
}))

vi.mock('@/stores/auth', () => ({
  useAuthStore: () => authState
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({
    showSuccess: showSuccessMock,
    showError: showErrorMock
  })
}))

vi.mock('vue-i18n', async (importOriginal) => {
  const actual = await importOriginal<typeof import('vue-i18n')>()
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string) => key
    })
  }
})

describe('ProfileBalanceNotifyCard', () => {
  it('saves a trimmed Telegram chat ID through the profile API', async () => {
    const updatedUser = {
      id: 1,
      email: 'alice@example.com',
      balance_notify_extra_emails: [],
      balance_notify_telegram_chat_id: '123456789'
    }
    updateProfileMock.mockReset()
    showSuccessMock.mockReset()
    showErrorMock.mockReset()
    authState.user = null
    updateProfileMock.mockResolvedValue(updatedUser)

    const wrapper = mount(ProfileBalanceNotifyCard, {
      props: {
        enabled: true,
        threshold: null,
        extraEmails: [],
        telegramChatId: '',
        systemDefaultThreshold: 2,
        userEmail: 'alice@example.com'
      }
    })

    await wrapper.get('[data-testid="balance-notify-telegram-chat-id-input"]').setValue(' 123456789 ')
    await wrapper.get('[data-testid="balance-notify-telegram-chat-id-save"]').trigger('click')
    await flushPromises()

    expect(updateProfileMock).toHaveBeenCalledWith({
      balance_notify_telegram_chat_id: '123456789'
    })
    expect(authState.user).toEqual(updatedUser)
    expect((wrapper.get('[data-testid="balance-notify-telegram-chat-id-input"]').element as HTMLInputElement).value).toBe('123456789')
    expect(showSuccessMock).toHaveBeenCalledWith('common.saved')
    expect(showErrorMock).not.toHaveBeenCalled()
  })
})
