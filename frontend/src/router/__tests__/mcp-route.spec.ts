import { afterEach, describe, expect, it, vi } from 'vitest'

vi.mock('@/stores/auth', () => ({ useAuthStore: () => ({ checkAuth: vi.fn(), isAuthenticated: false, isAdmin: false, isSimpleMode: false }) }))
vi.mock('@/stores/app', () => ({ useAppStore: () => ({ siteName: 'Sub2API', backendModeEnabled: false, cachedPublicSettings: null }) }))
vi.mock('@/stores/adminSettings', () => ({ useAdminSettingsStore: () => ({ customMenuItems: [] }) }))
vi.mock('@/stores/adminCompliance', () => ({ useAdminComplianceStore: () => ({}) }))
vi.mock('@/composables/useNavigationLoading', () => ({
  useNavigationLoadingState: () => ({ startNavigation: vi.fn(), endNavigation: vi.fn() })
}))
vi.mock('@/composables/useRoutePrefetch', () => ({ useRoutePrefetch: () => ({ triggerPrefetch: vi.fn() }) }))
vi.mock('@/views/auth/LoginView.vue', () => ({ default: { template: '<div>Login</div>' } }))
vi.mock('@/router/title', () => ({ resolveRouteDocumentTitle: () => 'MCP OAuth' }))

afterEach(() => { vi.restoreAllMocks() })

describe('MCP OAuth route', () => {
  it('requires login and preserves the entire authorization query in the login redirect', async () => {
    vi.spyOn(window, 'scrollTo').mockImplementation(() => {})
    const { default: router } = await import('@/router')
    const record = router.getRoutes().find(route => route.name === 'MCPOAuthAuthorize')
    expect(record?.path).toBe('/oauth/mcp/authorize')
    expect(record?.meta.requiresAuth).toBe(true)
    expect(record?.meta.requiresAdmin).toBe(false)
    const path = '/oauth/mcp/authorize?' + new URLSearchParams({
      response_type: 'code', client_id: 'desktop', redirect_uri: 'http://127.0.0.1:54321/callback',
      resource: 'https://api.example.com/api/v1/mcp', state: 'a+b&x=1',
      code_challenge: 'a'.repeat(43), code_challenge_method: 'S256'
    }).toString()
    await router.push(path)
    expect(router.currentRoute.value.path).toBe('/login')
    expect(router.currentRoute.value.query.redirect).toBe(path)
  })
})
