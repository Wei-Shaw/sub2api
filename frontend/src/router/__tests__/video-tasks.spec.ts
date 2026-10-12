import { beforeAll, describe, expect, it, vi } from 'vitest'
import type { RouteRecordRaw } from 'vue-router'
import { readFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

type Guard = (to: Record<string, unknown>, from: Record<string, unknown>, next: ReturnType<typeof vi.fn>) => Promise<void>
const harness = vi.hoisted(() => ({ routes: [] as RouteRecordRaw[], guard: null as Guard | null }))
const auth = vi.hoisted(() => ({ checkAuth: vi.fn(), isAuthenticated: false, isAdmin: false, isSimpleMode: false }))

vi.mock('vue-router', () => ({
  createWebHistory: vi.fn(),
  createRouter: (options: { routes: RouteRecordRaw[] }) => {
    harness.routes = options.routes
    return { beforeEach: (guard: Guard) => { harness.guard = guard }, afterEach: vi.fn(), onError: vi.fn() }
  }
}))
vi.mock('@/stores/auth', () => ({ useAuthStore: () => auth }))
vi.mock('@/stores/app', () => ({ useAppStore: () => ({ siteName: 'Test', backendModeEnabled: false, cachedPublicSettings: null }) }))
vi.mock('@/stores/adminSettings', () => ({ useAdminSettingsStore: () => ({ customMenuItems: [] }) }))
vi.mock('@/stores/adminCompliance', () => ({ useAdminComplianceStore: () => ({ initialized: true }) }))
vi.mock('@/composables/useNavigationLoading', () => ({ useNavigationLoadingState: () => ({ startNavigation: vi.fn(), endNavigation: vi.fn() }) }))
vi.mock('@/composables/useRoutePrefetch', () => ({ useRoutePrefetch: () => ({ triggerPrefetch: vi.fn() }) }))
vi.mock('@/api/setup', () => ({ getSetupStatus: vi.fn() }))
vi.mock('@/router/title', () => ({ resolveRouteDocumentTitle: () => 'Video Tasks' }))

describe('admin video tasks access', () => {
  beforeAll(async () => { await import('@/router') })

  it('registers a lazy admin-only route with localized title and description', () => {
    const route = harness.routes.find(route => route.path === '/admin/video-tasks')!
    expect(route.name).toBe('AdminVideoTasks')
    expect(typeof route.component).toBe('function')
    expect(route.meta).toMatchObject({ requiresAuth: true, requiresAdmin: true,
      titleKey: 'admin.videoTasks.title', descriptionKey: 'admin.videoTasks.description' })
  })

  it.each([
    [false, false, '/login'], [true, false, '/dashboard'], [true, true, null]
  ])('enforces the actual guard for authenticated=%s admin=%s', async (authenticated, admin, redirect) => {
    auth.isAuthenticated = authenticated
    auth.isAdmin = admin
    const route = harness.routes.find(route => route.path === '/admin/video-tasks')!
    const next = vi.fn()
    await harness.guard!({ ...route, fullPath: route.path }, {}, next)
    if (redirect === '/login') expect(next).toHaveBeenCalledWith({ path: '/login', query: { redirect: route.path } })
    else if (redirect) expect(next).toHaveBeenCalledWith(redirect)
    else expect(next).toHaveBeenCalledWith()
  })

  it('places the link directly after scheduler in the admin sidebar, independent of ops flags', () => {
    const source = readFileSync(resolve(dirname(fileURLToPath(import.meta.url)), '../../components/layout/AppSidebar.vue'), 'utf8')
    const adminNavigation = source.slice(source.indexOf('const adminNavItems = computed'))
    expect(adminNavigation).toMatch(/path: '\/admin\/scheduler'[^\n]+\n\s*\{ path: '\/admin\/video-tasks', label: t\('admin.videoTasks.title'\), icon: VideoTaskIcon \}/)
    expect(source.slice(0, source.indexOf('const adminNavItems = computed'))).not.toContain("path: '/admin/video-tasks'")
  })
})
