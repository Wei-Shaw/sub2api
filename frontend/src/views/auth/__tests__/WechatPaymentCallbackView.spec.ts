import { flushPromises, mount REDACTED from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi REDACTED from 'vitest'
import WechatPaymentCallbackView from '@/views/auth/WechatPaymentCallbackView.vue'

const { replaceMock, routeState, locationState, showErrorMock REDACTED = vi.hoisted(() => ({
  replaceMock: vi.fn(),
  routeState: {
    query: {REDACTED as Record<string, unknown>,
  REDACTED,
  locationState: {
    current: {
      href: 'http://localhost/auth/wechat/payment/callback',
      hash: '',
      search: '',
      pathname: '/auth/wechat/payment/callback',
      origin: 'http://localhost',
    REDACTED as Location & { origin: string REDACTED,
  REDACTED,
  showErrorMock: vi.fn(),
REDACTED))

vi.mock('vue-router', () => ({
  useRoute: () => routeState,
  useRouter: () => ({
    replace: replaceMock,
  REDACTED),
REDACTED))

vi.mock('vue-i18n', () => ({
  useI18n: () => ({
    t: (key: string) => {
      if (key === 'auth.wechatPayment.callbackTitle') return '正在恢复微信支付'
      if (key === 'auth.wechatPayment.callbackProcessing') return '正在恢复微信支付...'
      if (key === 'auth.wechatPayment.backToPayment') return '返回支付页'
      if (key === 'auth.wechatPayment.callbackMissingResumeToken') return '微信支付回调缺少恢复令牌。'
      return key
    REDACTED,
    locale: { value: 'zh-CN' REDACTED,
  REDACTED),
REDACTED))

vi.mock('@/stores', () => ({
  useAppStore: () => ({
    showError: (...args: any[]) => showErrorMock(...args),
  REDACTED),
REDACTED))

describe('WechatPaymentCallbackView', () => {
  beforeEach(() => {
    replaceMock.mockReset()
    showErrorMock.mockReset()
    routeState.query = {REDACTED
    locationState.current = {
      href: 'http://localhost/auth/wechat/payment/callback',
      hash: '',
      search: '',
      pathname: '/auth/wechat/payment/callback',
      origin: 'http://localhost',
    REDACTED as Location & { origin: string REDACTED
    Object.defineProperty(window, 'location', {
      configurable: true,
      value: locationState.current,
    REDACTED)
  REDACTED)

  it('redirects back to purchase with an opaque resume token from hash fragment', async () => {
    locationState.current.hash = '#wechat_resume_token=resume-token-123&redirect=%2Fpurchase%3Ffrom%3Dwechat'

    mount(WechatPaymentCallbackView)
    await flushPromises()

    expect(replaceMock).toHaveBeenCalledWith({
      path: '/purchase',
      query: {
        from: 'wechat',
        wechat_resume: '1',
        wechat_resume_token: 'resume-token-123',
      REDACTED,
    REDACTED)
  REDACTED)

  it('redirects legacy openid callback payloads back to purchase while preserving resume context', async () => {
    locationState.current.hash =
      '#openid=openid-123&state=oauth-state&scope=snsapi_base&payment_type=wxpay_direct&amount=128&order_type=subscription&plan_id=7&redirect=%2Fpayment%3Ffrom%3Dwechat'

    mount(WechatPaymentCallbackView)
    await flushPromises()

    expect(replaceMock).toHaveBeenCalledWith({
      path: '/purchase',
      query: {
        from: 'wechat',
        wechat_resume: '1',
        openid: 'openid-123',
        state: 'oauth-state',
        scope: 'snsapi_base',
        payment_type: 'wxpay_direct',
        amount: '128',
        order_type: 'subscription',
        plan_id: '7',
      REDACTED,
    REDACTED)
  REDACTED)

  it('shows an error when the callback payload is missing the resume token', async () => {
    locationState.current.hash = '#payment_type=wxpay'

    const wrapper = mount(WechatPaymentCallbackView)
    await flushPromises()

    expect(replaceMock).not.toHaveBeenCalled()
    expect(showErrorMock).toHaveBeenCalledWith('微信支付回调缺少恢复令牌。')
    expect(wrapper.text()).toContain('微信支付回调缺少恢复令牌。')
    expect(wrapper.find('.bg-red-50').exists()).toBe(false)
  REDACTED)
REDACTED)
