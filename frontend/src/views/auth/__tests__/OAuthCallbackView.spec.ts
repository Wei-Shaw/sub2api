import { mount REDACTED from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi REDACTED from 'vitest'
import OAuthCallbackView from '@/views/auth/OAuthCallbackView.vue'

const { routeState, showErrorMock, copyToClipboardMock REDACTED = vi.hoisted(() => ({
  routeState: {
    query: {REDACTED as Record<string, unknown>,
  REDACTED,
  showErrorMock: vi.fn(),
  copyToClipboardMock: vi.fn(),
REDACTED))

vi.mock('vue-router', () => ({
  useRoute: () => routeState,
REDACTED))

vi.mock('vue-i18n', () => ({
  useI18n: () => ({
    t: (key: string) => key,
  REDACTED),
REDACTED))

vi.mock('@/stores', () => ({
  useAppStore: () => ({
    showError: (...args: any[]) => showErrorMock(...args),
  REDACTED),
REDACTED))

vi.mock('@/composables/useClipboard', () => ({
  useClipboard: () => ({
    copyToClipboard: (...args: any[]) => copyToClipboardMock(...args),
  REDACTED),
REDACTED))

describe('OAuthCallbackView', () => {
  beforeEach(() => {
    routeState.query = {REDACTED
    showErrorMock.mockReset()
    copyToClipboardMock.mockReset()
  REDACTED)

  it('renders localized callback copy actions', () => {
    routeState.query = {
      code: 'oauth-code',
      state: 'oauth-state',
    REDACTED

    const wrapper = mount(OAuthCallbackView)

    expect(wrapper.text()).toContain('auth.oauth.callbackTitle')
    expect(wrapper.text()).toContain('auth.oauth.callbackHint')
    expect(wrapper.text()).toContain('common.copy')
    expect(wrapper.find('input[value="oauth-code"]').exists()).toBe(true)
    expect(wrapper.find('input[value="oauth-state"]').exists()).toBe(true)
  REDACTED)

  it('sends callback errors to toast instead of rendering inline red text', () => {
    routeState.query = {
      error: 'oauth failed',
    REDACTED

    const wrapper = mount(OAuthCallbackView)

    expect(showErrorMock).toHaveBeenCalledWith('oauth failed')
    expect(wrapper.text()).not.toContain('oauth failed')
    expect(wrapper.find('.bg-red-50').exists()).toBe(false)
  REDACTED)
REDACTED)
