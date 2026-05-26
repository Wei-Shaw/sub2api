import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createMemoryHistory, createRouter } from 'vue-router'
import { defineComponent as defineVueComponent, h } from 'vue'
import LoginView from '@/views/auth/LoginView.vue'

const {
  authStoreState,
  appStoreState,
  getPublicSettingsMock,
  showSuccessMock,
  showErrorMock,
  showWarningMock,
} = vi.hoisted(() => ({
  authStoreState: {
    login: vi.fn(),
    login2FA: vi.fn(),
  },
  appStoreState: {
    showSuccess: vi.fn(),
    showError: vi.fn(),
    showWarning: vi.fn(),
  },
  getPublicSettingsMock: vi.fn(),
  showSuccessMock: vi.fn(),
  showErrorMock: vi.fn(),
  showWarningMock: vi.fn(),
}))

vi.mock('@/stores', () => ({
  useAuthStore: () => authStoreState,
  useAppStore: () => appStoreState,
}))

vi.mock('@/api/auth', async () => {
  const actual = await vi.importActual<typeof import('@/api/auth')>('@/api/auth')
  return {
    ...actual,
    getPublicSettings: (...args: any[]) => getPublicSettingsMock(...args),
  }
})

vi.mock('vue-i18n', () => ({
  createI18n: () => ({
    global: {
      t: (key: string) => key,
    },
  }),
  useI18n: () => ({
    t: (key: string) => key,
  }),
}))

const AuthLayoutStub = defineVueComponent({
  name: 'AuthLayoutStub',
  template: '<div><slot /><slot name="footer" /></div>',
})

const TotpLoginModalStub = defineVueComponent({
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

const TestShell = defineVueComponent({
  template: '<router-view />',
})

function createTestRouter() {
  return createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: '/login', component: LoginView },
      { path: '/purchase', component: { template: '<div data-testid="purchase-view">purchase</div>' } },
    ],
  })
}

describe('LoginView redirect flow', () => {
  beforeEach(() => {
    authStoreState.login.mockReset()
    authStoreState.login2FA.mockReset()
    getPublicSettingsMock.mockReset()
    showSuccessMock.mockReset()
    showErrorMock.mockReset()
    showWarningMock.mockReset()
    appStoreState.showSuccess = showSuccessMock
    appStoreState.showError = showErrorMock
    appStoreState.showWarning = showWarningMock
    sessionStorage.clear()
    localStorage.clear()
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
  })

  it('logs in and returns the user to the purchase page from redirect', async () => {
    const router = createTestRouter()
    authStoreState.login.mockResolvedValue({
      access_token: 'token-1',
      refresh_token: 'refresh-1',
      expires_in: 3600,
      token_type: 'Bearer',
      user: {
        id: 1,
        email: 'demo@example.com',
        username: 'demo',
        role: 'user',
      },
    })

    await router.push('/login?redirect=/purchase?plan=8')

    const wrapper = mount(TestShell, {
      global: {
        plugins: [router],
        stubs: {
          AuthLayout: AuthLayoutStub,
          Icon: true,
          TurnstileWidget: true,
          LoginAgreementPrompt: true,
          EmailOAuthButtons: true,
          LinuxDoOAuthSection: true,
          DingTalkOAuthSection: true,
          WechatOAuthSection: true,
          OidcOAuthSection: true,
          TotpLoginModal: TotpLoginModalStub,
          'router-link': true,
        },
      },
    })

    await flushPromises()

    await wrapper.get('#email').setValue('demo@example.com')
    await wrapper.get('#password').setValue('secret-123')
    await wrapper.get('form').trigger('submit.prevent')
    await flushPromises()

    expect(authStoreState.login).toHaveBeenCalledWith({
      email: 'demo@example.com',
      password: 'secret-123',
      turnstile_token: undefined,
    })
    expect(showSuccessMock).toHaveBeenCalledWith('auth.loginSuccess')
    expect(router.currentRoute.value.path).toBe('/purchase')
    expect(router.currentRoute.value.query.plan).toBe('8')
    expect(wrapper.find('[data-testid="purchase-view"]').exists()).toBe(true)

    wrapper.unmount()
  })

  it('completes 2FA login and returns the user to the purchase page from redirect', async () => {
    const router = createTestRouter()
    authStoreState.login.mockResolvedValue({
      requires_2fa: true,
      temp_token: 'temp-2fa-token',
      user_email_masked: 'd***@example.com',
    })
    authStoreState.login2FA.mockResolvedValue({
      id: 1,
      email: 'demo@example.com',
      username: 'demo',
      role: 'user',
    })

    await router.push('/login?redirect=/purchase?plan=8')

    const wrapper = mount(TestShell, {
      global: {
        plugins: [router],
        stubs: {
          AuthLayout: AuthLayoutStub,
          Icon: true,
          TurnstileWidget: true,
          LoginAgreementPrompt: true,
          EmailOAuthButtons: true,
          LinuxDoOAuthSection: true,
          DingTalkOAuthSection: true,
          WechatOAuthSection: true,
          OidcOAuthSection: true,
          TotpLoginModal: TotpLoginModalStub,
          'router-link': true,
        },
      },
    })

    await flushPromises()

    await wrapper.get('#email').setValue('demo@example.com')
    await wrapper.get('#password').setValue('secret-123')
    await wrapper.get('form').trigger('submit.prevent')
    await flushPromises()

    expect(wrapper.find('[data-testid="totp-modal"]').exists()).toBe(true)

    await wrapper.get('[data-testid="totp-verify"]').trigger('click')
    await flushPromises()

    expect(authStoreState.login2FA).toHaveBeenCalledWith('temp-2fa-token', '123456')
    expect(showSuccessMock).toHaveBeenCalledWith('auth.loginSuccess')
    expect(router.currentRoute.value.path).toBe('/purchase')
    expect(router.currentRoute.value.query.plan).toBe('8')
    expect(wrapper.find('[data-testid="purchase-view"]').exists()).toBe(true)

    wrapper.unmount()
  })
})
