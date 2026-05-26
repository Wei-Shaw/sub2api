import { mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { defineComponent, h } from 'vue'

const { adminSettingsStore, appStore, authStore, onboardingStore, routeState, routerPush } = vi.hoisted(() => ({
  adminSettingsStore: {
    customMenuItems: [] as Array<{ id: string; label: string; icon_svg: string; visibility: string; sort_order: number }>,
    opsMonitoringEnabled: false,
    paymentEnabled: true,
    fetch: vi.fn(),
  },
  appStore: {
    sidebarCollapsed: false,
    mobileOpen: false,
    siteName: 'Sub2API',
    siteLogo: '',
    siteVersion: '',
    publicSettingsLoaded: true,
    backendModeEnabled: false,
    cachedPublicSettings: {
      available_channels_enabled: false,
      custom_menu_items: [],
      channel_monitor_enabled: true,
      payment_enabled: true,
      affiliate_enabled: false,
      risk_control_enabled: false,
    } as Record<string, unknown>,
    toggleSidebar: vi.fn(),
    setMobileOpen: vi.fn(),
  },
  authStore: {
    isAdmin: false,
    isSimpleMode: false,
  },
  onboardingStore: {
    isCurrentStep: vi.fn(() => false),
    nextStep: vi.fn(),
  },
  routeState: {
    path: '/dashboard',
  },
  routerPush: vi.fn(),
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  const messages: Record<string, string> = {
    'nav.dashboard': '仪表盘',
    'nav.apiKeys': 'API Keys',
    'nav.usage': 'Usage',
    'nav.modelMarketplace': '模型广场',
    'nav.availableChannels': '可用渠道',
    'nav.channelStatus': '渠道状态',
    'nav.mySubscriptions': '我的订阅',
    'nav.buySubscription': '购买订阅',
    'nav.myOrders': '我的订单',
    'nav.redeem': '兑换',
    'nav.affiliate': '推广返利',
    'nav.profile': '个人资料',
    'nav.lightMode': '浅色模式',
    'nav.darkMode': '深色模式',
    'nav.expand': '展开',
    'nav.collapse': '收起',
    'nav.myAccount': '我的账户',
  }
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string) => messages[key] ?? key,
    }),
  }
})

vi.mock('vue-router', async () => {
  const actual = await vi.importActual<typeof import('vue-router')>('vue-router')
  return {
    ...actual,
    useRoute: () => routeState,
    useRouter: () => ({
      push: routerPush,
    }),
  }
})

vi.mock('@/stores', () => ({
  useAdminSettingsStore: () => adminSettingsStore,
  useAppStore: () => appStore,
  useAuthStore: () => authStore,
  useOnboardingStore: () => onboardingStore,
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => appStore,
}))

import AppSidebar from '../AppSidebar.vue'

const RouterLinkStub = defineComponent({
  name: 'RouterLinkStub',
  props: {
    to: {
      type: [String, Object],
      required: true,
    },
  },
  setup(props, { slots }) {
    return () => h('a', { href: typeof props.to === 'string' ? props.to : '#' }, slots.default?.())
  },
})

function mountSidebar() {
  return mount(AppSidebar, {
    global: {
      stubs: {
        VersionBadge: true,
        RouterLink: RouterLinkStub,
        'router-link': RouterLinkStub,
      },
    },
  })
}

describe('AppSidebar model marketplace entry', () => {
  beforeEach(() => {
    adminSettingsStore.fetch.mockReset()
    appStore.cachedPublicSettings = {
      available_channels_enabled: false,
      custom_menu_items: [],
      channel_monitor_enabled: true,
      payment_enabled: true,
      affiliate_enabled: false,
      risk_control_enabled: false,
    }
    routeState.path = '/dashboard'
    authStore.isAdmin = false
    authStore.isSimpleMode = false
    appStore.backendModeEnabled = false
    vi.stubGlobal('matchMedia', vi.fn(() => ({
      matches: false,
      media: '',
      onchange: null,
      addListener: vi.fn(),
      removeListener: vi.fn(),
      addEventListener: vi.fn(),
      removeEventListener: vi.fn(),
      dispatchEvent: vi.fn(),
    })))
    localStorage.clear()
    document.documentElement.classList.remove('dark')
  })

  it('shows the model marketplace link when available channels are enabled', () => {
    appStore.cachedPublicSettings = {
      ...appStore.cachedPublicSettings,
      available_channels_enabled: true,
    }

    const wrapper = mountSidebar()

    expect(wrapper.text()).toContain('模型广场')
  })

  it('hides the model marketplace link when available channels are disabled', () => {
    const wrapper = mountSidebar()

    expect(wrapper.text()).not.toContain('模型广场')
  })
})
