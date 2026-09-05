import { beforeEach, describe, expect, it, vi } from 'vitest'

const mocks = vi.hoisted(() => ({
  authStore: {
    isAuthenticated: false,
    isAdmin: false,
    user: null as { id: number } | null,
  },
  appStore: {
    cachedPublicSettings: null as { custom_menu_items?: unknown[] } | null,
    siteName: 'Sub2API',
    showError: vi.fn(),
  },
  adminSettingsStore: {
    customMenuItems: [] as unknown[],
  },
  updateNotificationEmailLocale: vi.fn(),
}))

vi.mock('@/router', () => ({
  default: {
    currentRoute: {
      value: { name: 'dashboard', meta: {} },
    },
  },
}))

vi.mock('@/router/title', () => ({
  resolveRouteDocumentTitle: () => 'Sub2API',
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => mocks.appStore,
}))

vi.mock('@/stores/auth', () => ({
  useAuthStore: () => mocks.authStore,
}))

vi.mock('@/stores/adminSettings', () => ({
  useAdminSettingsStore: () => mocks.adminSettingsStore,
}))

vi.mock('@/api/user', () => ({
  updateNotificationEmailLocale: mocks.updateNotificationEmailLocale,
}))

import { getLocale, setLocale } from '../index'

describe('notification email locale persistence', () => {
  beforeEach(() => {
    mocks.authStore.isAuthenticated = false
    mocks.authStore.user = null
    mocks.updateNotificationEmailLocale.mockReset()
    mocks.appStore.showError.mockReset()
    localStorage.clear()
  })

  it('persists an explicit locale switch for an authenticated user', async () => {
    mocks.authStore.isAuthenticated = true
    mocks.authStore.user = { id: 42 }

    await setLocale('zh')

    expect(getLocale()).toBe('zh')
    expect(localStorage.getItem('sub2api_locale')).toBe('zh')
    expect(mocks.updateNotificationEmailLocale).toHaveBeenCalledOnce()
    expect(mocks.updateNotificationEmailLocale).toHaveBeenCalledWith('zh')
    expect(
      localStorage.getItem('notification_email_locale_initialized:v1:user:42'),
    ).toBe('1')
  })

  it('keeps anonymous locale switches local', async () => {
    await setLocale('en')

    expect(getLocale()).toBe('en')
    expect(mocks.updateNotificationEmailLocale).not.toHaveBeenCalled()
  })

  it('keeps the local switch and reports a persistence failure', async () => {
    mocks.authStore.isAuthenticated = true
    mocks.authStore.user = { id: 42 }
    mocks.updateNotificationEmailLocale.mockRejectedValueOnce(new Error('request failed'))

    await setLocale('zh')

    expect(getLocale()).toBe('zh')
    expect(mocks.appStore.showError).toHaveBeenCalledOnce()
    expect(
      localStorage.getItem('notification_email_locale_initialized:v1:user:42'),
    ).toBeNull()
  })
})
