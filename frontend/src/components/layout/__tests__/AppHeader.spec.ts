import { beforeEach, describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import AppHeader from '../AppHeader.vue'
import { useAppStore, useAuthStore } from '@/stores'

vi.mock('vue-router', () => ({
  useRoute: () => ({ meta: { title: 'Dashboard' }, name: 'Dashboard', params: {} }),
  useRouter: () => ({ push: vi.fn() }),
}))

vi.mock('vue-i18n', () => ({
  createI18n: () => ({
    global: {
      locale: { value: 'zh' },
      t: (key: string) => key,
      setLocaleMessage: vi.fn(),
    },
  }),
  useI18n: () => ({
    t: (key: string) => key,
  }),
}))

vi.mock('@/i18n', () => ({
  i18n: {
    global: {
      t: (key: string) => key,
    },
  },
}))

describe('AppHeader documentation link', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
  })

  it('hides the docs link when the configured documentation URL is empty', () => {
    const appStore = useAppStore()
    const authStore = useAuthStore()
    appStore.docUrl = ''
    authStore.user = {
      id: 1,
      username: 'user',
      email: 'user@example.com',
      role: 'user',
      balance: 0,
      concurrency: 1,
      status: 'active',
      allowed_groups: null,
      balance_notify_enabled: false,
      balance_notify_threshold: null,
      balance_notify_extra_emails: [],
      created_at: '2026-01-01T00:00:00Z',
      updated_at: '2026-01-01T00:00:00Z',
    }

    const wrapper = mount(AppHeader, {
      global: {
        stubs: {
          AnnouncementBell: true,
          Icon: { template: '<span />' },
          LocaleSwitcher: true,
          SubscriptionProgressMini: true,
          RouterLink: true,
        },
      },
    })

    expect(wrapper.find('a[href="https://docs.devrouter.dev/"]').exists()).toBe(false)
    expect(wrapper.text()).not.toContain('nav.docs')
  })
})
