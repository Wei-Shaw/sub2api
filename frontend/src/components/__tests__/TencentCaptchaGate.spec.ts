import { flushPromises, mount REDACTED from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi REDACTED from 'vitest'
import TencentCaptchaGate from '@/components/TencentCaptchaGate.vue'
import { resetTencentCaptchaLoaderForTest REDACTED from '@/utils/tencentCaptcha'

const locale = { value: 'zh' REDACTED

vi.mock('vue-i18n', () => ({
  useI18n: () => ({ locale REDACTED)
REDACTED))

type CaptchaResult = {
  ret: number
  ticket?: string | null
  randstr?: string | null
  errorCode?: number
REDACTED

describe('TencentCaptchaGate', () => {
  beforeEach(() => {
    locale.value = 'zh'
    delete window.TencentCaptcha
    document.head.querySelectorAll('script[src*="TJCaptcha.js"]').forEach((node) => node.remove())
    resetTencentCaptchaLoaderForTest()
  REDACTED)

  it('does not render a visible verification button', () => {
    const wrapper = mount(TencentCaptchaGate, { props: { appId: '123456789' REDACTED REDACTED)

    expect(wrapper.find('button').exists()).toBe(false)
  REDACTED)

  it('resolves proof after Tencent SDK success', async () => {
    let callback: ((result: CaptchaResult) => void) | undefined
    window.TencentCaptcha = class {
      constructor(_appId: string, resultCallback: (result: CaptchaResult) => void) {
        callback = resultCallback
      REDACTED
      show = vi.fn()
      destroy = vi.fn()
    REDACTED
    const wrapper = mount(TencentCaptchaGate, { props: { appId: '123456789' REDACTED REDACTED)

    const verification = wrapper.vm.verify()
    await flushPromises()
    callback?.({ ret: 0, ticket: 'ticket-value', randstr: 'rand-value' REDACTED)

    await expect(verification).resolves.toEqual({ ticket: 'ticket-value', randstr: 'rand-value' REDACTED)
  REDACTED)

  it('resolves null when the user closes the popup', async () => {
    let callback: ((result: CaptchaResult) => void) | undefined
    window.TencentCaptcha = class {
      constructor(_appId: string, resultCallback: (result: CaptchaResult) => void) {
        callback = resultCallback
      REDACTED
      show = vi.fn()
      destroy = vi.fn()
    REDACTED
    const wrapper = mount(TencentCaptchaGate, { props: { appId: '123456789' REDACTED REDACTED)

    const verification = wrapper.vm.verify()
    await flushPromises()
    callback?.({ ret: 2, ticket: null REDACTED)

    await expect(verification).resolves.toBeNull()
  REDACTED)

  it('rejects SDK load failures and disaster-recovery tickets', async () => {
    const failedLoad = mount(TencentCaptchaGate, { props: { appId: '123456789' REDACTED REDACTED)
    const loadVerification = failedLoad.vm.verify()
    const script = document.head.querySelector<HTMLScriptElement>('script[src*="TJCaptcha.js"]')
    expect(script).not.toBeNull()
    script?.dispatchEvent(new Event('error'))
    await expect(loadVerification).rejects.toThrow('Failed to load Tencent Captcha SDK')

    let callback: ((result: CaptchaResult) => void) | undefined
    window.TencentCaptcha = class {
      constructor(_appId: string, resultCallback: (result: CaptchaResult) => void) {
        callback = resultCallback
      REDACTED
      show = vi.fn()
      destroy = vi.fn()
    REDACTED
    const failedResult = mount(TencentCaptchaGate, { props: { appId: '123456789' REDACTED REDACTED)
    const resultVerification = failedResult.vm.verify()
    await flushPromises()
    callback?.({ ret: 0, ticket: 'trerror_1001_123456789', randstr: '@fallback', errorCode: 1001 REDACTED)

    await expect(resultVerification).rejects.toThrow('Tencent Captcha verification failed')
  REDACTED)

  it('reuses one pending promise for concurrent verify calls', async () => {
    const show = vi.fn()
    let callback: ((result: CaptchaResult) => void) | undefined
    window.TencentCaptcha = class {
      constructor(_appId: string, resultCallback: (result: CaptchaResult) => void) {
        callback = resultCallback
      REDACTED
      show = show
      destroy = vi.fn()
    REDACTED
    const wrapper = mount(TencentCaptchaGate, { props: { appId: '123456789' REDACTED REDACTED)

    const first = wrapper.vm.verify()
    const second = wrapper.vm.verify()
    await flushPromises()
    callback?.({ ret: 0, ticket: 'ticket-value', randstr: 'rand-value' REDACTED)

    await expect(first).resolves.toEqual({ ticket: 'ticket-value', randstr: 'rand-value' REDACTED)
    await expect(second).resolves.toEqual({ ticket: 'ticket-value', randstr: 'rand-value' REDACTED)
    expect(show).toHaveBeenCalledOnce()
  REDACTED)

  it('settles a pending verification when reset', async () => {
    const destroy = vi.fn()
    window.TencentCaptcha = class {
      constructor(_appId: string, _callback: (result: CaptchaResult) => void) {REDACTED
      show = vi.fn()
      destroy = destroy
    REDACTED
    const wrapper = mount(TencentCaptchaGate, { props: { appId: '123456789' REDACTED REDACTED)

    const verification = wrapper.vm.verify()
    await flushPromises()
    wrapper.vm.reset()

    await expect(verification).resolves.toBeNull()
    expect(destroy).toHaveBeenCalledOnce()
  REDACTED)
REDACTED)
