import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import RedeemView from '../RedeemView.vue'

const mocks = vi.hoisted(() => ({
  getHistory: vi.fn(),
  getPublicSettings: vi.fn(),
  redeem: vi.fn(),
  refreshUser: vi.fn(),
  fetchActiveSubscriptions: vi.fn(),
  showError: vi.fn(),
  showSuccess: vi.fn(),
  showWarning: vi.fn(),
}))

vi.mock('vue-i18n', async (importOriginal) => ({
  ...(await importOriginal<typeof import('vue-i18n')>()),
  useI18n: () => ({ t: (key: string) => key }),
}))

vi.mock('@/api', () => ({
  redeemAPI: {
    getHistory: mocks.getHistory,
    redeem: mocks.redeem,
  },
  authAPI: {
    getPublicSettings: mocks.getPublicSettings,
  },
}))

vi.mock('@/stores/auth', () => ({
  useAuthStore: () => ({
    user: { balance: 20, concurrency: 5 },
    refreshUser: mocks.refreshUser,
  }),
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({
    showError: mocks.showError,
    showSuccess: mocks.showSuccess,
    showWarning: mocks.showWarning,
  }),
}))

vi.mock('@/stores/subscriptions', () => ({
  useSubscriptionStore: () => ({
    fetchActiveSubscriptions: mocks.fetchActiveSubscriptions,
  }),
}))

function mountView() {
  return mount(RedeemView, {
    global: {
      stubs: {
        AppLayout: { template: '<main><slot /></main>' },
      },
    },
  })
}

describe('RedeemView responsive shop layout', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mocks.getHistory.mockResolvedValue([])
    mocks.getPublicSettings.mockResolvedValue({
      contact_info: '',
      purchase_subscription_enabled: true,
      purchase_subscription_url: 'https://shop.example.com/',
    })
  })

  it('uses a wider shop column on the left and keeps the redeem column bounded on desktop', async () => {
    const wrapper = mountView()
    await flushPromises()

    const shopPanel = wrapper.get('#redeem-panel-shop')
    const redeemPanel = wrapper.get('#redeem-panel-code')
    const layout = shopPanel.element.parentElement as HTMLElement

    expect(layout.className).toContain(
      'lg:grid-cols-[minmax(0,1fr)_minmax(420px,560px)]',
    )
    expect(layout.firstElementChild).toBe(shopPanel.element)
    expect(layout.lastElementChild).toBe(redeemPanel.element)
    expect(shopPanel.classes()).toContain('lg:sticky')
    expect(shopPanel.get('iframe').attributes('src')).toBe('https://shop.example.com/')
    expect(shopPanel.get('.shop-shell').classes()).toContain('flex')
  })

  it('opens recharge by default on mobile and switches to the redeem panel', async () => {
    const wrapper = mountView()
    await flushPromises()

    const shopTab = wrapper.get('#redeem-tab-shop')
    const redeemTab = wrapper.get('#redeem-tab-code')

    expect(shopTab.attributes('aria-selected')).toBe('true')
    expect(redeemTab.attributes('aria-selected')).toBe('false')
    expect(wrapper.get('#redeem-panel-shop').classes()).toContain('block')
    expect(wrapper.get('#redeem-panel-code').classes()).toContain('hidden')

    await redeemTab.trigger('click')

    expect(shopTab.attributes('aria-selected')).toBe('false')
    expect(redeemTab.attributes('aria-selected')).toBe('true')
    expect(wrapper.get('#redeem-panel-shop').classes()).toContain('hidden')
    expect(wrapper.get('#redeem-panel-code').classes()).toContain('block')
  })

  it('keeps the original single-column redeem view when no shop is configured', async () => {
    mocks.getPublicSettings.mockResolvedValue({
      contact_info: '',
      purchase_subscription_enabled: false,
      purchase_subscription_url: '',
    })

    const wrapper = mountView()
    await flushPromises()

    expect(wrapper.find('[role="tablist"]').exists()).toBe(false)
    expect(wrapper.find('#redeem-panel-shop').exists()).toBe(false)
    expect(wrapper.get('#redeem-panel-code').classes()).toContain('block')
    expect(wrapper.get('#redeem-panel-code').attributes('role')).toBeUndefined()
  })
})
