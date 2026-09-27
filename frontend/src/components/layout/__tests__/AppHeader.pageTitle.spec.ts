import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import AppHeader from '../AppHeader.vue'

vi.mock('vue-i18n', async (importOriginal) => ({
  ...(await importOriginal<typeof import('vue-i18n')>()),
  useI18n: () => ({ t: (key: string) => key })
}))

vi.mock('vue-router', () => ({
  useRoute: () => ({ name: 'Keys', params: {}, meta: {} }),
  useRouter: () => ({ push: vi.fn() })
}))

const appStore = { cachedPublicSettings: null, contactInfo: '', docUrl: '', toggleMobileSidebar: vi.fn() }

vi.mock('@/stores', () => ({
  useAppStore: () => appStore,
  useAuthStore: () => ({ user: null, isAdmin: false, isSimpleMode: false }),
  useOnboardingStore: () => ({ replay: vi.fn() })
}))

vi.mock('@/stores/app', () => ({ useAppStore: () => appStore }))

vi.mock('@/stores/adminSettings', () => ({
  useAdminSettingsStore: () => ({ customMenuItems: [] })
}))

vi.mock('@/router/title', () => ({
  resolveRouteMetaKeys: () => ({ titleKey: 'keys.title' })
}))

describe('AppHeader page title', () => {
  it('renders exactly one page-title h1 per breakpoint', () => {
    const wrapper = mount(AppHeader, {
      global: {
        stubs: { Icon: true, LocaleSwitcher: true, SubscriptionProgressMini: true, AnnouncementBell: true, RouterLink: true }
      }
    })

    const headings = wrapper.findAll('h1')
    expect(headings.map((h) => h.text())).toEqual(['keys.title', 'keys.title'])

    const [mobile, desktop] = headings
    expect(mobile.classes()).toContain('lg:hidden')
    expect(desktop.element.parentElement?.className).toContain('hidden lg:block')
  })
})
