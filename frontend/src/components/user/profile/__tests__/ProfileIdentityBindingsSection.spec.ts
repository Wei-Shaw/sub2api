import { mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import ProfileIdentityBindingsSection from '@/components/user/profile/ProfileIdentityBindingsSection.vue'
import { useAppStore, useAuthStore } from '@/stores'
import type { User } from '@/types'

const routeState = vi.hoisted(() => ({
  fullPath: '/profile',
}))

const locationState = vi.hoisted(() => ({
  current: { href: 'http://localhost/profile' } as { href: string },
}))

let pinia: ReturnType<typeof createPinia>

const userApiMocks = vi.hoisted(() => ({
  sendEmailBindingCode: vi.fn(),
  bindEmailIdentity: vi.fn(),
  unbindAuthIdentity: vi.fn(),
}))

vi.mock('vue-router', () => ({
  useRoute: () => routeState,
}))

vi.mock('@/api/user', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/api/user')>()
  return {
    ...actual,
    sendEmailBindingCode: (...args: any[]) => userApiMocks.sendEmailBindingCode(...args),
    bindEmailIdentity: (...args: any[]) => userApiMocks.bindEmailIdentity(...args),
    unbindAuthIdentity: (...args: any[]) => userApiMocks.unbindAuthIdentity(...args),
  }
})

vi.mock('vue-i18n', async (importOriginal) => {
  const actual = await importOriginal<typeof import('vue-i18n')>()
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string, params?: Record<string, string>) => {
        if (key === 'profile.authBindings.title') return 'Connected sign-in methods'
        if (key === 'profile.authBindings.description') return 'Manage bound providers'
        if (key === 'profile.authBindings.status.bound') return 'Bound'
        if (key === 'profile.authBindings.status.notBound') return 'Not bound'
        if (key === 'profile.authBindings.providers.email') return 'Email'
        if (key === 'profile.authBindings.providers.linuxdo') return 'LinuxDo'
        if (key === 'profile.authBindings.providers.wechat') return 'WeChat'
        if (key === 'profile.authBindings.providers.oidc') return params?.providerName || 'OIDC'
        if (key === 'profile.authBindings.bindAction') return `Bind ${params?.providerName || ''}`.trim()
        if (key === 'profile.authBindings.emailPlaceholder') return 'Email address'
        if (key === 'profile.authBindings.codePlaceholder') return 'Verification code'
        if (key === 'profile.authBindings.passwordPlaceholder') return 'Set password'
        if (key === 'profile.authBindings.replaceEmailPasswordPlaceholder')
          return 'Current password'
        if (key === 'profile.authBindings.sendCodeAction') return 'Send code'
        if (key === 'profile.authBindings.unbindAction') return 'Unbind'
        if (key === 'profile.authBindings.manageEmailAction') return 'Manage email'
        if (key === 'profile.authBindings.hideEmailFormAction') return 'Hide email form'
        if (key === 'profile.authBindings.confirmEmailBindAction') return 'Bind email'
        if (key === 'profile.authBindings.confirmEmailReplaceAction') return 'Replace primary email'
        if (key === 'profile.authBindings.codeSentTo') return `Code sent to ${params?.email || ''}`.trim()
        if (key === 'profile.authBindings.bindSuccess') return 'Bind success'
        if (key === 'profile.authBindings.replaceSuccess') return 'Primary email updated'
        if (key === 'profile.authBindings.notes.emailManagedFromProfile')
          return 'Primary email is managed in the profile form'
        if (key === 'profile.authBindings.notes.canUnbind')
          return 'You can unbind this sign-in method'
        if (key === 'profile.authBindings.notes.bindAnotherBeforeUnbind')
          return 'Bind another sign-in method before unbinding'
        return key
      },
    }),
  }
})

function createUser(overrides: Partial<User> = {}): User {
  return {
    id: 7,
    username: 'alice',
    email: 'alice@example.com',
    role: 'user',
    balance: 10,
    concurrency: 2,
    status: 'active',
    allowed_groups: null,
    balance_notify_enabled: true,
    balance_notify_threshold: null,
    balance_notify_extra_emails: [],
    created_at: '2026-04-20T00:00:00Z',
    updated_at: '2026-04-20T00:00:00Z',
    ...overrides,
  }
}

describe('ProfileIdentityBindingsSection', () => {
  beforeEach(() => {
    pinia = createPinia()
    setActivePinia(pinia)
    routeState.fullPath = '/profile'
    locationState.current = { href: 'http://localhost/profile' }
    Object.defineProperty(window, 'location', {
      configurable: true,
      value: locationState.current,
    })
    Object.defineProperty(window.navigator, 'userAgent', {
      configurable: true,
      value: 'Mozilla/5.0',
    })
    const appStore = useAppStore()
    appStore.cachedPublicSettings = null
    appStore.publicSettingsLoaded = false
    userApiMocks.sendEmailBindingCode.mockReset()
    userApiMocks.bindEmailIdentity.mockReset()
    userApiMocks.unbindAuthIdentity.mockReset()
  })

  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('renders provider binding states and provider-specific bind actions', () => {
    const wrapper = mount(ProfileIdentityBindingsSection, {
      global: {
        plugins: [pinia],
      },
      props: {
        user: createUser({
          auth_bindings: {
            email: { bound: true },
            linuxdo: { bound: true },
            oidc: { bound: false },
            wechat: false,
          },
        }),
        linuxdoEnabled: true,
        oidcEnabled: true,
        oidcProviderName: 'ExampleID',
        wechatEnabled: true,
        wechatOpenEnabled: true,
        wechatMpEnabled: false,
      },
    })

    expect(wrapper.get('[data-testid="profile-binding-email-status"]').text()).toBe('Bound')
    expect(wrapper.get('[data-testid="profile-binding-linuxdo-status"]').text()).toBe('Bound')
    expect(wrapper.get('[data-testid="profile-binding-oidc-status"]').text()).toBe('Not bound')
    expect(wrapper.get('[data-testid="profile-binding-oidc-action"]').text()).toBe(
      'Bind ExampleID'
    )
    expect(wrapper.get('[data-testid="profile-binding-wechat-action"]').text()).toBe('Bind WeChat')
  })

  it('starts the WeChat bind flow for the current profile page', async () => {
    const wrapper = mount(ProfileIdentityBindingsSection, {
      global: {
        plugins: [pinia],
      },
      props: {
        user: createUser(),
        linuxdoEnabled: false,
        oidcEnabled: false,
        wechatEnabled: true,
        wechatOpenEnabled: true,
        wechatMpEnabled: false,
      },
    })

    await wrapper.get('[data-testid="profile-binding-wechat-action"]').trigger('click')

    expect(locationState.current.href).toContain('/api/v1/auth/oauth/wechat/bind/start?')
    expect(locationState.current.href).toContain('mode=open')
    expect(locationState.current.href).toContain('intent=bind_current_user')
    expect(locationState.current.href).toContain('redirect=%2Fprofile')
  })

  it('hides the WeChat bind action outside the WeChat browser when only mp mode is configured', () => {
    const wrapper = mount(ProfileIdentityBindingsSection, {
      global: {
        plugins: [pinia],
      },
      props: {
        user: createUser(),
        linuxdoEnabled: false,
        oidcEnabled: false,
        wechatEnabled: true,
        wechatOpenEnabled: false,
        wechatMpEnabled: true,
      },
    })

    expect(wrapper.find('[data-testid="profile-binding-wechat-action"]').exists()).toBe(false)
  })

  it('keeps the WeChat bind action visible when only the legacy aggregate setting is present', () => {
    const wrapper = mount(ProfileIdentityBindingsSection, {
      global: {
        plugins: [pinia],
      },
      props: {
        user: createUser(),
        linuxdoEnabled: false,
        oidcEnabled: false,
        wechatEnabled: true,
      },
    })

    expect(wrapper.find('[data-testid="profile-binding-wechat-action"]').exists()).toBe(true)
  })

  it('starts the WeChat bind flow when only the legacy aggregate setting is present', async () => {
    const wrapper = mount(ProfileIdentityBindingsSection, {
      global: {
        plugins: [pinia],
      },
      props: {
        user: createUser(),
        linuxdoEnabled: false,
        oidcEnabled: false,
        wechatEnabled: true,
      },
    })

    await wrapper.get('[data-testid="profile-binding-wechat-action"]').trigger('click')

    expect(locationState.current.href).toContain('/api/v1/auth/oauth/wechat/bind/start?')
    expect(locationState.current.href).toContain('mode=open')
    expect(locationState.current.href).toContain('intent=bind_current_user')
    expect(locationState.current.href).toContain('redirect=%2Fprofile')
  })

  it('uses explicit cached WeChat capabilities and ignores legacy prop fallbacks', () => {
    const appStore = useAppStore()
    appStore.cachedPublicSettings = {
      registration_enabled: false,
      email_verify_enabled: false,
      force_email_on_third_party_signup: false,
      registration_email_suffix_whitelist: [],
      promo_code_enabled: true,
      password_reset_enabled: false,
      invitation_code_enabled: false,
      turnstile_enabled: false,
      turnstile_site_key: '',
      site_name: 'Sub2API',
      site_logo: '',
      site_subtitle: '',
      api_base_url: '',
      contact_info: '',
      doc_url: '',
      home_content: '',
      hide_ccs_import_button: false,
      payment_enabled: false,
      table_default_page_size: 20,
      table_page_size_options: [10, 20, 50, 100],
      custom_menu_items: [],
      custom_endpoints: [],
      linuxdo_oauth_enabled: false,
      wechat_oauth_enabled: true,
      wechat_oauth_open_enabled: true,
      wechat_oauth_mp_enabled: false,
      oidc_oauth_enabled: false,
      oidc_oauth_provider_name: 'OIDC',
      backend_mode_enabled: false,
      version: 'test',
      balance_low_notify_enabled: false,
      account_quota_notify_enabled: false,
      balance_low_notify_threshold: 0,
    }
    appStore.publicSettingsLoaded = true

    const wrapper = mount(ProfileIdentityBindingsSection, {
      global: {
        plugins: [pinia],
      },
      props: {
        user: createUser(),
        linuxdoEnabled: false,
        oidcEnabled: false,
        wechatEnabled: true,
      },
    })

    expect(wrapper.find('[data-testid="profile-binding-wechat-action"]').exists()).toBe(true)
  })

  it('sends email verification code and binds email from the profile card', async () => {
    userApiMocks.sendEmailBindingCode.mockResolvedValue(undefined)
    userApiMocks.bindEmailIdentity.mockResolvedValue(
      createUser({
        email: 'bound@example.com',
        email_bound: true,
        auth_bindings: {
          email: { bound: true },
        },
      })
    )

    const appStore = useAppStore()
    const authStore = useAuthStore()
    authStore.user = createUser({
      email: 'legacy-user@linuxdo-connect.invalid',
      email_bound: false,
      auth_bindings: {
        email: { bound: false },
      },
    })
    const showSuccessSpy = vi.spyOn(appStore, 'showSuccess')

    const wrapper = mount(ProfileIdentityBindingsSection, {
      global: {
        plugins: [pinia],
      },
      props: {
        user: authStore.user,
        linuxdoEnabled: false,
        oidcEnabled: false,
        wechatEnabled: false,
      },
    })

    await wrapper.get('[data-testid="profile-binding-email-input"]').setValue('bound@example.com')
    await wrapper.get('[data-testid="profile-binding-email-send-code"]').trigger('click')

    expect(userApiMocks.sendEmailBindingCode).toHaveBeenCalledWith('bound@example.com')
    expect(showSuccessSpy).toHaveBeenCalledWith('Code sent to bound@example.com')

    await wrapper.get('[data-testid="profile-binding-email-code-input"]').setValue('123456')
    await wrapper.get('[data-testid="profile-binding-email-password-input"]').setValue('new-password')
    await wrapper.get('[data-testid="profile-binding-email-submit"]').trigger('click')

    expect(userApiMocks.bindEmailIdentity).toHaveBeenCalledWith({
      email: 'bound@example.com',
      verify_code: '123456',
      password: 'new-password',
    })
    expect(wrapper.get('[data-testid="profile-binding-email-status"]').text()).toBe('Bound')
    expect(authStore.user?.email).toBe('bound@example.com')
  })

  it('keeps the email binding form visible when the user still lacks an email identity', () => {
    const wrapper = mount(ProfileIdentityBindingsSection, {
      global: {
        plugins: [pinia],
      },
      props: {
        user: createUser({
          email: 'legacy@example.com',
          email_bound: false,
          auth_bindings: {
            email: { bound: false },
          },
        }),
        linuxdoEnabled: false,
        oidcEnabled: false,
        wechatEnabled: false,
      },
    })

    expect(wrapper.get('[data-testid="profile-binding-email-status"]').text()).toBe('Not bound')
    expect(wrapper.get('[data-testid="profile-binding-email-input"]').exists()).toBe(true)
  })

  it('does not show a synthetic oauth-only email as the bound email summary', () => {
    const wrapper = mount(ProfileIdentityBindingsSection, {
      global: {
        plugins: [pinia],
      },
      props: {
        user: createUser({
          email: 'legacy-user@linuxdo-connect.invalid',
          email_bound: false,
          auth_bindings: {
            email: { bound: false },
          },
        }),
        linuxdoEnabled: false,
        oidcEnabled: false,
        wechatEnabled: false,
      },
    })

    expect(wrapper.text()).not.toContain('legacy-user@linuxdo-connect.invalid')
    expect(wrapper.get('[data-testid="profile-binding-email-status"]').text()).toBe('Not bound')
  })

  it('does not show a synthetic oauth-only email when only fallback auth bindings mark email as unbound', () => {
    const wrapper = mount(ProfileIdentityBindingsSection, {
      global: {
        plugins: [pinia],
      },
      props: {
        user: createUser({
          email: 'legacy-user@wechat-connect.invalid',
          auth_bindings: {
            email: { bound: false },
          },
        }),
        linuxdoEnabled: false,
        oidcEnabled: false,
        wechatEnabled: false,
      },
    })

    expect(wrapper.text()).not.toContain('legacy-user@wechat-connect.invalid')
    expect(wrapper.get('[data-testid="profile-binding-email-status"]').text()).toBe('Not bound')
  })

  it('shows the bound email only once and localizes the email management note', () => {
    const wrapper = mount(ProfileIdentityBindingsSection, {
      global: {
        plugins: [pinia],
      },
      props: {
        user: createUser({
          email: 'alice@example.com',
          email_bound: true,
          auth_bindings: {
            email: {
              bound: true,
              display_name: 'alice@example.com',
              subject_hint: 'a***e@example.com',
              note_key: 'profile.authBindings.notes.emailManagedFromProfile',
              note: 'Primary account email is managed from the profile form.',
            } as any,
          },
        }),
        linuxdoEnabled: false,
        oidcEnabled: false,
        wechatEnabled: false,
      },
    })

    expect(wrapper.text().match(/alice@example\.com/g)).toHaveLength(1)
    expect(wrapper.text()).not.toContain('a***e@example.com')
    expect(wrapper.text()).toContain('Primary email is managed in the profile form')
  })

  it('keeps the email form available for replacing a bound primary email', async () => {
    userApiMocks.sendEmailBindingCode.mockResolvedValue(undefined)
    userApiMocks.bindEmailIdentity.mockResolvedValue(
      createUser({
        email: 'new@example.com',
        email_bound: true,
        auth_bindings: {
          email: { bound: true },
        },
      })
    )

    const appStore = useAppStore()
    const authStore = useAuthStore()
    authStore.user = createUser({
      email: 'current@example.com',
      email_bound: true,
      auth_bindings: {
        email: { bound: true },
      },
    })
    const showSuccessSpy = vi.spyOn(appStore, 'showSuccess')

    const wrapper = mount(ProfileIdentityBindingsSection, {
      global: {
        plugins: [pinia],
      },
      props: {
        user: authStore.user,
        linuxdoEnabled: false,
        oidcEnabled: false,
        wechatEnabled: false,
      },
    })

    expect(wrapper.get('[data-testid="profile-binding-email-status"]').text()).toBe('Bound')
    expect(wrapper.get('[data-testid="profile-binding-email-input"]').exists()).toBe(true)
    expect(wrapper.get('[data-testid="profile-binding-email-submit"]').text()).toBe(
      'Replace primary email'
    )
    expect(
      (wrapper.get('[data-testid="profile-binding-email-password-input"]').element as HTMLInputElement)
        .placeholder
    ).toBe('Current password')

    await wrapper.get('[data-testid="profile-binding-email-input"]').setValue('new@example.com')
    await wrapper.get('[data-testid="profile-binding-email-send-code"]').trigger('click')
    expect(userApiMocks.sendEmailBindingCode).toHaveBeenCalledWith('new@example.com')

    await wrapper.get('[data-testid="profile-binding-email-code-input"]').setValue('123456')
    await wrapper.get('[data-testid="profile-binding-email-password-input"]').setValue(
      'current-password'
    )
    await wrapper.get('[data-testid="profile-binding-email-submit"]').trigger('click')

    expect(userApiMocks.bindEmailIdentity).toHaveBeenCalledWith({
      email: 'new@example.com',
      verify_code: '123456',
      password: 'current-password',
    })
    expect(authStore.user?.email).toBe('new@example.com')
    expect(showSuccessSpy).toHaveBeenCalledWith('Primary email updated')
  })

  it('collapses the email binding form in compact mode until the user expands it', async () => {
    const wrapper = mount(ProfileIdentityBindingsSection, {
      global: {
        plugins: [pinia],
      },
      props: {
        user: createUser({
          email: 'legacy@example.com',
          email_bound: false,
          auth_bindings: {
            email: { bound: false },
          },
        }),
        compact: true,
        linuxdoEnabled: false,
        oidcEnabled: false,
        wechatEnabled: false,
      },
    })

    expect(wrapper.find('[data-testid="profile-binding-email-input"]').exists()).toBe(false)
    expect(wrapper.get('[data-testid="profile-binding-email-toggle"]').text()).toBe('Manage email')

    await wrapper.get('[data-testid="profile-binding-email-toggle"]').trigger('click')

    expect(wrapper.get('[data-testid="profile-binding-email-input"]').exists()).toBe(true)
  })

  it('shows third-party binding details and unbinds a connected provider', async () => {
    userApiMocks.unbindAuthIdentity.mockResolvedValue(
      createUser({
        email_bound: true,
        linuxdo_bound: false,
        auth_bindings: {
          email: { bound: true },
          linuxdo: { bound: false, can_unbind: false },
        },
      })
    )

    const wrapper = mount(ProfileIdentityBindingsSection, {
      global: {
        plugins: [pinia],
      },
      props: {
        user: createUser({
          email_bound: true,
          linuxdo_bound: true,
          auth_bindings: {
            email: { bound: true },
            linuxdo: {
              bound: true,
              display_name: 'linuxdo-handle',
              subject_hint: 'lin***3456',
              note: 'Linked from LinuxDo',
              can_unbind: true,
            },
          },
        }),
        compact: true,
        linuxdoEnabled: true,
        oidcEnabled: false,
        wechatEnabled: false,
      },
    })

    expect(wrapper.text()).toContain('linuxdo-handle')
    expect(wrapper.text()).toContain('lin***3456')
    expect(wrapper.text()).toContain('Linked from LinuxDo')

    await wrapper.get('[data-testid="profile-binding-linuxdo-unbind"]').trigger('click')

    expect(userApiMocks.unbindAuthIdentity).toHaveBeenCalledWith('linuxdo')
    expect(wrapper.get('[data-testid="profile-binding-linuxdo-status"]').text()).toBe('Not bound')
  })

  it('localizes third-party unbind guidance from note_key', () => {
    const wrapper = mount(ProfileIdentityBindingsSection, {
      global: {
        plugins: [pinia],
      },
      props: {
        user: createUser({
          email_bound: true,
          linuxdo_bound: true,
          auth_bindings: {
            email: { bound: true },
            linuxdo: {
              bound: true,
              display_name: 'linuxdo-handle',
              note_key: 'profile.authBindings.notes.canUnbind',
              note: 'You can unbind this sign-in method.',
              can_unbind: true,
            } as any,
          },
        }),
        linuxdoEnabled: true,
        oidcEnabled: false,
        wechatEnabled: false,
      },
    })

    expect(wrapper.text()).toContain('You can unbind this sign-in method')
    expect(wrapper.text()).not.toContain('You can unbind this sign-in method.')
  })

  it('hides bind actions when provider details say bindable but the provider is disabled', () => {
    const wrapper = mount(ProfileIdentityBindingsSection, {
      global: {
        plugins: [pinia],
      },
      props: {
        user: createUser({
          auth_bindings: {
            linuxdo: { bound: false, can_bind: true },
            oidc: { bound: false, can_bind: true },
          },
        }),
        linuxdoEnabled: false,
        oidcEnabled: false,
        wechatEnabled: false,
      },
    })

    expect(wrapper.find('[data-testid="profile-binding-linuxdo-action"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="profile-binding-oidc-action"]').exists()).toBe(false)
  })
})
