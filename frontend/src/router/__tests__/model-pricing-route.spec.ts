import { readFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

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

const sidebarPath = resolve(dirname(fileURLToPath(import.meta.url)), '../../components/layout/AppSidebar.vue')

async function getRouter() {
  return (await import('@/router')).default
}

describe('model marketplace route', () => {
  it('uses a non-conflicting kebab-case route while preserving the route name', async () => {
    const router = await getRouter()
    const route = router.getRoutes().find((record) => record.name === 'ModelMarketplace')

    expect(route?.path).toBe('/model-pricing')
    expect(route?.meta.requiresAuth).toBe(true)
    expect(router.getRoutes().some((record) => record.path === '/models')).toBe(false)
  })

  it('points the user navigation to the model pricing page route', () => {
    const sidebarSource = readFileSync(sidebarPath, 'utf8')

    expect(sidebarSource).toContain("{ path: '/model-pricing', label: t('nav.modelMarketplace'), icon: PriceTagIcon }")
    expect(sidebarSource).not.toContain("{ path: '/models', label: t('nav.modelMarketplace'), icon: PriceTagIcon }")
  })
})
