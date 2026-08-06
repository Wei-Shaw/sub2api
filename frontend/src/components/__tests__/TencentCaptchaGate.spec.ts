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
    delete window.TCaptchaGlobal
    document.head
      .querySelectorAll('script[src*="TJCaptcha.js"], script[src*="TJNCaptcha-global.js"]')
      .forEach((node) => node.remove())
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
    await flushPromises()
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

  // 国际站新版 SDK 会先把 Robot checkbox 渲染到首参容器，点击后才弹出挑战。
  // 容器必须位于当前表单中；传 document.body 会让 checkbox 落在页面末尾并超出视口。
  it('passes a visible in-form container first for the international site', async () => {
    const args: unknown[][] = []
    let callback: ((result: CaptchaResult) => void) | undefined
    window.TCaptchaGlobal = true
    window.TencentCaptcha = class {
      constructor(...received: unknown[]) {
        args.push(received)
        callback = received[2] as (result: CaptchaResult) => void
      REDACTED
      show = vi.fn()
      destroy = vi.fn()
    REDACTED as unknown as typeof window.TencentCaptcha
    const wrapper = mount(TencentCaptchaGate, {
      props: { appId: '123456789', region: 'intl' REDACTED
    REDACTED)

    const verification = wrapper.vm.verify()
    await flushPromises()

    const container = wrapper.get('[data-testid="tencent-captcha-international-container"]')
    expect(args).toHaveLength(1)
    expect(args[0][0]).toBe(container.element)
    expect(args[0][0]).not.toBe(document.body)
    await vi.waitFor(() => expect(container.classes()).not.toContain('scale-[0.01]'))
    expect(args[0][1]).toBe('123456789')
    expect(typeof args[0][2]).toBe('function')
    expect(args[0][3]).toMatchObject({ enableAutoCheck: false, type: 'popup' REDACTED)

    callback?.({ ret: 0, ticket: 'ticket-value', randstr: 'rand-value' REDACTED)
    await expect(verification).resolves.toEqual({
      ticket: 'ticket-value',
      randstr: 'rand-value'
    REDACTED)
    expect(container.classes()).not.toContain('scale-[0.01]')
  REDACTED)

  it('preloads and displays the international checkbox on mount', async () => {
    window.TCaptchaGlobal = true
    window.TencentCaptcha = class {
      constructor() {REDACTED
      show = vi.fn()
      destroy = vi.fn()
    REDACTED as unknown as typeof window.TencentCaptcha
    const wrapper = mount(TencentCaptchaGate, {
      props: { appId: '123456789', region: 'intl' REDACTED
    REDACTED)

    const container = wrapper.get('[data-testid="tencent-captcha-international-container"]')
    await flushPromises()
    await vi.waitFor(() => expect(container.classes()).not.toContain('scale-[0.01]'))
  REDACTED)

  it('reuses a preloaded proof when the SDK reports success', async () => {
    let callback: ((result: CaptchaResult) => void) | undefined
    window.TCaptchaGlobal = true
    window.TencentCaptcha = class {
      constructor(...received: unknown[]) {
        callback = received[2] as (result: CaptchaResult) => void
      REDACTED
      show = vi.fn()
      destroy = vi.fn()
    REDACTED as unknown as typeof window.TencentCaptcha
    const wrapper = mount(TencentCaptchaGate, {
      props: { appId: '123456789', region: 'intl' REDACTED
    REDACTED)

    await flushPromises()
    callback?.({ ret: 0, ticket: 'preloaded-ticket', randstr: 'preloaded-rand' REDACTED)
    await flushPromises()

    await expect(wrapper.vm.verify()).resolves.toEqual({
      ticket: 'preloaded-ticket',
      randstr: 'preloaded-rand'
    REDACTED)
    expect(wrapper.get('[data-testid="tencent-captcha-international-container"]').classes()).not.toContain(
      'scale-[0.01]'
    )
  REDACTED)

  it('keeps the three-argument form for the Chinese mainland site', async () => {
    const args: unknown[][] = []
    window.TencentCaptcha = class {
      constructor(...received: unknown[]) {
        args.push(received)
      REDACTED
      show = vi.fn()
      destroy = vi.fn()
    REDACTED as unknown as typeof window.TencentCaptcha
    const wrapper = mount(TencentCaptchaGate, { props: { appId: '123456789' REDACTED REDACTED)

    void wrapper.vm.verify()
    await flushPromises()

    expect(args[0][0]).toBe('123456789')
    expect(typeof args[0][1]).toBe('function')
  REDACTED)

  it('loads the international SDK script for the international site', async () => {
    const wrapper = mount(TencentCaptchaGate, {
      props: { appId: '123456789', region: 'intl' REDACTED
    REDACTED)
    void wrapper.vm.verify()
    await flushPromises()

    const script = document.head.querySelector<HTMLScriptElement>(
      'script[src*="TJNCaptcha-global.js"]'
    )
    expect(script?.src).toBe('https://ca.turing.captcha.qcloud.com/TJNCaptcha-global.js')
  REDACTED)

  // 页面上已有的全局构造函数可能来自另一个站点（例如开发期 HMR 重置了模块状态）。
  // 两个站点签名不兼容，错配会抛错，因此站点不一致时必须重新加载对应脚本而不是复用。
  it('does not reuse a mainland global for the international site', async () => {
    window.TencentCaptcha = class {
      constructor() {REDACTED
      show = vi.fn()
      destroy = vi.fn()
    REDACTED as unknown as typeof window.TencentCaptcha

    const wrapper = mount(TencentCaptchaGate, {
      props: { appId: '123456789', region: 'intl' REDACTED
    REDACTED)
    void wrapper.vm.verify()
    await flushPromises()

    expect(
      document.head.querySelector('script[src*="TJNCaptcha-global.js"]')
    ).not.toBeNull()
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

  it('reinitializes the international checkbox after reset', async () => {
    const destroy = vi.fn()
    let constructorCount = 0
    window.TCaptchaGlobal = true
    window.TencentCaptcha = class {
      constructor() {
        constructorCount += 1
      REDACTED
      show = vi.fn()
      destroy = destroy
    REDACTED as unknown as typeof window.TencentCaptcha
    const wrapper = mount(TencentCaptchaGate, {
      props: { appId: '123456789', region: 'intl' REDACTED
    REDACTED)

    await flushPromises()
    expect(constructorCount).toBe(1)
    wrapper.vm.reset()
    await flushPromises()

    expect(destroy).toHaveBeenCalledOnce()
    expect(constructorCount).toBe(2)
    expect(
      wrapper.get('[data-testid="tencent-captcha-international-container"]').classes()
    ).not.toContain('scale-[0.01]')
  REDACTED)
REDACTED)
