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
  }),
}))

vi.mock('vue-i18n', async importOriginal => {
  const actual = await importOriginal<typeof import('vue-i18n')>()
  return {
    ...actual,
    useI18n: () => ({ t: (key: string) => key }),
  }
})

vi.mock('@/composables/useCaptchaSubmit', () => ({
  useCaptchaSubmit: () => ({ submit: vi.fn() }),
}))

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

    expect(wrapper.find('#iam-account-id').exists()).toBe(false)
    await wrapper.get('#iam-principal').setValue('reader@1719905235756637.opentk.ai')
    await wrapper.get('#iam-password').setValue('secret-password')
    await wrapper.get('form').trigger('submit')
    await flushPromises()

    expect(mocks.authStore.loginIAM).toHaveBeenCalledWith({
      principal: 'reader@1719905235756637.opentk.ai',
      password: 'secret-password',
    })
    expect(mocks.routerReplace).toHaveBeenCalledWith('/dashboard')
  })
})
