import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { enableAutoUnmount, flushPromises, mount } from '@vue/test-utils'

const s = await vi.hoisted(async () => {
  const { defineComponent, h, reactive } = await import('vue')
  return {
    stub: (testid: string) => defineComponent({ setup: () => () => h('div', { 'data-testid': testid }) }),
    RouterView: defineComponent({ setup: () => () => null }),
    loads: { popup: 0, compliance: 0 },
    route: reactive({ path: '/', fullPath: '/', meta: {} as Record<string, unknown> }),
    router: { afterEach: vi.fn(), replace: vi.fn() },
    getSetupStatus: vi.fn(),
    auth: reactive({ isAuthenticated: false, isAdmin: false }),
    compliance: reactive({ initialized: false, fetchStatus: vi.fn(), requireAcknowledgement: vi.fn(), reset: vi.fn() }),
    app: reactive({ siteLogo: '', siteName: 'Sub2API', cachedPublicSettings: null, fetchPublicSettings: vi.fn() }),
  }
})

vi.mock('vue-router', () => ({
  RouterView: s.RouterView,
  useRoute: () => s.route,
  useRouter: () => s.router,
}))
vi.mock('@/components/common/Toast.vue', () => ({ default: s.stub('toast') }))
vi.mock('@/components/common/NavigationProgress.vue', () => ({ default: s.stub('nav-progress') }))
vi.mock('@/api/setup', () => ({ getSetupStatus: s.getSetupStatus }))
vi.mock('@/router/title', () => ({ resolveRouteDocumentTitle: () => 'title' }))
vi.mock('@/utils/branding', () => ({ updateFavicon: vi.fn() }))
vi.mock('@/utils/featureFlags', () => ({ FeatureFlags: { subscription: 'subscription' }, isFeatureFlagEnabled: () => false }))
vi.mock('@/utils/siteBillingMode', () => ({ resolveSiteBillingMode: () => 'balance' }))
vi.mock('@/stores', () => ({
  useAppStore: () => s.app,
  useAuthStore: () => s.auth,
  useSubscriptionStore: () => ({ fetchActiveSubscriptions: vi.fn(), startPolling: vi.fn(), clear: vi.fn() }),
  useAnnouncementStore: () => ({ fetchAnnouncements: vi.fn(), reset: vi.fn() }),
  useAdminComplianceStore: () => s.compliance,
  useAdminSettingsStore: () => ({ customMenuItems: [] }),
}))

// Fresh App module per test with re-registered dialog mocks (doMock drops the cached mock), so the
// load counts are per test and prove the dialogs are not static imports in any test order.
async function mountApp() {
  const { default: App } = await import('../App.vue')
  return mount(App)
}

function signIn(isAdmin: boolean) {
  s.auth.isAuthenticated = true
  s.auth.isAdmin = isAdmin
}

describe('App', () => {
  enableAutoUnmount(afterEach)

  beforeEach(() => {
    vi.resetModules()
    s.loads.popup = 0
    s.loads.compliance = 0
    vi.doMock('@/components/common/AnnouncementPopup.vue', () => {
      s.loads.popup++
      return { __esModule: true, default: s.stub('announcement-popup') }
    })
    vi.doMock('@/components/admin/AdminComplianceDialog.vue', () => {
      s.loads.compliance++
      return { __esModule: true, default: s.stub('admin-compliance') }
    })
    window.__APP_CONFIG__ = {} as NonNullable<typeof window.__APP_CONFIG__>
    s.auth.isAuthenticated = false
    s.auth.isAdmin = false
    s.compliance.initialized = false
    s.compliance.fetchStatus.mockReset().mockResolvedValue({})
    s.getSetupStatus.mockReset().mockResolvedValue({ needs_setup: false, step: '' })
    s.app.fetchPublicSettings.mockReset().mockResolvedValue(null)
    s.router.replace.mockReset()
  })

  afterEach(() => {
    delete window.__APP_CONFIG__
  })

  it('does not load either dialog for guests and skips /setup/status when config is injected', async () => {
    const wrapper = await mountApp()
    await flushPromises()

    expect(s.loads).toEqual({ popup: 0, compliance: 0 })
    expect(wrapper.find('[data-testid="announcement-popup"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="admin-compliance"]').exists()).toBe(false)
    expect(s.getSetupStatus).not.toHaveBeenCalled()
    expect(s.app.fetchPublicSettings).toHaveBeenCalledTimes(1)
  })

  it('still redirects to /setup when the server injected no config', async () => {
    delete window.__APP_CONFIG__
    s.getSetupStatus.mockResolvedValue({ needs_setup: true, step: 'database' })

    await mountApp()
    await flushPromises()

    expect(s.getSetupStatus).toHaveBeenCalledTimes(1)
    expect(s.router.replace).toHaveBeenCalledWith('/setup')
  })

  it('loads only the announcement popup for signed-in users', async () => {
    signIn(false)
    const wrapper = await mountApp()
    await flushPromises()

    expect(wrapper.find('[data-testid="announcement-popup"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="admin-compliance"]').exists()).toBe(false)
    expect(s.loads).toEqual({ popup: 1, compliance: 0 })
  })

  it('reuses the compliance status the router guard already fetched', async () => {
    signIn(true)
    s.compliance.initialized = true
    const wrapper = await mountApp()
    await flushPromises()

    expect(s.compliance.fetchStatus).not.toHaveBeenCalled()
    expect(wrapper.find('[data-testid="admin-compliance"]').exists()).toBe(true)
  })

  it('fetches the compliance status once when the guard has not', async () => {
    signIn(true)
    await mountApp()
    await flushPromises()

    expect(s.compliance.fetchStatus).toHaveBeenCalledTimes(1)
  })
})
