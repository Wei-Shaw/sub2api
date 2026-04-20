import { mount REDACTED from '@vue/test-utils'
import { createPinia, setActivePinia REDACTED from 'pinia'
import { afterEach, beforeEach, describe, expect, it, vi REDACTED from 'vitest'
import ProfileIdentityBindingsSection from '@/components/user/profile/ProfileIdentityBindingsSection.vue'
import { useAppStore REDACTED from '@/stores'
import type { User REDACTED from '@/types'

const routeState = vi.hoisted(() => ({
  fullPath: '/profile',
REDACTED))

const locationState = vi.hoisted(() => ({
  current: { href: 'http://localhost/profile' REDACTED as { href: string REDACTED,
REDACTED))

let pinia: ReturnType<typeof createPinia>

vi.mock('vue-router', () => ({
  useRoute: () => routeState,
REDACTED))

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
REDACTED)
