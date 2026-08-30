import { readFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'
import { describe, expect, it, vi } from 'vitest'

const authStore = vi.hoisted(() => ({
  checkAuth: vi.fn(),
  isAuthenticated: true,
  isAdmin: true,
  isSimpleMode: false,
  hasPendingAuthSession: false,
}))

vi.mock('@/stores/auth', () => ({ useAuthStore: () => authStore }))
vi.mock('@/stores/app', () => ({
  useAppStore: () => ({
    siteName: 'Sub2API',
    backendModeEnabled: false,
    publicSettingsLoaded: true,
    cachedPublicSettings: { custom_menu_items: [] },
    fetchPublicSettings: vi.fn(),
  }),
}))
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

describe('official pricing admin navigation', () => {
  it('does not register a separate official pricing route', async () => {
    const { default: router } = await import('@/router')
    const route = router.getRoutes().find(record => record.name === 'AdminOfficialPricing')

    expect(route).toBeUndefined()
  })

  it('keeps one display-pricing sidebar entry and embeds official pricing as a local tab', () => {
    const sidebarPath = resolve(dirname(fileURLToPath(import.meta.url)), '../../components/layout/AppSidebar.vue')
    const viewPath = resolve(dirname(fileURLToPath(import.meta.url)), '../../views/admin/DisplayPricingView.vue')
    const sidebarSource = readFileSync(sidebarPath, 'utf8')
    const viewSource = readFileSync(viewPath, 'utf8')
    const displayEntry = "{ path: '/admin/channels/display-pricing', label: t('nav.displayPricing'), icon: PriceTagIcon }"
    const monitorEntry = "{ path: '/admin/channels/upstream-price-monitor', label: t('nav.upstreamPriceMonitor'), icon: SignalIcon }"

    expect(sidebarSource).toContain(displayEntry)
    expect(sidebarSource).not.toContain('/admin/channels/official-pricing')
    expect(sidebarSource).toContain(monitorEntry)
    expect(viewSource).toContain('<OfficialPricingView v-if="activePanel === \'official\'" />')
    expect(viewSource).toContain('data-testid="official-pricing-tab"')
    expect(viewSource).not.toContain('to="/admin/channels/official-pricing"')
  })
})
