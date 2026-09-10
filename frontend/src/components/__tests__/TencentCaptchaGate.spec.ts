import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import TencentCaptchaGate from '@/components/TencentCaptchaGate.vue'
import { loadTencentCaptcha, resetTencentCaptchaLoaderForTest } from '@/utils/tencentCaptcha'

const locale = { value: 'zh' }

vi.mock('vue-i18n', () => ({
  useI18n: () => ({ locale })
}))

type CaptchaResult = {
  ret: number
  ticket?: string | null
  randstr?: string | null
  errorCode?: number
}

describe('TencentCaptchaGate', () => {
  beforeEach(() => {
    locale.value = 'zh'
    delete window.TencentCaptcha
    delete window.TCaptchaGlobal
    document.head
      .querySelectorAll('script[src*="TJCaptcha.js"], script[src*="TJNCaptcha-global.js"]')
      .forEach((node) => node.remove())
    resetTencentCaptchaLoaderForTest()
  })

  it('does not render a visible verification button', () => {
    const wrapper = mount(TencentCaptchaGate, { props: { appId: '123456789' } })

    expect(wrapper.find('button').exists()).toBe(false)
  })

  it('resolves proof after Tencent SDK success', async () => {
    let callback: ((result: CaptchaResult) => void) | undefined
    window.TencentCaptcha = class {
      constructor(_appId: string, resultCallback: (result: CaptchaResult) => void) {
        callback = resultCallback
      }
      show = vi.fn()
      destroy = vi.fn()
    }
    const wrapper = mount(TencentCaptchaGate, { props: { appId: '123456789' } })

    const verification = wrapper.vm.verify()
    await flushPromises()
    callback?.({ ret: 0, ticket: 'ticket-value', randstr: 'rand-value' })

    await expect(verification).resolves.toEqual({ ticket: 'ticket-value', randstr: 'rand-value' })
  })

  it('resolves null when the user closes the popup', async () => {
    let callback: ((result: CaptchaResult) => void) | undefined
    window.TencentCaptcha = class {
      constructor(_appId: string, resultCallback: (result: CaptchaResult) => void) {
        callback = resultCallback
      }
      show = vi.fn()
      destroy = vi.fn()
    }
    const wrapper = mount(TencentCaptchaGate, { props: { appId: '123456789' } })

    const verification = wrapper.vm.verify()
    await flushPromises()
    callback?.({ ret: 2, ticket: null })

    await expect(verification).resolves.toBeNull()
  })

  it('rejects SDK load failures and disaster-recovery tickets', async () => {
    const failedLoad = mount(TencentCaptchaGate, { props: { appId: '123456789' } })
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
      }
      show = vi.fn()
      destroy = vi.fn()
    }
    const failedResult = mount(TencentCaptchaGate, { props: { appId: '123456789' } })
    const resultVerification = failedResult.vm.verify()
    await flushPromises()
    callback?.({ ret: 0, ticket: 'trerror_1001_123456789', randstr: '@fallback', errorCode: 1001 })

    await expect(resultVerification).rejects.toThrow('Tencent Captcha verification failed')
  })

  it('reuses one pending promise for concurrent verify calls', async () => {
    const show = vi.fn()
    let callback: ((result: CaptchaResult) => void) | undefined
    window.TencentCaptcha = class {
      constructor(_appId: string, resultCallback: (result: CaptchaResult) => void) {
        callback = resultCallback
      }
      show = show
      destroy = vi.fn()
    }
    const wrapper = mount(TencentCaptchaGate, { props: { appId: '123456789' } })

    const first = wrapper.vm.verify()
    const second = wrapper.vm.verify()
    await flushPromises()
    callback?.({ ret: 0, ticket: 'ticket-value', randstr: 'rand-value' })

    await expect(first).resolves.toEqual({ ticket: 'ticket-value', randstr: 'rand-value' })
    await expect(second).resolves.toEqual({ ticket: 'ticket-value', randstr: 'rand-value' })
    expect(show).toHaveBeenCalledOnce()
  })

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
      }
      show = vi.fn()
      destroy = vi.fn()
    } as unknown as typeof window.TencentCaptcha
    const wrapper = mount(TencentCaptchaGate, {
      props: { appId: '123456789', region: 'intl' }
    })

    const verification = wrapper.vm.verify()
    await flushPromises()

    const container = wrapper.get('[data-testid="tencent-captcha-international-container"]')
    expect(args).toHaveLength(1)
    expect(args[0][0]).toBe(container.element)
    expect(args[0][0]).not.toBe(document.body)
    await vi.waitFor(() => expect(container.classes()).not.toContain('scale-[0.01]'))
    expect(args[0][1]).toBe('123456789')
    expect(typeof args[0][2]).toBe('function')
    expect(args[0][3]).toMatchObject({ enableAutoCheck: false, type: 'popup' })

    callback?.({ ret: 0, ticket: 'ticket-value', randstr: 'rand-value' })
    await expect(verification).resolves.toEqual({
      ticket: 'ticket-value',
      randstr: 'rand-value'
    })
    expect(container.classes()).not.toContain('scale-[0.01]')
  })

  it('preloads and displays the international checkbox on mount', async () => {
    window.TCaptchaGlobal = true
    window.TencentCaptcha = class {
      constructor() {}
      show = vi.fn()
      destroy = vi.fn()
    } as unknown as typeof window.TencentCaptcha
    const wrapper = mount(TencentCaptchaGate, {
      props: { appId: '123456789', region: 'intl' }
    })

    const container = wrapper.get('[data-testid="tencent-captcha-international-container"]')
    await flushPromises()
    await vi.waitFor(() => expect(container.classes()).not.toContain('scale-[0.01]'))
  })

  it('reuses a preloaded proof when the SDK reports success', async () => {
    let callback: ((result: CaptchaResult) => void) | undefined
    window.TCaptchaGlobal = true
    window.TencentCaptcha = class {
      constructor(...received: unknown[]) {
        callback = received[2] as (result: CaptchaResult) => void
      }
      show = vi.fn()
      destroy = vi.fn()
    } as unknown as typeof window.TencentCaptcha
    const wrapper = mount(TencentCaptchaGate, {
      props: { appId: '123456789', region: 'intl' }
    })

    await flushPromises()
    callback?.({ ret: 0, ticket: 'preloaded-ticket', randstr: 'preloaded-rand' })
    await flushPromises()

    await expect(wrapper.vm.verify()).resolves.toEqual({
      ticket: 'preloaded-ticket',
      randstr: 'preloaded-rand'
    })
    expect(wrapper.get('[data-testid="tencent-captcha-international-container"]').classes()).not.toContain(
      'scale-[0.01]'
    )
  })

  it('refreshes a preloaded proof before the provider ticket expires', async () => {
    vi.useFakeTimers()
    try {
      let callback: ((result: CaptchaResult) => void) | undefined
      let constructorCount = 0
      window.TCaptchaGlobal = true
      window.TencentCaptcha = class {
        constructor(...received: unknown[]) {
          constructorCount += 1
          callback = received[2] as (result: CaptchaResult) => void
        }
        show = vi.fn()
        destroy = vi.fn()
      } as unknown as typeof window.TencentCaptcha
      const wrapper = mount(TencentCaptchaGate, {
        props: { appId: '123456789', region: 'intl' }
      })

      await flushPromises()
      callback?.({ ret: 0, ticket: 'expired-ticket', randstr: 'expired-rand' })
      await flushPromises()
      vi.advanceTimersByTime(4 * 60 * 1000)

      const verification = wrapper.vm.verify()
      await flushPromises()
      expect(constructorCount).toBe(2)

      callback?.({ ret: 0, ticket: 'fresh-ticket', randstr: 'fresh-rand' })
      await expect(verification).resolves.toEqual({
        ticket: 'fresh-ticket',
        randstr: 'fresh-rand'
      })
    } finally {
      vi.useRealTimers()
    }
  })

  it('keeps the three-argument form for the Chinese mainland site', async () => {
    const args: unknown[][] = []
    window.TencentCaptcha = class {
      constructor(...received: unknown[]) {
        args.push(received)
      }
      show = vi.fn()
      destroy = vi.fn()
    } as unknown as typeof window.TencentCaptcha
    const wrapper = mount(TencentCaptchaGate, { props: { appId: '123456789' } })

    void wrapper.vm.verify()
    await flushPromises()

    expect(args[0][0]).toBe('123456789')
    expect(typeof args[0][1]).toBe('function')
  })

  it('loads the international SDK script for the international site', async () => {
    const wrapper = mount(TencentCaptchaGate, {
      props: { appId: '123456789', region: 'intl' }
    })
    void wrapper.vm.verify()
    await flushPromises()

    const script = document.head.querySelector<HTMLScriptElement>(
      'script[src*="TJNCaptcha-global.js"]'
    )
    expect(script?.src).toBe('https://ca.turing.captcha.qcloud.com/TJNCaptcha-global.js')
  })

  // 页面上已有的全局构造函数可能来自另一个站点（例如开发期 HMR 重置了模块状态）。
  // 两个站点签名不兼容，错配会抛错，因此站点不一致时必须重新加载对应脚本而不是复用。
  it('does not reuse a mainland global for the international site', async () => {
    window.TencentCaptcha = class {
      constructor() {}
      show = vi.fn()
      destroy = vi.fn()
    } as unknown as typeof window.TencentCaptcha

    const wrapper = mount(TencentCaptchaGate, {
      props: { appId: '123456789', region: 'intl' }
    })
    void wrapper.vm.verify()
    await flushPromises()

    expect(
      document.head.querySelector('script[src*="TJNCaptcha-global.js"]')
    ).not.toBeNull()
  })

  it('does not inject a second SDK when the region changes in one page', async () => {
    const constructor = class {
      constructor() {}
      show = vi.fn()
      destroy = vi.fn()
    } as unknown as typeof window.TencentCaptcha

    const firstLoad = loadTencentCaptcha('cn')
    const firstScript = document.head.querySelector<HTMLScriptElement>('script[src*="TJCaptcha.js"]')
    expect(firstScript).not.toBeNull()
    window.TencentCaptcha = constructor
    firstScript?.dispatchEvent(new Event('load'))
    await expect(firstLoad).resolves.toBe(constructor)

    await expect(loadTencentCaptcha('intl')).rejects.toThrow(
      'Tencent Captcha region changed; reload the page to apply it'
    )
    expect(document.head.querySelector('script[src*="TJNCaptcha-global.js"]')).toBeNull()
  })

  it('settles a pending verification when reset', async () => {
    const destroy = vi.fn()
    window.TencentCaptcha = class {
      constructor(_appId: string, _callback: (result: CaptchaResult) => void) {}
      show = vi.fn()
      destroy = destroy
    }
    const wrapper = mount(TencentCaptchaGate, { props: { appId: '123456789' } })

    const verification = wrapper.vm.verify()
    await flushPromises()
    wrapper.vm.reset()

    await expect(verification).resolves.toBeNull()
    expect(destroy).toHaveBeenCalledOnce()
  })

  it('reinitializes the international checkbox after reset', async () => {
    const destroy = vi.fn()
    let constructorCount = 0
    window.TCaptchaGlobal = true
    window.TencentCaptcha = class {
      constructor() {
        constructorCount += 1
      }
      show = vi.fn()
      destroy = destroy
    } as unknown as typeof window.TencentCaptcha
    const wrapper = mount(TencentCaptchaGate, {
      props: { appId: '123456789', region: 'intl' }
    })

    await flushPromises()
    expect(constructorCount).toBe(1)
    wrapper.vm.reset()
    await flushPromises()

    expect(destroy).toHaveBeenCalledOnce()
    expect(constructorCount).toBe(2)
    expect(
      wrapper.get('[data-testid="tencent-captcha-international-container"]').classes()
    ).not.toContain('scale-[0.01]')
  })
})
