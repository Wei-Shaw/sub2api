import { afterEach, describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { nextTick, reactive } from 'vue'
import AppSidebar from '../AppSidebar.vue'

const appStore = reactive({
  sidebarCollapsed: false,
  mobileOpen: false,
  backendModeEnabled: false,
  siteName: 'Acme API',
  siteLogo: '',
  siteVersion: '',
  publicSettingsLoaded: true,
  cachedPublicSettings: null,
  sidebarScrollTop: 0,
  toggleSidebar: vi.fn(),
  setMobileOpen: vi.fn((open: boolean) => {
    appStore.mobileOpen = open
  })
})

vi.mock('vue-i18n', async (importOriginal) => ({
  ...(await importOriginal<typeof import('vue-i18n')>()),
  useI18n: () => ({ t: (key: string) => key })
}))

vi.mock('vue-router', () => ({
  useRoute: () => ({ path: '/dashboard', name: 'Dashboard', params: {}, meta: {} }),
  useRouter: () => ({ push: vi.fn() })
}))

vi.mock('@/stores', () => ({
  useAppStore: () => appStore,
  useAuthStore: () => ({ isAdmin: false, isSimpleMode: false }),
  useOnboardingStore: () => ({ isCurrentStep: () => false, nextStep: vi.fn() }),
  useAdminSettingsStore: () => ({ customMenuItems: [], fetch: vi.fn() })
}))

vi.mock('@/stores/app', () => ({ useAppStore: () => appStore }))

vi.mock('@/composables/useBatchImageAccess', () => ({
  useBatchImageAccess: () => ({ canUseBatchImage: { value: false }, refreshBatchImageAccess: vi.fn() })
}))

function mountSidebar() {
  return mount(AppSidebar, {
    attachTo: document.body,
    global: { stubs: { RouterLink: { template: '<a><slot /></a>' }, VersionBadge: true, Icon: true } }
  })
}

describe('AppSidebar mobile drawer', () => {
  afterEach(() => {
    appStore.mobileOpen = false
    document.body.innerHTML = ''
  })

  it('hides the closed drawer from keyboard and screen readers below lg', async () => {
    const wrapper = mountSidebar()
    const aside = wrapper.get('aside')
    expect(aside.classes()).toContain('max-lg:invisible')

    appStore.mobileOpen = true
    await nextTick()
    expect(aside.classes()).not.toContain('max-lg:invisible')
    wrapper.unmount()
  })

  it('closes the open drawer on Escape', async () => {
    appStore.mobileOpen = true
    const wrapper = mountSidebar()

    document.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }))
    await nextTick()

    expect(appStore.setMobileOpen).toHaveBeenCalledWith(false)
    expect(wrapper.get('aside').classes()).toContain('max-lg:invisible')
    wrapper.unmount()
  })

  it('hides the desktop-only collapse button on mobile', () => {
    const wrapper = mountSidebar()
    const collapse = wrapper.get('button[aria-label="nav.collapse"]')
    expect(collapse.classes()).toEqual(expect.arrayContaining(['hidden', 'lg:flex']))
    wrapper.unmount()
  })

  it('uses the site name as the logo alt text', () => {
    const wrapper = mountSidebar()
    expect(wrapper.get('.sidebar-logo img').attributes('alt')).toBe('Acme API')
    wrapper.unmount()
  })
})
