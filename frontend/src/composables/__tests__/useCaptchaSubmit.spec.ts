import { describe, expect, it, vi } from 'vitest'
import { ref } from 'vue'

import {
  useCaptchaSubmit,
  type CaptchaSubmitError
} from '@/composables/useCaptchaSubmit'

// 构造一个最简的 widget mock，模拟 CaptchaWidget defineExpose 的 execute() / lastError 形态。
const makeWidget = (
  executeImpl: () => Promise<Record<string, string> | null>,
  lastError: string | null = null
) =>
  ref({
    execute: vi.fn(executeImpl),
    lastError,
    reset: vi.fn()
  } as unknown as InstanceType<typeof import('@/components/CaptchaWidget.vue').default>)

describe('useCaptchaSubmit', () => {
  it('captcha disabled 时跳过 execute() 直接调 submitFn 并传空 payload', async () => {
    const submitFn = vi.fn().mockResolvedValue(undefined)
    const widget = makeWidget(() => Promise.resolve(null))

    const { submit } = useCaptchaSubmit({
      captchaRef: widget,
      captchaEnabled: () => false,
      submitFn
    })

    await submit()

    expect(submitFn).toHaveBeenCalledTimes(1)
    expect(submitFn.mock.calls[0][0]).toEqual({})
  })

  it('使用 getCachedToken 命中时不调用 execute()', async () => {
    const executeFn = vi.fn(() => Promise.resolve(null))
    const submitFn = vi.fn().mockResolvedValue(undefined)
    const widget = makeWidget(executeFn)

    const { submit } = useCaptchaSubmit({
      captchaRef: widget,
      captchaEnabled: () => true,
      getCachedToken: () => 'cached-token',
      submitFn
    })

    await submit()

    expect(executeFn).not.toHaveBeenCalled()
    expect(submitFn).toHaveBeenCalledWith({ token: 'cached-token' })
  })

  it('execute() 返回 payload 时调 submitFn 并把 payload 透传给业务', async () => {
    const submitFn = vi.fn().mockResolvedValue(undefined)
    const widget = makeWidget(() => Promise.resolve({ ticket: 't', randstr: 'r' }))

    const { submit } = useCaptchaSubmit({
      captchaRef: widget,
      captchaEnabled: () => true,
      submitFn
    })

    await submit()

    expect(submitFn).toHaveBeenCalledWith({ ticket: 't', randstr: 'r' })
  })

  it('execute() 返回 null 时直接抛 cancelled 错误（不重试）', async () => {
    const widget = makeWidget(() => Promise.resolve(null), 'fallback')
    const submitFn = vi.fn()

    const { submit } = useCaptchaSubmit({
      captchaRef: widget,
      captchaEnabled: () => true,
      submitFn
    })

    await expect(submit()).rejects.toMatchObject({
      reason: 'cancelled'
    } satisfies Partial<CaptchaSubmitError>)

    expect(submitFn).not.toHaveBeenCalled()
  })

  it('execute() 返回 null 且非 fallback 时也抛 cancelled 错误', async () => {
    const widget = makeWidget(() => Promise.resolve(null), null)
    const submitFn = vi.fn()

    const { submit } = useCaptchaSubmit({
      captchaRef: widget,
      captchaEnabled: () => true,
      submitFn
    })

    await expect(submit()).rejects.toMatchObject({
      reason: 'cancelled'
    } satisfies Partial<CaptchaSubmitError>)
  })

  it('submitFn 抛错时透传 reason=submit + cause', async () => {
    const businessError = new Error('email already taken')
    const submitFn = vi.fn().mockRejectedValue(businessError)
    const widget = makeWidget(() => Promise.resolve({ token: 'tk' }))

    const { submit } = useCaptchaSubmit({
      captchaRef: widget,
      captchaEnabled: () => true,
      submitFn
    })

    await expect(submit()).rejects.toMatchObject({
      reason: 'submit'
    } satisfies Partial<CaptchaSubmitError>)
    // submitFn 只调用一次（不重试）
    expect(submitFn).toHaveBeenCalledTimes(1)
  })

  it('submitFn 抛 captcha 相关错误时也不重试，直接 reason=submit', async () => {
    const submitFn = vi.fn().mockRejectedValue({
      response: { data: { metadata: { captcha_error_code: 'captcha.tencent.fallback_ticket' } } }
    })
    const widget = makeWidget(() => Promise.resolve({ ticket: 'tx', randstr: 'r' }))

    const { submit } = useCaptchaSubmit({
      captchaRef: widget,
      captchaEnabled: () => true,
      submitFn
    })

    await expect(submit()).rejects.toMatchObject({
      reason: 'submit'
    } satisfies Partial<CaptchaSubmitError>)

    // 只调用一次，不自动重试
    expect(submitFn).toHaveBeenCalledTimes(1)
  })
})
