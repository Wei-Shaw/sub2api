import { mount, flushPromises } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'

import CaptchaWidget from '@/components/CaptchaWidget.vue'

// 把三个子 widget stub 成可控的形态。每个 stub 暴露 execute / reset / lastError，
// 便于断言 dispatcher 是否正确转发 + 在天御 fallback 时同步 lastError。
const makeChildStub = (executeReturn: Record<string, string> | null, lastError = null) => ({
  setup() {
    return {
      execute: vi.fn(() => Promise.resolve(executeReturn)),
      reset: vi.fn(),
      lastError
    }
  },
  template: '<div class="stub-widget" />'
})

describe('CaptchaWidget dispatcher expose 行为', () => {
  it('provider=turnstile 时 execute() 转发到 TurnstileWidget 子组件', async () => {
    const wrapper = mount(CaptchaWidget, {
      props: { provider: 'turnstile', siteKey: 'tk-site' },
      global: {
        stubs: {
          TurnstileWidget: makeChildStub({ token: 'turnstile-token' }),
          HCaptchaWidget: makeChildStub(null),
          TencentCaptchaWidget: makeChildStub(null)
        }
      }
    })
    await flushPromises()

    const exposed = wrapper.vm as unknown as { execute: () => Promise<Record<string, string> | null> }
    const result = await exposed.execute()
    expect(result).toEqual({ token: 'turnstile-token' })
  })

  it('provider=hcaptcha 时 execute() 转发到 HCaptchaWidget 子组件', async () => {
    const wrapper = mount(CaptchaWidget, {
      props: { provider: 'hcaptcha', siteKey: 'hc-site' },
      global: {
        stubs: {
          TurnstileWidget: makeChildStub(null),
          HCaptchaWidget: makeChildStub({ token: 'hcaptcha-token' }),
          TencentCaptchaWidget: makeChildStub(null)
        }
      }
    })
    await flushPromises()

    const exposed = wrapper.vm as unknown as { execute: () => Promise<Record<string, string> | null> }
    const result = await exposed.execute()
    expect(result).toEqual({ token: 'hcaptcha-token' })
  })

  it('provider=tencent_captcha 时 execute() 转发并 resolve {ticket, randstr}', async () => {
    const wrapper = mount(CaptchaWidget, {
      props: { provider: 'tencent_captcha', siteKey: 'tencent-app-id' },
      global: {
        stubs: {
          TurnstileWidget: makeChildStub(null),
          HCaptchaWidget: makeChildStub(null),
          TencentCaptchaWidget: makeChildStub({ ticket: 'tx', randstr: '@1A' })
        }
      }
    })
    await flushPromises()

    const exposed = wrapper.vm as unknown as { execute: () => Promise<Record<string, string> | null> }
    const result = await exposed.execute()
    expect(result).toEqual({ ticket: 'tx', randstr: '@1A' })
  })

  it('天御子组件 emit fallback 时 dispatcher 暴露的 lastError = "fallback"', async () => {
    // 用真实 emit 来触发 onTencentFallback；stub 模板里加按钮触发即可。
    const wrapper = mount(CaptchaWidget, {
      props: { provider: 'tencent_captcha', siteKey: 'site' },
      global: {
        stubs: {
          TurnstileWidget: makeChildStub(null),
          HCaptchaWidget: makeChildStub(null),
          TencentCaptchaWidget: {
            emits: ['verify', 'fallback', 'error'],
            template: '<button data-testid="trigger-fallback" @click="$emit(\'fallback\')" />'
          }
        }
      }
    })
    await flushPromises()

    const exposed = wrapper.vm as unknown as { lastError: string | null }
    expect(exposed.lastError).toBeNull()
    await wrapper.get('[data-testid="trigger-fallback"]').trigger('click')
    expect(exposed.lastError).toBe('fallback')
  })

  it('execute() 触发前会重置 lastError，避免上一轮残留误导上层', async () => {
    const wrapper = mount(CaptchaWidget, {
      props: { provider: 'turnstile', siteKey: 'site' },
      global: {
        stubs: {
          TurnstileWidget: {
            setup() {
              return {
                execute: vi.fn(() => Promise.resolve({ token: 'tk' } as Record<string, string>)),
                reset: vi.fn(),
                lastError: null
              }
            },
            emits: ['verify', 'expire', 'error'],
            template: '<button data-testid="trigger-error" @click="$emit(\'error\')" />'
          },
          HCaptchaWidget: makeChildStub(null),
          TencentCaptchaWidget: makeChildStub(null)
        }
      }
    })
    await flushPromises()

    // 先制造一个 error 状态
    await wrapper.get('[data-testid="trigger-error"]').trigger('click')
    const exposed = wrapper.vm as unknown as {
      lastError: string | null
      execute: () => Promise<Record<string, string> | null>
    }
    expect(exposed.lastError).toBe('error')

    await exposed.execute()
    expect(exposed.lastError).toBeNull()
  })
})
