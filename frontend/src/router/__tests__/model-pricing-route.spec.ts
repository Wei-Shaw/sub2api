import { describe, expect, it, vi } from 'vitest'

const authStore = vi.hoisted(() => ({
  checkAuth: vi.fn(),
  isAuthenticated: false,
  isAdmin: false,
  isSimpleMode: false,
  hasPendingAuthSession: false
}))

const appStore = vi.hoisted(() => ({
  siteName: 'Sub2API',
  backendModeEnabled: false,
  publicSettingsLoaded: true,
  cachedPublicSettings: {
    model_plaza_enabled: true,
    model_plaza_require_auth: false,
    custom_menu_items: []
  },
  fetchPublicSettings: vi.fn()
}))

vi.mock('@/stores/auth', () => ({ useAuthStore: () => authStore }))
vi.mock('@/stores/app', () => ({ useAppStore: () => appStore }))
vi.mock('@/stores/adminSettings', () => ({
  useAdminSettingsStore: () => ({ customMenuItems: [] })
}))
vi.mock('@/stores/adminCompliance', () => ({
  useAdminComplianceStore: () => ({
    initialized: true,
    fetchStatus: vi.fn(),
    requireAcknowledgement: vi.fn()
  })
}))
vi.mock('@/composables/useNavigationLoading', () => ({
  useNavigationLoadingState: () => ({
    startNavigation: vi.fn(),
    endNavigation: vi.fn(),
    isLoading: { value: false }
  })
}))
vi.mock('@/composables/useRoutePrefetch', () => ({
  useRoutePrefetch: () => ({
    triggerPrefetch: vi.fn(),
    cancelPendingPrefetch: vi.fn(),
    resetPrefetchState: vi.fn()
  })
}))

describe('model pricing routes', () => {
  it('registers /pricing as public while retaining the authenticated sidebar route', async () => {
    const { default: router } = await import('@/router')
    const publicRoute = router.getRoutes().find((record) => record.name === 'PublicModelPrices')
    const userRoute = router.getRoutes().find((record) => record.name === 'ModelPrices')

    expect(publicRoute?.path).toBe('/pricing')
    expect(publicRoute?.meta.requiresAuth).toBe(false)
    expect(userRoute?.path).toBe('/model-prices')
    expect(userRoute?.meta.requiresAuth).toBe(true)
  })
})
