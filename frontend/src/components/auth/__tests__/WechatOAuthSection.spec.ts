import { mount REDACTED from '@vue/test-utils'
import { createPinia, setActivePinia REDACTED from 'pinia'
import { afterEach, beforeEach, describe, expect, it, vi REDACTED from 'vitest'
import WechatOAuthSection from '@/components/auth/WechatOAuthSection.vue'
import { useAppStore REDACTED from '@/stores'
import type { PublicSettings REDACTED from '@/types'

const routeState = vi.hoisted(() => ({
  query: {REDACTED as Record<string, unknown>,
REDACTED))

const locationState = vi.hoisted(() => ({
  current: { href: 'http://localhost/login' REDACTED as { href: string REDACTED,
REDACTED))

let pinia: ReturnType<typeof createPinia>

vi.mock('vue-router', () => ({
  useRoute: () => routeState,
REDACTED))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      locale: { value: 'en' REDACTED,
      t: (key: string, params?: Record<string, string>) => {
        if (key === 'auth.wechatProviderName') {
          return 'Mock WeChat'
        REDACTED
        if (key === 'auth.oidc.signIn') {
          return `Continue with ${params?.providerName ?? ''REDACTED`.trim()
        REDACTED
        if (key === 'auth.oauthFlow.wechatSystemBrowserOnly') {
          return 'MOCK-SYSTEM-BROWSER-ONLY'
        REDACTED
        if (key === 'auth.oauthFlow.wechatBrowserOnly') {
          return 'MOCK-WECHAT-BROWSER-ONLY'
        REDACTED
        if (key === 'auth.oauthFlow.wechatNotConfigured') {
          return 'MOCK-NOT-CONFIGURED'
        REDACTED
        if (key === 'auth.oauthOrContinue') {
          return 'or continue'
        REDACTED
        return key
      REDACTED,
    REDACTED),
  REDACTED
REDACTED)

type WeChatPublicSettings = PublicSettings & {
  wechat_oauth_open_enabled?: boolean
  wechat_oauth_mp_enabled?: boolean
REDACTED

function buildPublicSettings(overrides: Partial<WeChatPublicSettings> = {REDACTED): WeChatPublicSettings {
  return {
    registration_enabled: true,
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
    api_base_url: '/api/v1',
    contact_info: '',
    doc_url: '',
    home_content: '',
    compact_home_enabled: false,
    hide_ccs_import_button: false,
    payment_enabled: false,
    table_default_page_size: 20,
    table_page_size_options: [10, 20, 50, 100],
    custom_menu_items: [],
    custom_endpoints: [],
    linuxdo_oauth_enabled: false,
    wechat_oauth_enabled: true,
    oidc_oauth_enabled: false,
    oidc_oauth_provider_name: 'OIDC',
    backend_mode_enabled: false,
    version: 'test',
    balance_low_notify_enabled: false,
    account_quota_notify_enabled: false,
    balance_low_notify_threshold: 0,
    ...overrides,
  REDACTED
REDACTED

function seedPublicSettings(overrides: Partial<WeChatPublicSettings> = {REDACTED): void {
  const appStore = useAppStore()
  const settings = buildPublicSettings(overrides)
  appStore.cachedPublicSettings = settings
  appStore.publicSettingsLoaded = true
REDACTED

describe('WechatOAuthSection', () => {
  beforeEach(() => {
    pinia = createPinia()
    setActivePinia(pinia)
    routeState.query = { redirect: '/billing?plan=pro' REDACTED
    locationState.current = { href: 'http://localhost/login' REDACTED
    Object.defineProperty(window, 'location', {
      configurable: true,
      value: locationState.current,
    REDACTED)
    Object.defineProperty(window.navigator, 'userAgent', {
      configurable: true,
      value: 'Mozilla/5.0',
    REDACTED)
  REDACTED)

  afterEach(() => {
    vi.unstubAllGlobals()
  REDACTED)

  it('starts the open WeChat OAuth flow with the current redirect target when open mode is configured', async () => {
    seedPublicSettings({
      wechat_oauth_open_enabled: true,
      wechat_oauth_mp_enabled: false,
    REDACTED)
    const wrapper = mount(WechatOAuthSection, {
      global: {
        plugins: [pinia],
      REDACTED,
    REDACTED)

    expect(wrapper.text()).toContain('Mock WeChat')

    await wrapper.get('button').trigger('click')

    expect(locationState.current.href).toContain(
      '/api/v1/auth/oauth/wechat/start?mode=open&redirect=%2Fbilling%3Fplan%3Dpro'
    )
  REDACTED)

  it('uses mp mode inside the WeChat browser when mp mode is configured', async () => {
    Object.defineProperty(window.navigator, 'userAgent', {
      configurable: true,
      value: 'Mozilla/5.0 MicroMessenger',
    REDACTED)
    seedPublicSettings({
      wechat_oauth_open_enabled: false,
      wechat_oauth_mp_enabled: true,
    REDACTED)
    const wrapper = mount(WechatOAuthSection, {
      global: {
        plugins: [pinia],
      REDACTED,
    REDACTED)

    await wrapper.get('button').trigger('click')

    expect(locationState.current.href).toContain(
      '/api/v1/auth/oauth/wechat/start?mode=mp&redirect=%2Fbilling%3Fplan%3Dpro'
    )
  REDACTED)

  it('disables the button outside the WeChat browser when only mp mode is configured', async () => {
    seedPublicSettings({
      wechat_oauth_open_enabled: false,
      wechat_oauth_mp_enabled: true,
    REDACTED)
    const wrapper = mount(WechatOAuthSection, {
      global: {
        plugins: [pinia],
      REDACTED,
    REDACTED)

    expect(wrapper.get('button').attributes('disabled')).toBeDefined()
    expect(wrapper.text()).toContain('MOCK-WECHAT-BROWSER-ONLY')

    await wrapper.get('button').trigger('click')

    expect(locationState.current.href).toBe('http://localhost/login')
  REDACTED)

  it('disables the button inside the WeChat browser when only open mode is configured', async () => {
    Object.defineProperty(window.navigator, 'userAgent', {
      configurable: true,
      value: 'Mozilla/5.0 MicroMessenger',
    REDACTED)
    seedPublicSettings({
      wechat_oauth_open_enabled: true,
      wechat_oauth_mp_enabled: false,
    REDACTED)
    const wrapper = mount(WechatOAuthSection, {
      global: {
        plugins: [pinia],
      REDACTED,
    REDACTED)

    expect(wrapper.get('button').attributes('disabled')).toBeDefined()
    expect(wrapper.text()).toContain('MOCK-SYSTEM-BROWSER-ONLY')

    await wrapper.get('button').trigger('click')

    expect(locationState.current.href).toBe('http://localhost/login')
  REDACTED)

  it('uses the legacy overall enabled flag when per-mode settings are not present', async () => {
    seedPublicSettings({
      wechat_oauth_enabled: true,
    REDACTED)
    const wrapper = mount(WechatOAuthSection, {
      global: {
        plugins: [pinia],
      REDACTED,
    REDACTED)

    await wrapper.get('button').trigger('click')

    expect(locationState.current.href).toContain(
      '/api/v1/auth/oauth/wechat/start?mode=open&redirect=%2Fbilling%3Fplan%3Dpro'
    )
  REDACTED)

  it('shows the localized not-configured hint when WeChat OAuth is unavailable', async () => {
    seedPublicSettings({
      wechat_oauth_enabled: false,
      wechat_oauth_open_enabled: false,
      wechat_oauth_mp_enabled: false,
    REDACTED)

    const wrapper = mount(WechatOAuthSection, {
      global: {
        plugins: [pinia],
      REDACTED,
    REDACTED)

    expect(wrapper.text()).toContain('MOCK-NOT-CONFIGURED')
  REDACTED)
REDACTED)
