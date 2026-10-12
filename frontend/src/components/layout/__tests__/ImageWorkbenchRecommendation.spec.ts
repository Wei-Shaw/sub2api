import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { reactive, ref } from 'vue'
import { createMemoryHistory, createRouter } from 'vue-router'
import AppSidebar from '../AppSidebar.vue'
import DashboardView from '@/views/user/DashboardView.vue'

const workbench = { id: '97370c95df45cba1', label: '生图工作台', visibility: 'user', sort_order: 10, url: '/image' }
const target = `/custom/${workbench.id}`
const app = reactive({
  cachedPublicSettings: { custom_menu_items: [workbench] }, sidebarCollapsed: false,
  mobileOpen: false, sidebarScrollTop: 0, siteLogo: '', backendModeEnabled: false,
  toggleSidebar: vi.fn(), setMobileOpen: vi.fn()
})
const auth = reactive({ isAdmin: false, isSimpleMode: false, user: { balance: 0 }, refreshUser: vi.fn() })
const admin = { customMenuItems: [], fetch: vi.fn(), opsMonitoringEnabled: true, paymentEnabled: true }

vi.mock('@/stores', () => ({
  useAppStore: () => app, useAuthStore: () => auth, useAdminSettingsStore: () => admin,
  useOnboardingStore: () => ({ isCurrentStep: () => false })
}))
vi.mock('@/stores/app', () => ({ useAppStore: () => app }))
vi.mock('@/stores/auth', () => ({ useAuthStore: () => auth }))
vi.mock('@/composables/useBatchImageAccess', () => ({
  useBatchImageAccess: () => ({ canUseBatchImage: ref(false), refreshBatchImageAccess: vi.fn() })
}))
vi.mock('vue-i18n', async (importOriginal) => ({
  ...await importOriginal<typeof import('vue-i18n')>(),
  useI18n: () => ({ t: (key: string) => key })
}))
vi.mock('@/api/user', () => ({ getMyPlatformQuotas: vi.fn().mockResolvedValue({ platform_quotas: [] }) }))
vi.mock('@/api/usage', () => ({ usageAPI: {
  getDashboardStats: vi.fn().mockResolvedValue(null),
  getDashboardTrend: vi.fn().mockResolvedValue({ trend: [] }),
  getDashboardModels: vi.fn().mockResolvedValue({ models: [] }),
  getByDateRange: vi.fn().mockResolvedValue({ items: [] })
} }))
vi.mock('@/components/layout/AppLayout.vue', () => ({ default: { template: '<main><slot /></main>' } }))

async function render(component: typeof AppSidebar | typeof DashboardView) {
  const router = createRouter({ history: createMemoryHistory(), routes: [
    { path: '/:pathMatch(.*)*', component: { template: '<div />' } }
  ] })
  await router.push('/dashboard')
  await router.isReady()
  const wrapper = mount(component, { global: { plugins: [router], stubs: {
    UserDashboardStats: true, UserDashboardCharts: true, UserDashboardRecentUsage: true, UserDashboardQuickActions: true
  } } })
  await flushPromises()
  return { wrapper, router }
}

beforeEach(() => {
  app.cachedPublicSettings.custom_menu_items = [{ ...workbench }]
  app.sidebarCollapsed = false
  auth.isAdmin = false
  auth.isSimpleMode = false
})

describe('image workbench recommendation', () => {
  for (const isAdmin of [false, true]) {
    it(`shows one working recommended link above API keys for ${isAdmin ? 'admin' : 'user'}`, async () => {
      auth.isAdmin = isAdmin
      const { wrapper, router } = await render(AppSidebar)
      const links = wrapper.findAll('a')
      const workbenchLinks = links.filter(link => link.attributes('href') === target)
      expect(workbenchLinks).toHaveLength(1)
      expect(workbenchLinks[0].text()).toContain('推荐')
      expect(links.indexOf(workbenchLinks[0])).toBeLessThan(links.findIndex(link => link.attributes('href') === '/keys'))
      await workbenchLinks[0].trigger('click')
      await flushPromises()
      expect(router.currentRoute.value.path).toBe(target)
      app.sidebarCollapsed = true
      await flushPromises()
      const collapsed = wrapper.find(`a[href="${target}"]`)
      expect(collapsed.attributes('title')).toBe('生图工作台')
      expect(collapsed.text()).not.toContain('推荐')
      wrapper.unmount()
    })
  }

  it('shows the dashboard card when menu settings arrive and navigates to the existing workbench', async () => {
    app.cachedPublicSettings.custom_menu_items = []
    const { wrapper, router } = await render(DashboardView)
    expect(wrapper.find(`a[href="${target}"]`).exists()).toBe(false)
    app.cachedPublicSettings.custom_menu_items = [{ ...workbench }]
    await flushPromises()
    const card = wrapper.find(`a[href="${target}"]`)
    expect(card.text()).toContain('AI 生图工作台')
    expect(card.text()).toContain('推荐')
    await card.trigger('click')
    await flushPromises()
    expect(router.currentRoute.value.path).toBe(target)
    wrapper.unmount()
  })

  it('does not promote admin-only or missing custom menus to users', async () => {
    app.cachedPublicSettings.custom_menu_items = [{ ...workbench, visibility: 'admin' }]
    const sidebar = await render(AppSidebar)
    const dashboard = await render(DashboardView)
    expect(sidebar.wrapper.find(`a[href="${target}"]`).exists()).toBe(false)
    expect(dashboard.wrapper.find(`a[href="${target}"]`).exists()).toBe(false)
    sidebar.wrapper.unmount()
    dashboard.wrapper.unmount()
  })
})
