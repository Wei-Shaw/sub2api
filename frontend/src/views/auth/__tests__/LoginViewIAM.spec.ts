import { flushPromises, shallowMount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import LoginView from '@/views/auth/LoginView.vue'
import IAMLoginView from '@/views/auth/IAMLoginView.vue'

const mocks = vi.hoisted(() => ({
  getPublicSettings: vi.fn(),
  appStore: {
    cachedPublicSettings: null as null | { company_iam_enabled?: boolean },
    showError: vi.fn(),
    showSuccess: vi.fn(),
    showWarning: vi.fn(),
  },
  authStore: { login: vi.fn(), loginIAM: vi.fn() },
  routerReplace: vi.fn(),
}))

vi.mock('@/api/auth', () => ({
  getPublicSettings: mocks.getPublicSettings,
  isTotp2FARequired: () => false,
  isWeChatWebOAuthEnabled: () => false,
}))

vi.mock('@/stores', () => ({
  useAppStore: () => mocks.appStore,
  useAuthStore: () => mocks.authStore,
}))

vi.mock('vue-router', () => ({
  useRouter: () => ({
    currentRoute: { value: { query: {} } },
    replace: mocks.routerReplace,
    resolve: (path: string) => ({ name: path.startsWith('/oidc') ? 'NotFound' : 'Route' }),
  }),
  useRoute: () => ({ query: {} }),
}))

vi.mock('vue-i18n', async importOriginal => {
  const actual = await importOriginal<typeof import('vue-i18n')>()
  return {
    ...actual,
    useI18n: () => ({ t: (key: string) => key }),
  }
})

function publicSettings(companyIAMEnabled: boolean) {
  return {
    company_iam_enabled: companyIAMEnabled,
    captcha_enabled: false,
    turnstile_enabled: false,
    captcha_provider: 'turnstile',
    captcha_site_key: '',
    turnstile_site_key: '',
    linuxdo_oauth_enabled: false,
    dingtalk_oauth_enabled: false,
    backend_mode_enabled: false,
    oidc_oauth_enabled: false,
    oidc_oauth_provider_name: 'OIDC',
    github_oauth_enabled: false,
    google_oauth_enabled: false,
    password_reset_enabled: false,
  }
}

async function mountLogin() {
  const wrapper = shallowMount(LoginView, {
    global: {
      stubs: {
        AuthLayout: { template: '<div><slot /></div>' },
        RouterLink: {
          props: ['to'],
          template: '<a :data-to="to"><slot /></a>',
        },
      },
    },
  })
  await flushPromises()
  return wrapper
}

describe('LoginView IAM entry', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mocks.appStore.cachedPublicSettings = null
    mocks.getPublicSettings.mockReset()
    mocks.getPublicSettings.mockResolvedValue(publicSettings(false))
  })

  it('hides IAM login while company IAM is disabled', async () => {
    mocks.getPublicSettings.mockResolvedValue(publicSettings(false))

    const wrapper = await mountLogin()

    expect(wrapper.find('[data-to="/iam-login"]').exists()).toBe(false)
  })

  it('shows IAM login when company IAM is enabled', async () => {
    mocks.getPublicSettings.mockResolvedValue(publicSettings(true))

    const wrapper = await mountLogin()

    expect(wrapper.find('[data-to="/iam-login"]').exists()).toBe(true)
  })

  it('submits the complete IAM principal without a separate account ID', async () => {
    mocks.authStore.loginIAM.mockResolvedValue({ user: { must_change_password: false } })
    const wrapper = shallowMount(IAMLoginView, {
      global: {
        stubs: {
          AuthLayout: { template: '<div><slot /></div>' },
          RouterLink: { props: ['to'], template: '<a :data-to="to"><slot /></a>' },
        },
      },
    })
    await flushPromises()

    expect(wrapper.find('#iam-account-id').exists()).toBe(false)
    await wrapper.get('#iam-principal').setValue('reader@c123456789012345.opentk.ai')
    await wrapper.get('#iam-password').setValue('secret-password')
    await wrapper.get('form').trigger('submit')
    await flushPromises()

    expect(mocks.authStore.loginIAM).toHaveBeenCalledWith({
      principal: 'reader@c123456789012345.opentk.ai',
      password: 'secret-password',
      captcha_payload: undefined,
    })
    expect(mocks.routerReplace).toHaveBeenCalledWith('/dashboard')
  })

  it('stays on IAM login and shows authentication failures in a toast', async () => {
    mocks.authStore.loginIAM.mockRejectedValue(new Error('invalid credentials'))
    const wrapper = shallowMount(IAMLoginView, {
      global: {
        stubs: {
          AuthLayout: { template: '<div><slot /></div>' },
          RouterLink: { props: ['to'], template: '<a :data-to="to"><slot /></a>' },
        },
      },
    })
    await flushPromises()

    await wrapper.get('#iam-principal').setValue('reader@c123456789012345.opentk.ai')
    await wrapper.get('#iam-password').setValue('wrong-password')
    await wrapper.get('form').trigger('submit')
    await flushPromises()

    expect(wrapper.text()).not.toContain('organization.login.genericError')
    expect(mocks.appStore.showError).toHaveBeenCalledWith('invalid credentials')
    expect(mocks.routerReplace).not.toHaveBeenCalled()
  })

  it('requires and submits the configured captcha for IAM login', async () => {
    mocks.getPublicSettings.mockResolvedValue({
      ...publicSettings(true),
      captcha_enabled: true,
      turnstile_enabled: true,
      captcha_site_key: 'site-key',
      turnstile_site_key: 'site-key',
    })
    mocks.authStore.loginIAM.mockResolvedValue({ user: { must_change_password: false } })
    const wrapper = shallowMount(IAMLoginView, {
      global: {
        stubs: {
          AuthLayout: { template: '<div><slot /></div>' },
          RouterLink: { props: ['to'], template: '<a :data-to="to"><slot /></a>' },
        },
      },
    })
    await flushPromises()

    const captcha = wrapper.findComponent({ name: 'CaptchaWidget' })
    expect(captcha.exists()).toBe(true)
    await captcha.vm.$emit('verify', 'captcha-token')
    await wrapper.get('#iam-principal').setValue('reader@c123456789012345.opentk.ai')
    await wrapper.get('#iam-password').setValue('secret-password')
    await wrapper.get('form').trigger('submit')
    await flushPromises()

    expect(mocks.authStore.loginIAM).toHaveBeenCalledWith({
      principal: 'reader@c123456789012345.opentk.ai',
      password: 'secret-password',
      captcha_payload: { token: 'captcha-token' },
    })
  })
})
