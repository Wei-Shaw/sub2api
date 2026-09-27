import { afterAll, beforeAll, describe, expect, it, vi } from 'vitest'
import { i18n, setLocale } from '@/i18n'
import en from '../locales/en'

// vite.config.ts defines this for the app build; without it the runtime build cannot compile messages and t() returns keys.
vi.hoisted(() => {
  (globalThis as Record<string, unknown>).__INTLIFY_JIT_COMPILATION__ = true
})

type NavigationGuard = (
  to: Record<string, any>,
  from: Record<string, any>,
  next: ReturnType<typeof vi.fn>
) => Promise<void>

const routerHarness = vi.hoisted(() => ({
  guard: null as NavigationGuard | null,
  currentRoute: { value: { name: 'Dashboard', params: {}, meta: {} } as Record<string, any> },
}))

const authStore = vi.hoisted(() => ({
  checkAuth: vi.fn(),
  isAuthenticated: false,
  isAdmin: false,
  isSimpleMode: false,
  hasPendingAuthSession: false,
}))

const appStore = vi.hoisted(() => ({
  siteName: 'Sub2API',
  backendModeEnabled: false,
  publicSettingsLoaded: true,
  cachedPublicSettings: {} as Record<string, unknown>,
  fetchPublicSettings: vi.fn(),
}))

vi.mock('vue-router', () => ({
  createWebHistory: vi.fn(() => ({})),
  createRouter: vi.fn(() => ({
    beforeEach: vi.fn((guard: NavigationGuard) => {
      routerHarness.guard = guard
    }),
    afterEach: vi.fn(),
    onError: vi.fn(),
    currentRoute: routerHarness.currentRoute,
  })),
}))
vi.mock('@/stores/auth', () => ({ useAuthStore: () => authStore }))
vi.mock('@/stores/app', () => ({ useAppStore: () => appStore }))
vi.mock('@/stores/adminSettings', () => ({ useAdminSettingsStore: () => ({ customMenuItems: [] }) }))
vi.mock('@/stores/adminCompliance', () => ({
  useAdminComplianceStore: () => ({ initialized: true, fetchStatus: vi.fn(), requireAcknowledgement: vi.fn() }),
}))
vi.mock('@/composables/useNavigationLoading', () => ({
  useNavigationLoadingState: () => ({ startNavigation: vi.fn(), endNavigation: vi.fn(), isLoading: { value: false } }),
}))
vi.mock('@/composables/useRoutePrefetch', () => ({
  useRoutePrefetch: () => ({ triggerPrefetch: vi.fn(), cancelPendingPrefetch: vi.fn(), resetPrefetchState: vi.fn() }),
}))

async function navigate(path: string, meta: Record<string, unknown>) {
  const next = vi.fn()
  await routerHarness.guard!({ path, fullPath: path, name: path, params: {}, meta }, {}, next)
  return next
}

describe('lazy admin locale namespace', () => {
  beforeAll(async () => {
    await import('@/router')
  })

  afterAll(() => {
    localStorage.clear()
  })

  it('does not load admin.* for anonymous visitors on public pages', async () => {
    const next = await navigate('/login', { requiresAuth: false, title: 'Login' })

    expect(next).toHaveBeenCalledWith()
    expect(i18n.global.te('admin.ops.title', 'en')).toBe(false)
  })

  it('loads admin.* before entering a signed-in route, so no raw key is shown', async () => {
    authStore.isAuthenticated = true
    authStore.isAdmin = true

    const next = await navigate('/admin/ops', { requiresAuth: true, requiresAdmin: true, title: 'Ops', titleKey: 'admin.ops.title' })

    expect(next).toHaveBeenCalledWith()
    expect(i18n.global.t('admin.ops.title')).toBe(en.admin.ops.title)
    expect(document.title).toBe(`${en.admin.ops.title} - Sub2API`)
  })

  it('loads admin.* for the new locale when switching language after sign-in', async () => {
    await setLocale('vi')

    expect(i18n.global.te('admin.ops.title', 'vi')).toBe(true)
  })
})
