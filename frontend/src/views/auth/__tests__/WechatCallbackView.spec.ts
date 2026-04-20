import { flushPromises, mount REDACTED from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi REDACTED from 'vitest'
import WechatCallbackView from '@/views/auth/WechatCallbackView.vue'

const {
  exchangePendingOAuthCompletionMock,
  completeWeChatOAuthRegistrationMock,
  prepareOAuthBindAccessTokenCookieMock,
  getAuthTokenMock,
  replaceMock,
  setTokenMock,
  showSuccessMock,
  showErrorMock,
  routeState,
  locationState,
REDACTED = vi.hoisted(() => ({
  exchangePendingOAuthCompletionMock: vi.fn(),
  completeWeChatOAuthRegistrationMock: vi.fn(),
  prepareOAuthBindAccessTokenCookieMock: vi.fn(),
  getAuthTokenMock: vi.fn(),
  replaceMock: vi.fn(),
  setTokenMock: vi.fn(),
  showSuccessMock: vi.fn(),
  showErrorMock: vi.fn(),
  routeState: {
    query: {REDACTED as Record<string, unknown>,
  REDACTED,
  locationState: {
    current: {
      href: 'http://localhost/auth/wechat/callback',
      hash: '',
      search: '',
      pathname: '/auth/wechat/callback'
    REDACTED as { href: string; hash: string; search: string; pathname: string REDACTED,
  REDACTED,
REDACTED))

vi.mock('vue-router', () => ({
  useRoute: () => routeState,
  useRouter: () => ({
    replace: replaceMock,
  REDACTED),
REDACTED))

vi.mock('vue-i18n', () => ({
  createI18n: () => ({
    global: {
      t: (key: string) => key,
    REDACTED,
  REDACTED),
  useI18n: () => ({
    t: (key: string, params?: Record<string, string>) => {
      if (key === 'auth.oidc.callbackTitle') {
        return `Signing you in with ${params?.providerName ?? ''REDACTED`.trim()
      REDACTED
      if (key === 'auth.oidc.callbackProcessing') {
        return `Completing login with ${params?.providerName ?? ''REDACTED`.trim()
      REDACTED
      if (key === 'auth.oidc.invitationRequired') {
        return `${params?.providerName ?? ''REDACTED invitation required`.trim()
      REDACTED
      if (key === 'auth.oidc.completeRegistration') {
        return 'Complete registration'
      REDACTED
      if (key === 'auth.oidc.completing') {
        return 'Completing'
      REDACTED
      if (key === 'auth.oidc.backToLogin') {
        return 'Back to login'
      REDACTED
      if (key === 'auth.invitationCodePlaceholder') {
        return 'Invitation code'
      REDACTED
      if (key === 'auth.loginSuccess') {
        return 'Login success'
      REDACTED
      if (key === 'auth.loginFailed') {
        return 'Login failed'
      REDACTED
      if (key === 'auth.oidc.callbackHint') {
        return 'Callback hint'
      REDACTED
      if (key === 'auth.oidc.callbackMissingToken') {
        return 'Missing login token'
      REDACTED
      if (key === 'auth.oidc.completeRegistrationFailed') {
        return 'Complete registration failed'
      REDACTED
      return key
    REDACTED,
  REDACTED),
REDACTED))

vi.mock('@/stores', () => ({
  useAuthStore: () => ({
    setToken: setTokenMock,
  REDACTED),
  useAppStore: () => ({
    showSuccess: showSuccessMock,
    showError: showErrorMock,
  REDACTED),
REDACTED))

vi.mock('@/api/auth', async () => {
  const actual = await vi.importActual<typeof import('@/api/auth')>('@/api/auth')
  return {
    ...actual,
    exchangePendingOAuthCompletion: (...args: any[]) => exchangePendingOAuthCompletionMock(...args),
    completeWeChatOAuthRegistration: (...args: any[]) => completeWeChatOAuthRegistrationMock(...args),
    prepareOAuthBindAccessTokenCookie: (...args: any[]) => prepareOAuthBindAccessTokenCookieMock(...args),
    getAuthToken: (...args: any[]) => getAuthTokenMock(...args),
  REDACTED
REDACTED)

describe('WechatCallbackView', () => {
  beforeEach(() => {
    exchangePendingOAuthCompletionMock.mockReset()
    completeWeChatOAuthRegistrationMock.mockReset()
    replaceMock.mockReset()
    setTokenMock.mockReset()
    showSuccessMock.mockReset()
    showErrorMock.mockReset()
    prepareOAuthBindAccessTokenCookieMock.mockReset()
    getAuthTokenMock.mockReset()
    routeState.query = {REDACTED
    localStorage.clear()
    locationState.current = {
      href: 'http://localhost/auth/wechat/callback',
      hash: '',
      search: '',
      pathname: '/auth/wechat/callback'
    REDACTED
    Object.defineProperty(window, 'location', {
      configurable: true,
      value: locationState.current,
    REDACTED)
    Object.defineProperty(window.navigator, 'userAgent', {
      configurable: true,
      value: 'Mozilla/5.0',
    REDACTED)
  REDACTED)

  it('does not send adoption decisions during the initial exchange', async () => {
    exchangePendingOAuthCompletionMock.mockResolvedValue({
      access_token: 'access-token',
      refresh_token: 'refresh-token',
      expires_in: 3600,
      redirect: '/dashboard',
      adoption_required: true,
    REDACTED)
    setTokenMock.mockResolvedValue({REDACTED)

    mount(WechatCallbackView, {
      global: {
        stubs: {
          AuthLayout: { template: '<div><slot /></div>' REDACTED,
          Icon: true,
          RouterLink: { template: '<a><slot /></a>' REDACTED,
          transition: false,
        REDACTED,
      REDACTED,
    REDACTED)

    await flushPromises()

    expect(exchangePendingOAuthCompletionMock).toHaveBeenCalledWith()
    expect(exchangePendingOAuthCompletionMock).toHaveBeenCalledTimes(1)
  REDACTED)

  it('waits for explicit adoption confirmation before finishing a non-invitation login', async () => {
    exchangePendingOAuthCompletionMock
      .mockResolvedValueOnce({
        redirect: '/dashboard',
        adoption_required: true,
        suggested_display_name: 'WeChat Nick',
        suggested_avatar_url: 'https://cdn.example/wechat.png',
      REDACTED)
      .mockResolvedValueOnce({
        access_token: 'wechat-access-token',
        refresh_token: 'wechat-refresh-token',
        expires_in: 3600,
        token_type: 'Bearer',
        redirect: '/dashboard',
      REDACTED)
    setTokenMock.mockResolvedValue({REDACTED)

    const wrapper = mount(WechatCallbackView, {
      global: {
        stubs: {
          AuthLayout: { template: '<div><slot /></div>' REDACTED,
          Icon: true,
          RouterLink: { template: '<a><slot /></a>' REDACTED,
          transition: false,
        REDACTED,
      REDACTED,
    REDACTED)

    await flushPromises()

    expect(wrapper.text()).toContain('WeChat Nick')
    expect(setTokenMock).not.toHaveBeenCalled()
    expect(replaceMock).not.toHaveBeenCalled()

    const checkboxes = wrapper.findAll('input[type="checkbox"]')
    expect(checkboxes).toHaveLength(2)
    await checkboxes[1].setValue(false)

    const buttons = wrapper.findAll('button')
    expect(buttons).toHaveLength(1)
    await buttons[0].trigger('click')
    await flushPromises()

    expect(exchangePendingOAuthCompletionMock).toHaveBeenNthCalledWith(1)
    expect(exchangePendingOAuthCompletionMock).toHaveBeenNthCalledWith(2, {
      adoptDisplayName: true,
      adoptAvatar: false,
    REDACTED)
    expect(setTokenMock).toHaveBeenCalledWith('wechat-access-token')
    expect(replaceMock).toHaveBeenCalledWith('/dashboard')
    expect(localStorage.getItem('refresh_token')).toBe('wechat-refresh-token')
  REDACTED)

  it('supports bind completion after adoption confirmation', async () => {
    exchangePendingOAuthCompletionMock
      .mockResolvedValueOnce({
        redirect: '/dashboard',
        adoption_required: true,
        suggested_display_name: 'WeChat Nick',
        suggested_avatar_url: 'https://cdn.example/wechat.png',
      REDACTED)
      .mockResolvedValueOnce({
        redirect: '/profile/connections',
      REDACTED)

    const wrapper = mount(WechatCallbackView, {
      global: {
        stubs: {
          AuthLayout: { template: '<div><slot /></div>' REDACTED,
          Icon: true,
          RouterLink: { template: '<a><slot /></a>' REDACTED,
          transition: false,
        REDACTED,
      REDACTED,
    REDACTED)

    await flushPromises()

    await wrapper.findAll('button')[0].trigger('click')
    await flushPromises()

    expect(exchangePendingOAuthCompletionMock).toHaveBeenNthCalledWith(2, {
      adoptDisplayName: true,
      adoptAvatar: true,
    REDACTED)
    expect(setTokenMock).not.toHaveBeenCalled()
    expect(showSuccessMock).toHaveBeenCalledWith('profile.authBindings.bindSuccess')
    expect(replaceMock).toHaveBeenCalledWith('/profile/connections')
  REDACTED)

  it('renders adoption choices for invitation flow and submits the selected values', async () => {
    exchangePendingOAuthCompletionMock.mockResolvedValue({
      error: 'invitation_required',
      redirect: '/subscriptions',
      adoption_required: true,
      suggested_display_name: 'WeChat Nick',
      suggested_avatar_url: 'https://cdn.example/wechat.png',
    REDACTED)
    completeWeChatOAuthRegistrationMock.mockResolvedValue({
      access_token: 'wechat-invite-token',
      refresh_token: 'wechat-invite-refresh',
      expires_in: 600,
      token_type: 'Bearer',
    REDACTED)

    const wrapper = mount(WechatCallbackView, {
      global: {
        stubs: {
          AuthLayout: { template: '<div><slot /></div>' REDACTED,
          Icon: true,
          RouterLink: { template: '<a><slot /></a>' REDACTED,
          transition: false,
        REDACTED,
      REDACTED,
    REDACTED)

    await flushPromises()

    expect(wrapper.text()).toContain('WeChat Nick')
    const checkboxes = wrapper.findAll('input[type="checkbox"]')
    expect(checkboxes).toHaveLength(2)
    await checkboxes[0].setValue(false)
    await wrapper.get('input[type="text"]').setValue(' INVITE-CODE ')
    await wrapper.get('button').trigger('click')
    await flushPromises()

    expect(completeWeChatOAuthRegistrationMock).toHaveBeenCalledWith('INVITE-CODE', {
      adoptDisplayName: false,
      adoptAvatar: true,
    REDACTED)
    expect(setTokenMock).toHaveBeenCalledWith('wechat-invite-token')
    expect(replaceMock).toHaveBeenCalledWith('/subscriptions')
  REDACTED)

  it('offers existing-account email collection during invitation flow', async () => {
    exchangePendingOAuthCompletionMock.mockResolvedValue({
      error: 'invitation_required',
      redirect: '/usage',
    REDACTED)
    getAuthTokenMock.mockReturnValue(null)

    const wrapper = mount(WechatCallbackView, {
      global: {
        stubs: {
          AuthLayout: { template: '<div><slot /></div>' REDACTED,
          Icon: true,
          RouterLink: { template: '<a><slot /></a>' REDACTED,
          transition: false,
        REDACTED,
      REDACTED,
    REDACTED)

    await flushPromises()

    const emailInput = wrapper.get('[data-testid="existing-account-email"]')
    await emailInput.setValue('user@example.com')
    await wrapper.get('[data-testid="existing-account-submit"]').trigger('click')

    expect(replaceMock).toHaveBeenCalledTimes(1)
    expect(replaceMock.mock.calls[0]?.[0]).toContain('/login?')
    expect(replaceMock.mock.calls[0]?.[0]).toContain('wechat_bind_existing%3D1')
    expect(replaceMock.mock.calls[0]?.[0]).toContain('email=user%40example.com')
  REDACTED)

  it('restarts the current-user bind flow after returning from login', async () => {
    routeState.query = {
      wechat_bind_existing: '1',
      redirect: '/profile'
    REDACTED
    getAuthTokenMock.mockReturnValue('existing-auth-token')

    mount(WechatCallbackView, {
      global: {
        stubs: {
          AuthLayout: { template: '<div><slot /></div>' REDACTED,
          Icon: true,
          RouterLink: { template: '<a><slot /></a>' REDACTED,
          transition: false,
        REDACTED,
      REDACTED,
    REDACTED)

    await flushPromises()

    expect(exchangePendingOAuthCompletionMock).not.toHaveBeenCalled()
    expect(prepareOAuthBindAccessTokenCookieMock).toHaveBeenCalledTimes(1)
    expect(locationState.current.href).toContain('/api/v1/auth/oauth/wechat/start?')
    expect(locationState.current.href).toContain('intent=bind_current_user')
    expect(locationState.current.href).toContain('redirect=%2Fprofile')
  REDACTED)
REDACTED)
