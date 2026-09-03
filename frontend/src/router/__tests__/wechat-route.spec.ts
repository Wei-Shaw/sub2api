import { describe, expect, it, vi } from 'vitest'

const authStore = vi.hoisted(() => ({
  checkAuth: vi.fn(),
  isAuthenticated: false,
  isAdmin: false,
  isSimpleMode: false,
}))

const appStore = vi.hoisted(() => ({
  siteName: 'Sub2API',
  backendModeEnabled: false,
  cachedPublicSettings: null as null | Record<string, unknown>,
}))

vi.mock('@/stores/auth', () => ({
  useAuthStore: () => authStore,
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => appStore,
}))

vi.mock('@/stores/adminSettings', () => ({
  useAdminSettingsStore: () => ({
    customMenuItems: [],
  }),
}))

vi.mock('@/composables/useNavigationLoading', () => ({
  useNavigationLoadingState: () => ({
    startNavigation: vi.fn(),
    endNavigation: vi.fn(),
    isLoading: { value: false },
  }),
}))

vi.mock('@/composables/useRoutePrefetch', () => ({
  useRoutePrefetch: () => ({
    triggerPrefetch: vi.fn(),
    cancelPendingPrefetch: vi.fn(),
    resetPrefetchState: vi.fn(),
  }),
}))

describe('router WeChat OAuth route', () => {
  it('registers the WeChat callback route as a public route', async () => {
    const { default: router } = await import('@/router')
    const route = router.getRoutes().find((record) => record.name === 'WeChatOAuthCallback')

    expect(route?.path).toBe('/auth/wechat/callback')
    expect(route?.meta.requiresAuth).toBe(false)
    expect(route?.meta.title).toBe('WeChat OAuth Callback')
  })

  it('no longer registers a WeChat payment callback route', async () => {
    // WeChat Pay was removed with the other gateways; only the login callback
    // survives, and a stale payment route would 404 on a dead flow.
    const { default: router } = await import('@/router')

    expect(router.getRoutes().some((record) => record.name === 'WeChatPaymentOAuthCallback')).toBe(false)
    expect(router.getRoutes().some((record) => record.path === '/auth/wechat/payment/callback')).toBe(false)
  })
})
