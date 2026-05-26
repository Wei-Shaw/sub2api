import { flushPromises, mount } from '@vue/test-utils'
import { defineComponent, h } from 'vue'
import { beforeEach, describe, expect, it, vi } from 'vitest'

const {
  authStoreState,
  appStoreState,
  adminSettingsStoreState,
  getModelMarketplaceMock,
  getPublicSettingsMock,
  fetchPublicSettingsMock,
  checkAuthMock,
} = vi.hoisted(() => ({
  authStoreState: {
    isAuthenticated: false,
    isAdmin: false,
    isSimpleMode: false,
    hasPendingAuthSession: false,
    checkAuth: vi.fn(),
    login: vi.fn(),
    login2FA: vi.fn(),
  },
  appStoreState: {
    siteName: 'Sub2API',
    backendModeEnabled: false,
    cachedPublicSettings: {
      model_marketplace_public_enabled: true,
      payment_enabled: true,
      custom_menu_items: [],
    } as Record<string, unknown>,
    fetchPublicSettings: vi.fn(),
    showError: vi.fn(),
    showSuccess: vi.fn(),
    showWarning: vi.fn(),
  },
  adminSettingsStoreState: {
    customMenuItems: [],
    fetch: vi.fn(),
  },
  getModelMarketplaceMock: vi.fn(),
  getPublicSettingsMock: vi.fn(),
  fetchPublicSettingsMock: vi.fn(),
  checkAuthMock: vi.fn(),
}))

vi.mock('@/stores/auth', () => ({
  useAuthStore: () => authStoreState,
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => appStoreState,
}))

vi.mock('@/stores/adminSettings', () => ({
  useAdminSettingsStore: () => adminSettingsStoreState,
}))

vi.mock('@/stores', () => ({
  useAuthStore: () => authStoreState,
  useAppStore: () => appStoreState,
  useAdminSettingsStore: () => adminSettingsStoreState,
}))

vi.mock('@/api/modelMarketplace', () => ({
  default: {
    getModelMarketplace: getModelMarketplaceMock,
  },
  getModelMarketplace: getModelMarketplaceMock,
}))

vi.mock('@/api/setup', () => ({
  getSetupStatus: vi.fn(),
}))

vi.mock('@/api/auth', async () => {
  const actual = await vi.importActual<typeof import('@/api/auth')>('@/api/auth')
  return {
    ...actual,
    getPublicSettings: (...args: any[]) => getPublicSettingsMock(...args),
  }
})

vi.mock('@/composables/useNavigationLoading', () => ({
  useNavigationLoadingState: () => ({
    startNavigation: vi.fn(),
    endNavigation: vi.fn(),
    isLoading: { value: false },
  }),
}))

vi.mock('@/composables/useRoutePrefetch', () => ({
  useRoutePrefetch: () => ({
    triggerPrefetch: vi.fn(),
    cancelPendingPrefetch: vi.fn(),
    resetPrefetchState: vi.fn(),
  }),
}))

vi.mock('@/i18n', () => ({
  i18n: {
    global: {
      t: (key: string) => key,
    },
  },
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    createI18n: () => ({
      global: {
        t: (key: string) => key,
      },
    }),
    useI18n: () => ({
      t: (key: string, params?: Record<string, string> | string) => {
        if (!params || typeof params === 'string') return key
        return key.replace(/\{(\w+)\}/g, (_, token) => params[token] ?? `{${token}}`)
      },
    }),
  }
})

vi.mock('@/views/user/PaymentView.vue', () => ({
  default: {
    template: '<div data-testid="payment-view">Payment</div>',
  },
}))

const AuthLayoutStub = defineComponent({
  name: 'AuthLayoutStub',
  template: '<div><slot /><slot name="footer" /></div>',
})

const TotpLoginModalStub = defineComponent({
  name: 'TotpLoginModalStub',
  emits: ['verify', 'cancel'],
  setup(_, { emit, expose }) {
    expose({
      setVerifying: vi.fn(),
      setError: vi.fn(),
    })
    return () =>
      h('div', { 'data-testid': 'totp-modal' }, [
        h(
          'button',
          {
            type: 'button',
            'data-testid': 'totp-verify',
            onClick: () => emit('verify', '123456'),
          },
          'verify',
        ),
      ])
  },
})

const RouterHost = defineComponent({
  template: '<router-view />',
})

const marketplaceResponse = {
  auth: {
    authenticated: false,
  },
  models: [
    {
      name: 'claude-3-5-sonnet',
      platform: 'anthropic',
      pricing: {
        billing_mode: 'per_request',
        input_price: null,
        output_price: null,
        cache_write_price: null,
        cache_read_price: null,
        image_output_price: null,
        per_request_price: 0.25,
        intervals: [],
      },
      channels: [{ name: 'Premium', description: '' }],
      groups: [
        {
          id: 2,
          name: 'Pro',
          platform: 'anthropic',
          subscription_type: 'monthly',
          rate_multiplier: 1,
          user_rate_multiplier: null,
          is_exclusive: false,
          access_state: 'purchasable',
          active_subscription: null,
          plans: [
            {
              id: 8,
              group_id: 2,
              name: 'Pro Plan',
              description: '',
              price: 9.9,
              original_price: null,
              validity_days: 30,
              validity_unit: 'day',
              features: [],
              product_name: 'pro-plan',
              sort_order: 1,
            },
          ],
        },
      ],
      access_state: 'purchasable',
    },
  ],
}

function setViewportWidth(width: number) {
  Object.defineProperty(window, 'innerWidth', {
    configurable: true,
    writable: true,
    value: width,
  })
}

describe('model marketplace router integration', () => {
  beforeEach(() => {
    vi.resetModules()
    window.history.pushState({}, '', '/models')
    setViewportWidth(1280)
    vi.stubGlobal('scrollTo', vi.fn())

    authStoreState.isAuthenticated = false
    authStoreState.isAdmin = false
    authStoreState.isSimpleMode = false
    authStoreState.hasPendingAuthSession = false
    authStoreState.checkAuth = checkAuthMock
    authStoreState.login.mockReset()
    authStoreState.login2FA.mockReset()
    checkAuthMock.mockReset()

    appStoreState.backendModeEnabled = false
    appStoreState.cachedPublicSettings = {
      model_marketplace_public_enabled: true,
      payment_enabled: true,
      custom_menu_items: [],
    }
    appStoreState.fetchPublicSettings = fetchPublicSettingsMock
    fetchPublicSettingsMock.mockReset()
    fetchPublicSettingsMock.mockResolvedValue(appStoreState.cachedPublicSettings)
    appStoreState.showError.mockReset()
    appStoreState.showSuccess.mockReset()
    appStoreState.showWarning.mockReset()

    getPublicSettingsMock.mockReset()
    getPublicSettingsMock.mockResolvedValue({
      turnstile_enabled: false,
      turnstile_site_key: '',
      linuxdo_oauth_enabled: false,
      dingtalk_oauth_enabled: false,
      wechat_oauth_enabled: false,
      backend_mode_enabled: false,
      oidc_oauth_enabled: false,
      github_oauth_enabled: false,
      google_oauth_enabled: false,
      password_reset_enabled: false,
      login_agreement_enabled: false,
      login_agreement_documents: [],
    })

    adminSettingsStoreState.customMenuItems = []
    adminSettingsStoreState.fetch.mockReset()

    getModelMarketplaceMock.mockReset()
    getModelMarketplaceMock.mockResolvedValue(marketplaceResponse)
  })

  it('uses the real router guard to redirect guest purchase navigation to login with redirect query', async () => {
    const { default: router } = await import('@/router')

    const wrapper = mount(RouterHost, {
      global: {
        plugins: [router],
        stubs: {
          AuthLayout: AuthLayoutStub,
          Icon: true,
          PlatformIcon: true,
          GroupBadge: true,
          SupportedModelChip: true,
          TurnstileWidget: true,
          LoginAgreementPrompt: true,
          EmailOAuthButtons: true,
          LinuxDoOAuthSection: true,
          DingTalkOAuthSection: true,
          WechatOAuthSection: true,
          OidcOAuthSection: true,
          TotpLoginModal: TotpLoginModalStub,
          RouterLink: true,
          'router-link': true,
        },
      },
    })

    await router.isReady()
    await flushPromises()

    expect(router.currentRoute.value.path).toBe('/models')
    expect(getModelMarketplaceMock).toHaveBeenCalledTimes(1)

    await wrapper.get('[data-testid="marketplace-plan-button-8"]').trigger('click')
    await vi.dynamicImportSettled()
    await flushPromises()

    expect(router.currentRoute.value.path).toBe('/login')
    expect(router.currentRoute.value.query.redirect).toBe('/purchase?plan=8')
    expect(router.currentRoute.value.fullPath).toBe('/login?redirect=/purchase?plan=8')
    expect(wrapper.find('#email').exists()).toBe(true)

    wrapper.unmount()
  })

  it('completes the marketplace purchase redirect after a 2FA login on the real router', async () => {
    authStoreState.login.mockResolvedValue({
      requires_2fa: true,
      temp_token: 'temp-2fa-token',
      user_email_masked: 'd***@example.com',
    })
    authStoreState.login2FA.mockImplementation(async () => {
      authStoreState.isAuthenticated = true
      return {
        id: 1,
        email: 'demo@example.com',
        username: 'demo',
        role: 'user',
      }
    })

    const { default: router } = await import('@/router')

    const wrapper = mount(RouterHost, {
      global: {
        plugins: [router],
        stubs: {
          AuthLayout: AuthLayoutStub,
          Icon: true,
          PlatformIcon: true,
          GroupBadge: true,
          SupportedModelChip: true,
          TurnstileWidget: true,
          LoginAgreementPrompt: true,
          EmailOAuthButtons: true,
          LinuxDoOAuthSection: true,
          DingTalkOAuthSection: true,
          WechatOAuthSection: true,
          OidcOAuthSection: true,
          TotpLoginModal: TotpLoginModalStub,
          RouterLink: true,
          'router-link': true,
        },
      },
    })

    await router.isReady()
    await flushPromises()

    await wrapper.get('[data-testid="marketplace-plan-button-8"]').trigger('click')
    await vi.dynamicImportSettled()
    await flushPromises()

    expect(router.currentRoute.value.fullPath).toBe('/login?redirect=/purchase?plan=8')

    await wrapper.get('#email').setValue('demo@example.com')
    await wrapper.get('#password').setValue('secret-123')
    await wrapper.get('form').trigger('submit.prevent')
    await vi.dynamicImportSettled()
    await flushPromises()

    expect(wrapper.find('[data-testid="totp-modal"]').exists()).toBe(true)

    await wrapper.get('[data-testid="totp-verify"]').trigger('click')
    await vi.dynamicImportSettled()
    await flushPromises()

    expect(authStoreState.login).toHaveBeenCalledWith({
      email: 'demo@example.com',
      password: 'secret-123',
      turnstile_token: undefined,
    })
    expect(authStoreState.login2FA).toHaveBeenCalledWith('temp-2fa-token', '123456')
    expect(router.currentRoute.value.path).toBe('/purchase')
    expect(router.currentRoute.value.query.plan).toBe('8')
    expect(wrapper.find('[data-testid="payment-view"]').exists()).toBe(true)

    wrapper.unmount()
  })
})
