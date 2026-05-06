import { mount REDACTED from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi REDACTED from 'vitest'
import OAuthCallbackView from '@/views/auth/OAuthCallbackView.vue'

const {
  routeState,
  routerReplaceMock,
  showErrorMock,
  showSuccessMock,
  setTokenMock,
  copyToClipboardMock,
  exchangePendingOAuthCompletionMock,
  apiPostMock,
REDACTED = vi.hoisted(() => ({
  routeState: {
    path: '/auth/callback',
    query: {REDACTED as Record<string, unknown>,
  REDACTED,
  routerReplaceMock: vi.fn(),
  showErrorMock: vi.fn(),
  showSuccessMock: vi.fn(),
  setTokenMock: vi.fn(),
  copyToClipboardMock: vi.fn(),
  exchangePendingOAuthCompletionMock: vi.fn(),
  apiPostMock: vi.fn(),
REDACTED))

vi.mock('vue-router', () => ({
  useRoute: () => routeState,
  useRouter: () => ({
    replace: (...args: any[]) => routerReplaceMock(...args),
  REDACTED),
REDACTED))

vi.mock('vue-i18n', () => ({
  useI18n: () => ({
    t: (key: string) => key,
  REDACTED),
REDACTED))

vi.mock('@/stores', () => ({
  useAuthStore: () => ({
    setToken: (...args: any[]) => setTokenMock(...args),
  REDACTED),
  useAppStore: () => ({
    showError: (...args: any[]) => showErrorMock(...args),
    showSuccess: (...args: any[]) => showSuccessMock(...args),
  REDACTED),
REDACTED))

vi.mock('@/api/client', () => ({
  apiClient: {
    post: (...args: any[]) => apiPostMock(...args),
  REDACTED,
REDACTED))

vi.mock('@/api/auth', async () => {
  const actual = await vi.importActual<typeof import('@/api/auth')>('@/api/auth')
  return {
    ...actual,
    exchangePendingOAuthCompletion: (...args: any[]) => exchangePendingOAuthCompletionMock(...args),
    persistOAuthTokenContext: vi.fn(),
  REDACTED
REDACTED)

vi.mock('@/composables/useClipboard', () => ({
  useClipboard: () => ({
    copyToClipboard: (...args: any[]) => copyToClipboardMock(...args),
  REDACTED),
REDACTED))

describe('OAuthCallbackView', () => {
  beforeEach(() => {
    routeState.path = '/auth/callback'
    routeState.query = {REDACTED
    window.location.hash = ''
    routerReplaceMock.mockReset()
    showErrorMock.mockReset()
    showSuccessMock.mockReset()
    setTokenMock.mockReset()
    copyToClipboardMock.mockReset()
    exchangePendingOAuthCompletionMock.mockReset()
    apiPostMock.mockReset()
    window.sessionStorage.clear()
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

  it('does not render manual copy fields for direct email oauth callback visits', async () => {
    routeState.path = '/auth/oauth/callback'
    exchangePendingOAuthCompletionMock.mockRejectedValue(new Error('pending session not found'))

    const wrapper = mount(OAuthCallbackView)
    await vi.dynamicImportSettled()

    expect(exchangePendingOAuthCompletionMock).toHaveBeenCalledTimes(1)
    expect(wrapper.text()).toContain('auth.oauth.invalidCallbackTitle')
    expect(wrapper.text()).toContain('auth.oauth.invalidCallbackHint')
    expect(wrapper.find('input[readonly]').exists()).toBe(false)
  REDACTED)

  it('submits stored affiliate code when completing invited email oauth registration', async () => {
    routeState.path = '/auth/oauth/callback'
    exchangePendingOAuthCompletionMock.mockResolvedValue({
      error: 'invitation_required',
      provider: 'google',
      redirect: '/dashboard',
    REDACTED)
    apiPostMock.mockResolvedValue({
      data: {
        access_token: 'token-1',
      REDACTED,
    REDACTED)
    window.sessionStorage.setItem('oauth_aff_code', 'AFF456')

    const wrapper = mount(OAuthCallbackView)
    await vi.dynamicImportSettled()
    const input = wrapper.find('input[type="text"]')
    await input.setValue('INVITE456')
    await wrapper.findAll('button').at(0)?.trigger('click')

    expect(apiPostMock).toHaveBeenCalledWith('/auth/oauth/google/complete-registration', {
      invitation_code: 'INVITE456',
      aff_code: 'AFF456',
    REDACTED)
    expect(setTokenMock).toHaveBeenCalledWith('token-1')
  REDACTED)
REDACTED)
