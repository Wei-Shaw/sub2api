import { mount REDACTED from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi REDACTED from 'vitest'
import EmailOAuthButtons from '@/components/auth/EmailOAuthButtons.vue'

const routeState = vi.hoisted(() => ({
  query: {REDACTED as Record<string, unknown>,
REDACTED))

vi.mock('vue-router', () => ({
  useRoute: () => routeState,
REDACTED))

vi.mock('vue-i18n', () => ({
  useI18n: () => ({
    t: (key: string, params?: Record<string, string>) => {
      if (key === 'auth.emailOAuth.signIn') {
        return `使用 ${params?.providerName ?? ''REDACTED 登录`
      REDACTED
      return key
    REDACTED,
  REDACTED),
REDACTED))

describe('EmailOAuthButtons', () => {
  beforeEach(() => {
    routeState.query = { redirect: '/billing?plan=pro', aff: 'AFF123' REDACTED
    window.localStorage.clear()
    window.sessionStorage.clear()
  REDACTED)

  it('emits the GitHub OAuth request with redirect and affiliate parameters', async () => {
    const wrapper = mount(EmailOAuthButtons, {
      props: {
        githubEnabled: true,
        googleEnabled: false,
      REDACTED,
      global: {
        stubs: {
          GitHubMark: true,
          GoogleMark: true,
        REDACTED,
      REDACTED,
    REDACTED)

    await wrapper.get('button').trigger('click')

    expect(wrapper.emitted('start')).toEqual([[
      {
        provider: 'github',
        params: { redirect: '/billing?plan=pro', aff_code: 'AFF123' REDACTED
      REDACTED
    ]])
    expect(window.sessionStorage.getItem('oauth_aff_code')).toBe('AFF123')
    expect(window.sessionStorage.getItem('email_oauth_pending_provider')).toBe('github')
  REDACTED)

  it('emits the Google provider without navigating directly', async () => {
    const originalHref = window.location.href
    const wrapper = mount(EmailOAuthButtons, {
      props: {
        githubEnabled: false,
        googleEnabled: true,
      REDACTED,
      global: {
        stubs: {
          GitHubMark: true,
          GoogleMark: true,
        REDACTED,
      REDACTED,
    REDACTED)

    await wrapper.get('button').trigger('click')

    expect(wrapper.emitted('start')?.[0]?.[0]).toEqual({
      provider: 'google',
      params: { redirect: '/billing?plan=pro', aff_code: 'AFF123' REDACTED
    REDACTED)
    expect(window.location.href).toBe(originalHref)
  REDACTED)

  it('uses a full-width descriptive button when only GitHub is enabled', () => {
    const wrapper = mount(EmailOAuthButtons, {
      props: {
        githubEnabled: true,
        googleEnabled: false,
      REDACTED,
      global: {
        stubs: {
          GitHubMark: true,
          GoogleMark: true,
        REDACTED,
      REDACTED,
    REDACTED)

    expect(wrapper.find('.grid').classes()).not.toContain('sm:grid-cols-2')
    expect(wrapper.get('button').text()).toContain('使用 GitHub 登录')
  REDACTED)

  it('uses compact labels and two columns when GitHub and Google are both enabled', () => {
    const wrapper = mount(EmailOAuthButtons, {
      props: {
        githubEnabled: true,
        googleEnabled: true,
      REDACTED,
      global: {
        stubs: {
          GitHubMark: true,
          GoogleMark: true,
        REDACTED,
      REDACTED,
    REDACTED)

    expect(wrapper.find('.grid').classes()).toContain('sm:grid-cols-2')
    const buttons = wrapper.findAll('button')
    expect(buttons).toHaveLength(2)
    expect(buttons[0].text()).toContain('GitHub')
    expect(buttons[0].text()).not.toContain('使用 GitHub 登录')
    expect(buttons[1].text()).toContain('Google')
    expect(buttons[1].text()).not.toContain('使用 Google 登录')
  REDACTED)
REDACTED)
