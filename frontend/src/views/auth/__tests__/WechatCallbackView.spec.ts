import { flushPromises, mount REDACTED from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi REDACTED from 'vitest'
import WechatCallbackView from '@/views/auth/WechatCallbackView.vue'

const {
  postMock,
  replaceMock,
  setTokenMock,
  showSuccessMock,
  showErrorMock,
  routeState,
REDACTED = vi.hoisted(() => ({
  postMock: vi.fn(),
  replaceMock: vi.fn(),
  setTokenMock: vi.fn(),
  showSuccessMock: vi.fn(),
  showErrorMock: vi.fn(),
  routeState: {
    query: {REDACTED as Record<string, unknown>,
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

vi.mock('@/api/client', () => ({
  apiClient: {
    post: postMock,
  REDACTED,
REDACTED))

describe('WechatCallbackView', () => {
  beforeEach(() => {
    postMock.mockReset()
    replaceMock.mockReset()
    setTokenMock.mockReset()
    showSuccessMock.mockReset()
    showErrorMock.mockReset()
    routeState.query = {REDACTED
    localStorage.clear()
  REDACTED)

  it('does not send adoption decisions during the initial exchange', async () => {
    postMock.mockResolvedValueOnce({
      data: {
        access_token: 'access-token',
        refresh_token: 'refresh-token',
        expires_in: 3600,
        redirect: '/dashboard',
        adoption_required: true,
      REDACTED,
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

    expect(postMock).toHaveBeenCalledWith('/auth/oauth/pending/exchange', {REDACTED)
    expect(postMock).toHaveBeenCalledTimes(1)
  REDACTED)

  it('waits for explicit adoption confirmation before finishing a non-invitation login', async () => {
    postMock
      .mockResolvedValueOnce({
        data: {
          redirect: '/dashboard',
          adoption_required: true,
          suggested_display_name: 'WeChat Nick',
          suggested_avatar_url: 'https://cdn.example/wechat.png',
        REDACTED,
      REDACTED)
      .mockResolvedValueOnce({
        data: {
          access_token: 'wechat-access-token',
          refresh_token: 'wechat-refresh-token',
          expires_in: 3600,
          token_type: 'Bearer',
          redirect: '/dashboard',
        REDACTED,
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

    expect(postMock).toHaveBeenNthCalledWith(1, '/auth/oauth/pending/exchange', {REDACTED)
    expect(postMock).toHaveBeenNthCalledWith(2, '/auth/oauth/pending/exchange', {
      adopt_display_name: true,
      adopt_avatar: false,
    REDACTED)
    expect(setTokenMock).toHaveBeenCalledWith('wechat-access-token')
    expect(replaceMock).toHaveBeenCalledWith('/dashboard')
    expect(localStorage.getItem('refresh_token')).toBe('wechat-refresh-token')
  REDACTED)

  it('renders adoption choices for invitation flow and submits the selected values', async () => {
    postMock
      .mockResolvedValueOnce({
        data: {
          error: 'invitation_required',
          redirect: '/subscriptions',
          adoption_required: true,
          suggested_display_name: 'WeChat Nick',
          suggested_avatar_url: 'https://cdn.example/wechat.png',
        REDACTED,
      REDACTED)
      .mockResolvedValueOnce({
        data: {
          access_token: 'wechat-invite-token',
          refresh_token: 'wechat-invite-refresh',
          expires_in: 600,
          token_type: 'Bearer',
        REDACTED,
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

    expect(postMock).toHaveBeenNthCalledWith(2, '/auth/oauth/wechat/complete-registration', {
      invitation_code: 'INVITE-CODE',
      adopt_display_name: false,
      adopt_avatar: true,
    REDACTED)
    expect(setTokenMock).toHaveBeenCalledWith('wechat-invite-token')
    expect(replaceMock).toHaveBeenCalledWith('/subscriptions')
  REDACTED)
REDACTED)
