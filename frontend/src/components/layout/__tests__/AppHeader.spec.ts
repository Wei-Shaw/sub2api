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
    t: (key: string) => ({
      'common.status': '状态',
      'nav.channelStatus': '服务状态',
      'announcements.title': '公告',
    }[key] ?? key),
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

describe('AppHeader service status shortcut', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
  })

  it('renders a service status icon before announcements and opens /monitor in a new tab', async () => {
    const openSpy = vi.spyOn(window, 'open').mockImplementation(() => null)
    const appStore = useAppStore()
    const authStore = useAuthStore()
    appStore.cachedPublicSettings = { channel_monitor_enabled: true }
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
          AnnouncementBell: { template: '<button class="announcement-bell" />' },
          Icon: { props: ['name'], template: '<span :data-icon="name" />' },
          LocaleSwitcher: true,
          SubscriptionProgressMini: true,
          RouterLink: true,
        },
      },
    })

    const statusButton = wrapper.find('.app-header-service-status')
    expect(statusButton.exists()).toBe(true)
    expect(statusButton.find('[data-icon="activity"]').exists()).toBe(true)
    expect(statusButton.text()).toContain('状态')
    expect(wrapper.find('.app-header-service-status').element.compareDocumentPosition(
      wrapper.find('.announcement-bell').element,
    ) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()

    await statusButton.trigger('click')

    expect(openSpy).toHaveBeenCalledWith('/monitor', '_blank', 'noopener,noreferrer')
    openSpy.mockRestore()
  })

  it('hides the service status shortcut when channel monitor is disabled', () => {
    const appStore = useAppStore()
    const authStore = useAuthStore()
    appStore.cachedPublicSettings = { channel_monitor_enabled: false }
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
          AnnouncementBell: { template: '<button class="announcement-bell" />' },
          Icon: { props: ['name'], template: '<span :data-icon="name" />' },
          LocaleSwitcher: true,
          SubscriptionProgressMini: true,
          RouterLink: true,
        },
      },
    })

    expect(wrapper.find('.app-header-service-status').exists()).toBe(false)
    expect(wrapper.find('.announcement-bell').exists()).toBe(true)
  })
})
