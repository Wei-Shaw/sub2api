import { mount REDACTED from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi REDACTED from 'vitest'
import WechatOAuthSection from '@/components/auth/WechatOAuthSection.vue'

const routeState = vi.hoisted(() => ({
  query: {REDACTED as Record<string, unknown>,
REDACTED))

const locationState = vi.hoisted(() => ({
  current: { href: 'http://localhost/login' REDACTED as { href: string REDACTED,
REDACTED))

vi.mock('vue-router', () => ({
  useRoute: () => routeState,
REDACTED))

vi.mock('vue-i18n', () => ({
  useI18n: () => ({
    t: (key: string, params?: Record<string, string>) => {
      if (key === 'auth.oidc.signIn') {
        return `Continue with ${params?.providerName ?? ''REDACTED`.trim()
      REDACTED
      if (key === 'auth.oauthOrContinue') {
        return 'or continue'
      REDACTED
      return key
    REDACTED,
  REDACTED),
REDACTED))

describe('WechatOAuthSection', () => {
  beforeEach(() => {
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

  it('starts the open WeChat OAuth flow with the current redirect target', async () => {
    const wrapper = mount(WechatOAuthSection)

    expect(wrapper.text()).toContain('WeChat')

    await wrapper.get('button').trigger('click')

    expect(locationState.current.href).toContain(
      '/api/v1/auth/oauth/wechat/start?mode=open&redirect=%2Fbilling%3Fplan%3Dpro'
    )
  REDACTED)

  it('uses mp mode inside the WeChat browser', async () => {
    Object.defineProperty(window.navigator, 'userAgent', {
      configurable: true,
      value: 'Mozilla/5.0 MicroMessenger',
    REDACTED)
    const wrapper = mount(WechatOAuthSection)

    await wrapper.get('button').trigger('click')

    expect(locationState.current.href).toContain(
      '/api/v1/auth/oauth/wechat/start?mode=mp&redirect=%2Fbilling%3Fplan%3Dpro'
    )
  REDACTED)
REDACTED)
