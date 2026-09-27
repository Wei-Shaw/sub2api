import { afterEach, describe, expect, it, vi } from 'vitest'
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
  useAuthStore: () => ({
    user: { username: 'alice', email: 'alice@example.com', role: 'user', balance: 0 },
    isAdmin: false,
    isSimpleMode: false
  }),
  useOnboardingStore: () => ({ replay: vi.fn() })
}))

vi.mock('@/stores/app', () => ({ useAppStore: () => appStore }))

vi.mock('@/stores/adminSettings', () => ({
  useAdminSettingsStore: () => ({ customMenuItems: [] })
}))

vi.mock('@/router/title', () => ({
  resolveRouteMetaKeys: () => ({ titleKey: 'keys.title' })
}))

let wrapper: ReturnType<typeof mount> | undefined
afterEach(() => {
  wrapper?.unmount()
  document.body.innerHTML = ''
})

describe('AppHeader user menu', () => {
  it('closes on Escape from inside the menu and returns focus to the trigger', async () => {
    wrapper = mount(AppHeader, {
      attachTo: document.body,
      global: {
        stubs: {
          Icon: true,
          LocaleSwitcher: true,
          SubscriptionProgressMini: true,
          AnnouncementBell: true,
          RouterLink: { template: '<a href="#"><slot /></a>' }
        }
      }
    })
    const trigger = wrapper.get('button[aria-label="common.userMenu"]')
    await trigger.trigger('click')
    expect(trigger.attributes('aria-expanded')).toBe('true')

    const item = wrapper.get('.dropdown a')
    ;(item.element as HTMLElement).focus()
    await item.trigger('keydown', { key: 'Escape' })

    expect(trigger.attributes('aria-expanded')).toBe('false')
    expect(document.activeElement).toBe(trigger.element)
  })
})
