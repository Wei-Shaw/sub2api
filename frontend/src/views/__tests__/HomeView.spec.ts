import { readFileSync } from 'node:fs'
import { join } from 'node:path'
import { mount } from '@vue/test-utils'
import { defineComponent } from 'vue'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import HomeView from '@/views/HomeView.vue'

const root = process.cwd()
const readSource = (path: string) => readFileSync(join(root, path), 'utf8')

const mockStores = vi.hoisted(() => ({
  authStore: {
    isAuthenticated: false,
    isAdmin: false,
    user: null as null | { email: string },
    checkAuth: vi.fn(),
  },
  appStore: {
    cachedPublicSettings: null as null | Record<string, string>,
    siteName: 'Sub2API',
    siteLogo: '',
    docUrl: '',
    publicSettingsLoaded: true,
    fetchPublicSettings: vi.fn(),
  },
}))

vi.mock('@/stores', () => ({
  useAuthStore: () => mockStores.authStore,
  useAppStore: () => mockStores.appStore,
}))

vi.mock('@/components/common/LocaleSwitcher.vue', () => ({
  default: {
    name: 'LocaleSwitcher',
    template: '<div />',
  },
}))

vi.mock('vue-i18n', () => ({
  useI18n: () => ({
    t: (key: string) => key,
  }),
}))

const RouterLinkStub = defineComponent({
  name: 'RouterLink',
  props: {
    to: {
      type: [String, Object],
      required: true,
    },
  },
  computed: {
    href(): string {
      const target = this.to as string | { path?: string }
      return typeof target === 'string' ? target : target.path || ''
    },
  },
  template: '<a :href="href"><slot /></a>',
})

const mountHomeView = () => mount(HomeView, {
  global: {
    stubs: {
      RouterLink: RouterLinkStub,
      LocaleSwitcher: true,
      Icon: true,
    },
  },
})

describe('HomeView', () => {
  beforeEach(() => {
    mockStores.authStore.isAuthenticated = false
    mockStores.authStore.isAdmin = false
    mockStores.authStore.user = null
    mockStores.authStore.checkAuth.mockClear()
    mockStores.appStore.cachedPublicSettings = null
    mockStores.appStore.siteName = 'Sub2API'
    mockStores.appStore.siteLogo = ''
    mockStores.appStore.docUrl = ''
    mockStores.appStore.publicSettingsLoaded = true
    mockStores.appStore.fetchPublicSettings.mockClear()
    localStorage.setItem('theme', 'light')
    window.matchMedia = vi.fn().mockReturnValue({
      matches: false,
      addEventListener: vi.fn(),
      removeEventListener: vi.fn(),
    })
  })

  it('未登录首页只保留 Hero 区一个登录入口', () => {
    const wrapper = mountHomeView()
    const loginLinks = wrapper.findAll('a[href="/login"]')

    expect(loginLinks).toHaveLength(1)
  })

  it('中文首页文案展示精简套餐、时长、日限额和刷新规则', () => {
    const source = readSource('src/i18n/locales/zh.ts')

    expect(source).toContain("getStarted: '立即登录'")
    expect(source).toContain("unifiedGateway: '29 元套餐'")
    expect(source).toContain('月度订阅-时间 30天，日限额 19刀，24点刷新')
    expect(source).toContain("multiAccount: '39 元套餐'")
    expect(source).toContain('月度订阅-时间 30天，日限额 29刀，24点刷新')
    expect(source).toContain("balanceQuota: '59 元套餐'")
    expect(source).toContain('月度订阅-时间 30天，日限额 49刀，24点刷新')
    expect(source).not.toContain("getStarted: '立即开始'")
    expect(source).not.toContain("realtimeBilling: '按量计费'")
    expect(source).not.toContain('使用 cc-switch 项目，一键接入 API 到 Codex')
  })

  it('只展示当前支持的模型', () => {
    const source = readSource('src/views/HomeView.vue')

    expect(source).toContain('GPT 5.3')
    expect(source).toContain('Codex 5.4')
    expect(source).toContain('GPT 5.5')
    expect(source).not.toContain('home.providers.claude')
    expect(source).not.toContain('home.providers.gemini')
    expect(source).not.toContain('home.providers.antigravity')
  })
})
