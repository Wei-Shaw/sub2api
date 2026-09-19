import { beforeEach, describe, expect, it, vi } from 'vitest'

const mocks = vi.hoisted(() => ({
  initializeNotificationEmailLocale: vi.fn(),
}))

vi.mock('@/api/user', () => ({
  initializeNotificationEmailLocale: mocks.initializeNotificationEmailLocale,
}))

import { initializeNotificationEmailLocaleOnce } from '../notificationEmailLocale'

describe('notification email locale initialization', () => {
  beforeEach(() => {
    localStorage.clear()
    mocks.initializeNotificationEmailLocale.mockReset()
  })

  it('records a per-user marker and skips later initialization calls', async () => {
    mocks.initializeNotificationEmailLocale.mockResolvedValue(undefined)

    await initializeNotificationEmailLocaleOnce(42, 'zh')
    await initializeNotificationEmailLocaleOnce(42, 'zh')

    expect(mocks.initializeNotificationEmailLocale).toHaveBeenCalledOnce()
    expect(mocks.initializeNotificationEmailLocale).toHaveBeenCalledWith('zh')
    expect(
      localStorage.getItem('notification_email_locale_initialized:v1:user:42'),
    ).toBe('1')
  })

  it('coalesces concurrent initialization calls for the same user', async () => {
    let resolveRequest: (() => void) | undefined
    mocks.initializeNotificationEmailLocale.mockReturnValue(
      new Promise<void>((resolve) => {
        resolveRequest = resolve
      }),
    )

    const first = initializeNotificationEmailLocaleOnce(42, 'zh')
    const second = initializeNotificationEmailLocaleOnce(42, 'zh')

    expect(mocks.initializeNotificationEmailLocale).toHaveBeenCalledOnce()
    resolveRequest?.()
    await Promise.all([first, second])
  })

  it('does not mark failures so a later authenticated session can retry', async () => {
    mocks.initializeNotificationEmailLocale
      .mockRejectedValueOnce(new Error('request failed'))
      .mockResolvedValueOnce(undefined)

    await expect(initializeNotificationEmailLocaleOnce(42, 'zh')).rejects.toThrow(
      'request failed',
    )
    expect(
      localStorage.getItem('notification_email_locale_initialized:v1:user:42'),
    ).toBeNull()

    await initializeNotificationEmailLocaleOnce(42, 'zh')

    expect(mocks.initializeNotificationEmailLocale).toHaveBeenCalledTimes(2)
    expect(
      localStorage.getItem('notification_email_locale_initialized:v1:user:42'),
    ).toBe('1')
  })
})
