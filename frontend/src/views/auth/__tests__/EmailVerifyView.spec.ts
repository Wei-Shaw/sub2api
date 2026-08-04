import { defineComponent, h REDACTED from 'vue'
import { flushPromises, mount REDACTED from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi REDACTED from 'vitest'
import EmailVerifyView from '@/views/auth/EmailVerifyView.vue'

const {
  pushMock,
  showSuccessMock,
  showErrorMock,
  registerMock,
  setTokenMock,
  setPendingAuthSessionMock,
  clearPendingAuthSessionMock,
  getPublicSettingsMock,
  sendVerifyCodeMock,
  sendPendingOAuthVerifyCodeMock,
  persistOAuthTokenContextMock,
  apiClientPostMock,
  authStoreState,
  createTurnstileResetMock,
  verifyTencentMock,
REDACTED = vi.hoisted(() => ({
  pushMock: vi.fn(),
  showSuccessMock: vi.fn(),
  showErrorMock: vi.fn(),
  registerMock: vi.fn(),
  setTokenMock: vi.fn(),
  setPendingAuthSessionMock: vi.fn(),
  clearPendingAuthSessionMock: vi.fn(),
  getPublicSettingsMock: vi.fn(),
  sendVerifyCodeMock: vi.fn(),
  sendPendingOAuthVerifyCodeMock: vi.fn(),
  persistOAuthTokenContextMock: vi.fn(),
  apiClientPostMock: vi.fn(),
  createTurnstileResetMock: vi.fn(),
  verifyTencentMock: vi.fn(),
  authStoreState: {
    pendingAuthSession: null as null | {
      token: string
      token_field: 'pending_auth_token' | 'pending_oauth_token'
      provider: string
      redirect?: string
      adoption_required?: boolean
      suggested_display_name?: string
      suggested_avatar_url?: string
    REDACTED
  REDACTED,
REDACTED))

vi.mock('vue-router', () => ({
  useRouter: () => ({
    push: pushMock,
  REDACTED),
REDACTED))

vi.mock('vue-i18n', () => ({
  createI18n: () => ({
    global: {
      t: (key: string) => key,
    REDACTED,
  REDACTED),
  useI18n: () => ({
    t: (key: string, params?: Record<string, string | number>) => {
      if (key === 'auth.accountCreatedSuccess') {
        return `Account created for ${params?.siteName ?? 'Sub2API'REDACTED`
      REDACTED
      return key
    REDACTED,
    locale: { value: 'en' REDACTED,
  REDACTED),
REDACTED))

vi.mock('@/stores', () => ({
  useAuthStore: () => ({
    pendingAuthSession: authStoreState.pendingAuthSession,
    register: (...args: any[]) => registerMock(...args),
    setToken: (...args: any[]) => setTokenMock(...args),
    setPendingAuthSession: (...args: any[]) => setPendingAuthSessionMock(...args),
    clearPendingAuthSession: (...args: any[]) => clearPendingAuthSessionMock(...args),
  REDACTED),
  useAppStore: () => ({
    showSuccess: (...args: any[]) => showSuccessMock(...args),
    showError: (...args: any[]) => showErrorMock(...args),
  REDACTED),
REDACTED))

vi.mock('@/api/auth', async () => {
  const actual = await vi.importActual<typeof import('@/api/auth')>('@/api/auth')
  return {
    ...actual,
    getPublicSettings: (...args: any[]) => getPublicSettingsMock(...args),
    sendVerifyCode: (...args: any[]) => sendVerifyCodeMock(...args),
    sendPendingOAuthVerifyCode: (...args: any[]) => sendPendingOAuthVerifyCodeMock(...args),
    persistOAuthTokenContext: (...args: any[]) => persistOAuthTokenContextMock(...args),
  REDACTED
REDACTED)

vi.mock('@/api/client', () => ({
  apiClient: {
    post: (...args: any[]) => apiClientPostMock(...args),
  REDACTED,
REDACTED))

describe('EmailVerifyView', () => {
  beforeEach(() => {
    pushMock.mockReset()
    showSuccessMock.mockReset()
    showErrorMock.mockReset()
    registerMock.mockReset()
    setTokenMock.mockReset()
    setPendingAuthSessionMock.mockReset()
    clearPendingAuthSessionMock.mockReset()
    getPublicSettingsMock.mockReset()
    sendVerifyCodeMock.mockReset()
    sendPendingOAuthVerifyCodeMock.mockReset()
    persistOAuthTokenContextMock.mockReset()
    apiClientPostMock.mockReset()
    createTurnstileResetMock.mockReset()
    verifyTencentMock.mockReset()
    authStoreState.pendingAuthSession = null
    sessionStorage.clear()
    localStorage.clear()

    getPublicSettingsMock.mockResolvedValue({
      turnstile_enabled: false,
      turnstile_site_key: '',
      site_name: 'Sub2API',
      registration_email_suffix_whitelist: [],
    REDACTED)
    sendVerifyCodeMock.mockResolvedValue({ countdown: 60 REDACTED)
    sendPendingOAuthVerifyCodeMock.mockResolvedValue({ countdown: 60 REDACTED)
    setTokenMock.mockResolvedValue({REDACTED)
  REDACTED)

  it('acquires a fresh Tencent proof for each resend action', async () => {
    getPublicSettingsMock.mockResolvedValue({
      turnstile_enabled: false,
      turnstile_site_key: '',
      tencent_captcha_enabled: true,
      tencent_captcha_app_id: 'tencent-app-id',
      site_name: 'Sub2API',
      registration_email_suffix_whitelist: [],
    REDACTED)
    sendVerifyCodeMock.mockResolvedValue({ countdown: 0 REDACTED)
    verifyTencentMock
      .mockResolvedValueOnce({ ticket: 'ticket-1', randstr: '@rand-1' REDACTED)
      .mockResolvedValueOnce({ ticket: 'ticket-2', randstr: '@rand-2' REDACTED)
    sessionStorage.setItem(
      'register_data',
      JSON.stringify({
        email: 'fresh@example.com',
        password: 'secret-123',
        tencent_captcha_ticket: 'initial-ticket',
        tencent_captcha_randstr: '@initial-rand',
      REDACTED)
    )

    const CaptchaChallengeStub = defineComponent({
      setup(_, { expose REDACTED) {
        expose({ verifyTencent: verifyTencentMock, reset: createTurnstileResetMock REDACTED)
        return () => h('div')
      REDACTED,
    REDACTED)
    const wrapper = mount(EmailVerifyView, {
      global: {
        stubs: {
          AuthLayout: { template: '<div><slot /><slot name="footer" /></div>' REDACTED,
          Icon: true,
          TurnstileWidget: CaptchaChallengeStub,
          transition: false,
        REDACTED,
      REDACTED,
    REDACTED)

    await flushPromises()
    const resendButton = () => wrapper.findAll('button').find((button) =>
      button.text().includes('auth.clickToResend')
    )!

    await resendButton().trigger('click')
    await flushPromises()
    await resendButton().trigger('click')
    await flushPromises()

    expect(verifyTencentMock).toHaveBeenCalledTimes(2)
    expect(sendVerifyCodeMock).toHaveBeenNthCalledWith(2, expect.objectContaining({
      tencent_captcha_ticket: 'ticket-1',
      tencent_captcha_randstr: '@rand-1',
    REDACTED))
    expect(sendVerifyCodeMock).toHaveBeenNthCalledWith(3, expect.objectContaining({
      tencent_captcha_ticket: 'ticket-2',
      tencent_captcha_randstr: '@rand-2',
    REDACTED))
  REDACTED)

  it('uses the pending oauth verify-code endpoint when register data carries a pending auth session', async () => {
    authStoreState.pendingAuthSession = {
      token: 'pending-token-1',
      token_field: 'pending_auth_token',
      provider: 'wechat',
      redirect: '/profile',
    REDACTED
    sessionStorage.setItem(
      'register_data',
      JSON.stringify({
        email: 'fresh@example.com',
        password: 'secret-123',
      REDACTED)
    )

    mount(EmailVerifyView, {
      global: {
        stubs: {
          AuthLayout: { template: '<div><slot /><slot name="footer" /></div>' REDACTED,
          Icon: true,
          TurnstileWidget: true,
          transition: false,
        REDACTED,
      REDACTED,
    REDACTED)

    await flushPromises()

    expect(sendPendingOAuthVerifyCodeMock).toHaveBeenCalledWith({
      email: 'fresh@example.com',
      pending_auth_token: 'pending-token-1',
    REDACTED)
    expect(sendVerifyCodeMock).not.toHaveBeenCalled()
  REDACTED)

  it('requires a fresh captcha proof after the initial send-code request fails', async () => {
    getPublicSettingsMock.mockResolvedValue({
      turnstile_enabled: true,
      turnstile_site_key: 'site-key',
      site_name: 'Sub2API',
      registration_email_suffix_whitelist: [],
    REDACTED)
    sendVerifyCodeMock.mockRejectedValue(new Error('send failed'))
    sessionStorage.setItem(
      'register_data',
      JSON.stringify({
        email: 'fresh@example.com',
        password: 'secret-123',
        turnstile_token: 'initial-proof',
      REDACTED)
    )

    const wrapper = mount(EmailVerifyView, {
      global: {
        stubs: {
          AuthLayout: { template: '<div><slot /><slot name="footer" /></div>' REDACTED,
          Icon: true,
          TurnstileWidget: {
            template: '<button data-testid="resend-captcha" @click="$emit(\'verify\', \'fresh-proof\')">verify</button>',
          REDACTED,
          transition: false,
        REDACTED,
      REDACTED,
    REDACTED)

    await flushPromises()

    expect(sendVerifyCodeMock).toHaveBeenCalledWith(expect.objectContaining({
      turnstile_token: 'initial-proof',
    REDACTED))
    expect(wrapper.find('[data-testid="resend-captcha"]').exists()).toBe(true)
  REDACTED)

  it('skips the registration email suffix whitelist for pending oauth verification', async () => {
    authStoreState.pendingAuthSession = {
      token: 'pending-token-2',
      token_field: 'pending_auth_token',
      provider: 'oidc',
      redirect: '/profile',
    REDACTED
    getPublicSettingsMock.mockResolvedValue({
      turnstile_enabled: false,
      turnstile_site_key: '',
      site_name: 'Sub2API',
      registration_email_suffix_whitelist: ['allowed.com'],
    REDACTED)
    sessionStorage.setItem(
      'register_data',
      JSON.stringify({
        email: 'fresh@example.com',
        password: 'secret-123',
      REDACTED)
    )

    mount(EmailVerifyView, {
      global: {
        stubs: {
          AuthLayout: { template: '<div><slot /><slot name="footer" /></div>' REDACTED,
          Icon: true,
          TurnstileWidget: true,
          transition: false,
        REDACTED,
      REDACTED,
    REDACTED)

    await flushPromises()

    expect(sendPendingOAuthVerifyCodeMock).toHaveBeenCalledWith({
      email: 'fresh@example.com',
      pending_auth_token: 'pending-token-2',
    REDACTED)
    expect(showErrorMock).not.toHaveBeenCalled()
  REDACTED)

  it('uses the pending oauth verify-code endpoint when auth store only carries the pending provider', async () => {
    authStoreState.pendingAuthSession = {
      token: '',
      token_field: 'pending_oauth_token',
      provider: 'oidc',
      redirect: '/profile',
    REDACTED
    getPublicSettingsMock.mockResolvedValue({
      turnstile_enabled: false,
      turnstile_site_key: '',
      site_name: 'Sub2API',
      registration_email_suffix_whitelist: ['allowed.com'],
    REDACTED)
    sessionStorage.setItem(
      'register_data',
      JSON.stringify({
        email: 'fresh@example.com',
        password: 'secret-123',
      REDACTED)
    )

    mount(EmailVerifyView, {
      global: {
        stubs: {
          AuthLayout: { template: '<div><slot /><slot name="footer" /></div>' REDACTED,
          Icon: true,
          TurnstileWidget: true,
          transition: false,
        REDACTED,
      REDACTED,
    REDACTED)

    await flushPromises()

    expect(sendPendingOAuthVerifyCodeMock).toHaveBeenCalledWith({
      email: 'fresh@example.com',
      pending_oauth_token: undefined,
    REDACTED)
    expect(sendVerifyCodeMock).not.toHaveBeenCalled()
    expect(showErrorMock).not.toHaveBeenCalled()
  REDACTED)

  it('returns to the oauth callback flow when pending send-code detects an existing account email', async () => {
    authStoreState.pendingAuthSession = {
      token: '',
      token_field: 'pending_oauth_token',
      provider: 'oidc',
      redirect: '/profile/security',
    REDACTED
    getPublicSettingsMock.mockResolvedValue({
      turnstile_enabled: false,
      turnstile_site_key: '',
      site_name: 'Sub2API',
      registration_email_suffix_whitelist: ['allowed.com'],
    REDACTED)
    sendPendingOAuthVerifyCodeMock.mockResolvedValue({
      auth_result: 'pending_session',
      provider: 'oidc',
      redirect: '/profile/security',
    REDACTED)
    sessionStorage.setItem(
      'register_data',
      JSON.stringify({
        email: 'fresh@example.com',
        password: 'secret-123',
      REDACTED)
    )

    mount(EmailVerifyView, {
      global: {
        stubs: {
          AuthLayout: { template: '<div><slot /><slot name="footer" /></div>' REDACTED,
          Icon: true,
          TurnstileWidget: true,
          transition: false,
        REDACTED,
      REDACTED,
    REDACTED)

    await flushPromises()

    expect(setPendingAuthSessionMock).toHaveBeenCalledWith({
      token: '',
      token_field: 'pending_oauth_token',
      provider: 'oidc',
      redirect: '/profile/security',
    REDACTED)
    expect(pushMock).toHaveBeenCalledWith('/auth/oidc/callback')
    expect(showErrorMock).not.toHaveBeenCalled()
  REDACTED)

  it('submits pending auth account creation when session storage has no pending metadata but auth store does', async () => {
    authStoreState.pendingAuthSession = {
      token: 'pending-token-1',
      token_field: 'pending_auth_token',
      provider: 'wechat',
      redirect: '/profile',
    REDACTED
    sessionStorage.setItem(
      'register_data',
      JSON.stringify({
        email: 'fresh@example.com',
        password: 'secret-123',
        aff_code: 'AFF123',
      REDACTED)
    )
    apiClientPostMock.mockResolvedValue({
      data: {
        access_token: 'oauth-access-token',
        refresh_token: 'oauth-refresh-token',
        expires_in: 3600,
        token_type: 'Bearer',
      REDACTED,
    REDACTED)

    const wrapper = mount(EmailVerifyView, {
      global: {
        stubs: {
          AuthLayout: { template: '<div><slot /><slot name="footer" /></div>' REDACTED,
          Icon: true,
          TurnstileWidget: true,
          transition: false,
        REDACTED,
      REDACTED,
    REDACTED)

    await flushPromises()
    await wrapper.get('#code').setValue('123456')
    await wrapper.get('form').trigger('submit.prevent')
    await flushPromises()

    expect(apiClientPostMock).toHaveBeenCalledWith('/auth/oauth/pending/create-account', {
      email: 'fresh@example.com',
      password: 'secret-123',
      verify_code: '123456',
      aff_code: 'AFF123',
    REDACTED)
    expect(persistOAuthTokenContextMock).toHaveBeenCalledWith({
      access_token: 'oauth-access-token',
      refresh_token: 'oauth-refresh-token',
      expires_in: 3600,
      token_type: 'Bearer',
    REDACTED)
    expect(setTokenMock).toHaveBeenCalledWith('oauth-access-token')
    expect(clearPendingAuthSessionMock).toHaveBeenCalled()
    expect(pushMock).toHaveBeenCalledWith('/profile')
    expect(registerMock).not.toHaveBeenCalled()
  REDACTED)

  it('requires and submits a fresh turnstile token for pending oauth account creation', async () => {
    authStoreState.pendingAuthSession = {
      token: 'pending-token-3',
      token_field: 'pending_auth_token',
      provider: 'oidc',
      redirect: '/profile',
    REDACTED
    getPublicSettingsMock.mockResolvedValue({
      turnstile_enabled: true,
      turnstile_site_key: 'site-key',
      site_name: 'Sub2API',
      registration_email_suffix_whitelist: ['allowed.com'],
    REDACTED)
    sessionStorage.setItem(
      'register_data',
      JSON.stringify({
        email: 'fresh@example.com',
        password: 'secret-123',
        turnstile_token: 'send-code-token',
      REDACTED)
    )
    apiClientPostMock.mockResolvedValue({
      data: {
        access_token: 'oauth-access-token',
        refresh_token: 'oauth-refresh-token',
        expires_in: 3600,
        token_type: 'Bearer',
      REDACTED,
    REDACTED)

    const wrapper = mount(EmailVerifyView, {
      global: {
        stubs: {
          AuthLayout: { template: '<div><slot /><slot name="footer" /></div>' REDACTED,
          Icon: true,
          TurnstileWidget: {
            template: '<button data-testid="create-turnstile" @click="$emit(\'verify\', \'create-token\')">verify</button>',
            methods: {
              reset: createTurnstileResetMock,
            REDACTED,
          REDACTED,
          transition: false,
        REDACTED,
      REDACTED,
    REDACTED)

    await flushPromises()

    expect(sendPendingOAuthVerifyCodeMock).toHaveBeenCalledWith({
      email: 'fresh@example.com',
      pending_auth_token: 'pending-token-3',
      turnstile_token: 'send-code-token',
    REDACTED)

    await wrapper.get('#code').setValue('123456')
    expect(wrapper.get('button[type="submit"]').attributes('disabled')).toBeDefined()

    await wrapper.get('[data-testid="create-turnstile"]').trigger('click')
    expect(wrapper.get('button[type="submit"]').attributes('disabled')).toBeUndefined()

    await wrapper.get('form').trigger('submit.prevent')
    await flushPromises()

    expect(apiClientPostMock).toHaveBeenCalledWith('/auth/oauth/pending/create-account', {
      email: 'fresh@example.com',
      password: 'secret-123',
      verify_code: '123456',
      turnstile_token: 'create-token',
    REDACTED)
    expect(setTokenMock).toHaveBeenCalledWith('oauth-access-token')
  REDACTED)

  it('resets the pending oauth create-account turnstile after submit failure', async () => {
    authStoreState.pendingAuthSession = {
      token: 'pending-token-4',
      token_field: 'pending_auth_token',
      provider: 'oidc',
      redirect: '/profile',
    REDACTED
    getPublicSettingsMock.mockResolvedValue({
      turnstile_enabled: true,
      turnstile_site_key: 'site-key',
      site_name: 'Sub2API',
      registration_email_suffix_whitelist: ['allowed.com'],
    REDACTED)
    sessionStorage.setItem(
      'register_data',
      JSON.stringify({
        email: 'fresh@example.com',
        password: 'secret-123',
        turnstile_token: 'send-code-token',
      REDACTED)
    )
    apiClientPostMock.mockRejectedValue(new Error('invalid verify code'))

    const wrapper = mount(EmailVerifyView, {
      global: {
        stubs: {
          AuthLayout: { template: '<div><slot /><slot name="footer" /></div>' REDACTED,
          Icon: true,
          TurnstileWidget: {
            template: '<button data-testid="create-turnstile" @click="$emit(\'verify\', \'create-token\')">verify</button>',
            methods: {
              reset: createTurnstileResetMock,
            REDACTED,
          REDACTED,
          transition: false,
        REDACTED,
      REDACTED,
    REDACTED)

    await flushPromises()
    await wrapper.get('#code').setValue('123456')
    await wrapper.get('[data-testid="create-turnstile"]').trigger('click')
    expect(wrapper.get('button[type="submit"]').attributes('disabled')).toBeUndefined()

    await wrapper.get('form').trigger('submit.prevent')
    await flushPromises()

    expect(apiClientPostMock).toHaveBeenCalledWith('/auth/oauth/pending/create-account', {
      email: 'fresh@example.com',
      password: 'secret-123',
      verify_code: '123456',
      turnstile_token: 'create-token',
    REDACTED)
    expect(createTurnstileResetMock).toHaveBeenCalled()
    expect(wrapper.get('button[type="submit"]').attributes('disabled')).toBeDefined()
  REDACTED)

  it('returns to the oauth callback flow when pending account creation becomes bind-login', async () => {
    authStoreState.pendingAuthSession = {
      token: '',
      token_field: 'pending_oauth_token',
      provider: 'oidc',
      redirect: '/profile/security',
    REDACTED
    getPublicSettingsMock.mockResolvedValue({
      turnstile_enabled: false,
      turnstile_site_key: '',
      site_name: 'Sub2API',
      registration_email_suffix_whitelist: ['allowed.com'],
    REDACTED)
    sessionStorage.setItem(
      'register_data',
      JSON.stringify({
        email: 'fresh@example.com',
        password: 'secret-123',
      REDACTED)
    )
    apiClientPostMock.mockResolvedValue({
      data: {
        auth_result: 'pending_session',
        provider: 'oidc',
        step: 'bind_login_required',
        redirect: '/profile/security',
        email: 'fresh@example.com',
      REDACTED,
    REDACTED)

    const wrapper = mount(EmailVerifyView, {
      global: {
        stubs: {
          AuthLayout: { template: '<div><slot /><slot name="footer" /></div>' REDACTED,
          Icon: true,
          TurnstileWidget: true,
          transition: false,
        REDACTED,
      REDACTED,
    REDACTED)

    await flushPromises()
    await wrapper.get('#code').setValue('123456')
    await wrapper.get('form').trigger('submit.prevent')
    await flushPromises()

    expect(apiClientPostMock).toHaveBeenCalledWith('/auth/oauth/pending/create-account', {
      email: 'fresh@example.com',
      password: 'secret-123',
      verify_code: '123456',
    REDACTED)
    expect(setPendingAuthSessionMock).toHaveBeenCalledWith({
      token: '',
      token_field: 'pending_oauth_token',
      provider: 'oidc',
      redirect: '/profile/security',
    REDACTED)
    expect(pushMock).toHaveBeenCalledWith('/auth/oidc/callback')
    expect(setTokenMock).not.toHaveBeenCalled()
    expect(persistOAuthTokenContextMock).not.toHaveBeenCalled()
    expect(clearPendingAuthSessionMock).not.toHaveBeenCalled()
    expect(showSuccessMock).not.toHaveBeenCalled()
  REDACTED)

  it('keeps the normal email registration flow unchanged', async () => {
    sessionStorage.setItem(
      'register_data',
      JSON.stringify({
        email: 'normal@example.com',
        password: 'secret-456',
        promo_code: 'PROMO',
        invitation_code: 'INVITE',
      REDACTED)
    )
    registerMock.mockResolvedValue({REDACTED)

    const wrapper = mount(EmailVerifyView, {
      global: {
        stubs: {
          AuthLayout: { template: '<div><slot /><slot name="footer" /></div>' REDACTED,
          Icon: true,
          TurnstileWidget: true,
          transition: false,
        REDACTED,
      REDACTED,
    REDACTED)

    await flushPromises()
    await wrapper.get('#code').setValue('654321')
    await wrapper.get('form').trigger('submit.prevent')
    await flushPromises()

    expect(registerMock).toHaveBeenCalledWith({
      email: 'normal@example.com',
      password: 'secret-456',
      verify_code: '654321',
      turnstile_token: undefined,
      tencent_captcha_ticket: undefined,
      tencent_captcha_randstr: undefined,
      promo_code: 'PROMO',
      invitation_code: 'INVITE',
    REDACTED)
    expect(apiClientPostMock).not.toHaveBeenCalled()
    expect(pushMock).toHaveBeenCalledWith('/dashboard')
  REDACTED)

  it('does not require another Tencent proof for final email registration', async () => {
    getPublicSettingsMock.mockResolvedValue({
      turnstile_enabled: false,
      turnstile_site_key: '',
      tencent_captcha_enabled: true,
      tencent_captcha_app_id: 'tencent-app-id',
      site_name: 'Sub2API',
      registration_email_suffix_whitelist: [],
    REDACTED)
    sessionStorage.setItem(
      'register_data',
      JSON.stringify({
        email: 'normal@example.com',
        password: 'secret-456',
        tencent_captcha_ticket: 'send-code-ticket',
        tencent_captcha_randstr: '@send-code-rand',
      REDACTED)
    )
    registerMock.mockResolvedValue({REDACTED)

    const wrapper = mount(EmailVerifyView, {
      global: {
        stubs: {
          AuthLayout: { template: '<div><slot /><slot name="footer" /></div>' REDACTED,
          Icon: true,
          TurnstileWidget: {
            template: '<span />',
            methods: {
              reset: createTurnstileResetMock,
            REDACTED,
          REDACTED,
          transition: false,
        REDACTED,
      REDACTED,
    REDACTED)

    await flushPromises()
    expect(sendVerifyCodeMock).toHaveBeenCalledWith(expect.objectContaining({
      tencent_captcha_ticket: 'send-code-ticket',
      tencent_captcha_randstr: '@send-code-rand',
    REDACTED))
    expect(JSON.parse(sessionStorage.getItem('register_data') || '{REDACTED')).toEqual({
      email: 'normal@example.com',
      password: 'secret-456',
    REDACTED)

    await wrapper.get('#code').setValue('654321')
    await wrapper.get('form').trigger('submit.prevent')
    await flushPromises()

    expect(registerMock).toHaveBeenCalledWith(expect.objectContaining({
      email: 'normal@example.com',
      verify_code: '654321',
      tencent_captcha_ticket: undefined,
      tencent_captcha_randstr: undefined,
    REDACTED))
  REDACTED)
REDACTED)
