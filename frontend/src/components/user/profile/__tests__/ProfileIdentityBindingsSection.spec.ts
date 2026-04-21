import { mount REDACTED from '@vue/test-utils'
import { createPinia, setActivePinia REDACTED from 'pinia'
import { afterEach, beforeEach, describe, expect, it, vi REDACTED from 'vitest'
import ProfileIdentityBindingsSection from '@/components/user/profile/ProfileIdentityBindingsSection.vue'
import { useAppStore, useAuthStore REDACTED from '@/stores'
import type { User REDACTED from '@/types'

const routeState = vi.hoisted(() => ({
  fullPath: '/profile',
REDACTED))

const locationState = vi.hoisted(() => ({
  current: { href: 'http://localhost/profile' REDACTED as { href: string REDACTED,
REDACTED))

let pinia: ReturnType<typeof createPinia>

const userApiMocks = vi.hoisted(() => ({
  sendEmailBindingCode: vi.fn(),
  bindEmailIdentity: vi.fn(),
REDACTED))

vi.mock('vue-router', () => ({
  useRoute: () => routeState,
REDACTED))

vi.mock('@/api/user', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/api/user')>()
  return {
    ...actual,
    sendEmailBindingCode: (...args: any[]) => userApiMocks.sendEmailBindingCode(...args),
    bindEmailIdentity: (...args: any[]) => userApiMocks.bindEmailIdentity(...args),
  REDACTED
REDACTED)

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
        if (key === 'profile.authBindings.bindAction') return `Bind ${params?.providerName || ''REDACTED`.trim()
        if (key === 'profile.authBindings.emailPlaceholder') return 'Email address'
        if (key === 'profile.authBindings.codePlaceholder') return 'Verification code'
        if (key === 'profile.authBindings.passwordPlaceholder') return 'Set password'
        if (key === 'profile.authBindings.sendCodeAction') return 'Send code'
        if (key === 'profile.authBindings.confirmEmailBindAction') return 'Bind email'
        if (key === 'profile.authBindings.codeSentTo') return `Code sent to ${params?.email || ''REDACTED`.trim()
        if (key === 'profile.authBindings.bindSuccess') return 'Bind success'
        return key
      REDACTED,
    REDACTED),
  REDACTED
REDACTED)

function createUser(overrides: Partial<User> = {REDACTED): User {
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
  REDACTED
REDACTED

describe('ProfileIdentityBindingsSection', () => {
  beforeEach(() => {
    pinia = createPinia()
    setActivePinia(pinia)
    routeState.fullPath = '/profile'
    locationState.current = { href: 'http://localhost/profile' REDACTED
    Object.defineProperty(window, 'location', {
      configurable: true,
      value: locationState.current,
    REDACTED)
    Object.defineProperty(window.navigator, 'userAgent', {
      configurable: true,
      value: 'Mozilla/5.0',
    REDACTED)
    const appStore = useAppStore()
    appStore.cachedPublicSettings = null
    appStore.publicSettingsLoaded = false
    userApiMocks.sendEmailBindingCode.mockReset()
    userApiMocks.bindEmailIdentity.mockReset()
  REDACTED)

  afterEach(() => {
    vi.unstubAllGlobals()
  REDACTED)

  it('renders provider binding states and provider-specific bind actions', () => {
    const wrapper = mount(ProfileIdentityBindingsSection, {
      global: {
        plugins: [pinia],
      REDACTED,
      props: {
        user: createUser({
          auth_bindings: {
            email: { bound: true REDACTED,
            linuxdo: { bound: true REDACTED,
            oidc: { bound: false REDACTED,
            wechat: false,
          REDACTED,
        REDACTED),
        linuxdoEnabled: true,
        oidcEnabled: true,
        oidcProviderName: 'ExampleID',
        wechatEnabled: true,
        wechatOpenEnabled: true,
        wechatMpEnabled: false,
      REDACTED,
    REDACTED)

    expect(wrapper.get('[data-testid="profile-binding-email-status"]').text()).toBe('Bound')
    expect(wrapper.get('[data-testid="profile-binding-linuxdo-status"]').text()).toBe('Bound')
    expect(wrapper.get('[data-testid="profile-binding-oidc-status"]').text()).toBe('Not bound')
    expect(wrapper.get('[data-testid="profile-binding-oidc-action"]').text()).toBe(
      'Bind ExampleID'
    )
    expect(wrapper.get('[data-testid="profile-binding-wechat-action"]').text()).toBe('Bind WeChat')
  REDACTED)

  it('starts the WeChat bind flow for the current profile page', async () => {
    const wrapper = mount(ProfileIdentityBindingsSection, {
      global: {
        plugins: [pinia],
      REDACTED,
      props: {
        user: createUser(),
        linuxdoEnabled: false,
        oidcEnabled: false,
        wechatEnabled: true,
        wechatOpenEnabled: true,
        wechatMpEnabled: false,
      REDACTED,
    REDACTED)

    await wrapper.get('[data-testid="profile-binding-wechat-action"]').trigger('click')

    expect(locationState.current.href).toContain('/api/v1/auth/oauth/wechat/start?')
    expect(locationState.current.href).toContain('mode=open')
    expect(locationState.current.href).toContain('intent=bind_current_user')
    expect(locationState.current.href).toContain('redirect=%2Fprofile')
  REDACTED)

  it('hides the WeChat bind action outside the WeChat browser when only mp mode is configured', () => {
    const wrapper = mount(ProfileIdentityBindingsSection, {
      global: {
        plugins: [pinia],
      REDACTED,
      props: {
        user: createUser(),
        linuxdoEnabled: false,
        oidcEnabled: false,
        wechatEnabled: true,
        wechatOpenEnabled: false,
        wechatMpEnabled: true,
      REDACTED,
    REDACTED)

    expect(wrapper.find('[data-testid="profile-binding-wechat-action"]').exists()).toBe(false)
  REDACTED)

  it('hides the WeChat bind action when only the legacy aggregate setting is present', () => {
    const wrapper = mount(ProfileIdentityBindingsSection, {
      global: {
        plugins: [pinia],
      REDACTED,
      props: {
        user: createUser(),
        linuxdoEnabled: false,
        oidcEnabled: false,
        wechatEnabled: true,
      REDACTED,
    REDACTED)

    expect(wrapper.find('[data-testid="profile-binding-wechat-action"]').exists()).toBe(false)
  REDACTED)

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
    REDACTED
    appStore.publicSettingsLoaded = true

    const wrapper = mount(ProfileIdentityBindingsSection, {
      global: {
        plugins: [pinia],
      REDACTED,
      props: {
        user: createUser(),
        linuxdoEnabled: false,
        oidcEnabled: false,
        wechatEnabled: true,
      REDACTED,
    REDACTED)

    expect(wrapper.find('[data-testid="profile-binding-wechat-action"]').exists()).toBe(true)
  REDACTED)

  it('sends email verification code and binds email from the profile card', async () => {
    userApiMocks.sendEmailBindingCode.mockResolvedValue(undefined)
    userApiMocks.bindEmailIdentity.mockResolvedValue(
      createUser({
        email: 'bound@example.com',
        email_bound: true,
        auth_bindings: {
          email: { bound: true REDACTED,
        REDACTED,
      REDACTED)
    )

    const appStore = useAppStore()
    const authStore = useAuthStore()
    authStore.user = createUser({
      email: 'legacy-user@linuxdo-connect.invalid',
      email_bound: false,
      auth_bindings: {
        email: { bound: false REDACTED,
      REDACTED,
    REDACTED)
    const showSuccessSpy = vi.spyOn(appStore, 'showSuccess')

    const wrapper = mount(ProfileIdentityBindingsSection, {
      global: {
        plugins: [pinia],
      REDACTED,
      props: {
        user: authStore.user,
        linuxdoEnabled: false,
        oidcEnabled: false,
        wechatEnabled: false,
      REDACTED,
    REDACTED)

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
    REDACTED)
    expect(wrapper.get('[data-testid="profile-binding-email-status"]').text()).toBe('Bound')
    expect(authStore.user?.email).toBe('bound@example.com')
  REDACTED)
REDACTED)
