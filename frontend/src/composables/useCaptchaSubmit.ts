import type { Ref } from 'vue'

import type CaptchaWidget from '@/components/CaptchaWidget.vue'

/**
 * useCaptchaSubmit 抽出 4 个 captcha-gated 表单（Login/Register/ForgotPassword/EmailVerify）
 * 共享的 submit 状态机：
 *
 * 状态流转：
 *   1. 调用 captcha.execute() 拿 payload
 *      a. 返回有效 payload → 调用 submitFn(payload)
 *      b. 返回 null → 直接抛错由调用方处理（用户取消 / widget error / fallback）
 *   2. submitFn 抛出错误 → 透传给调用方处理
 *
 * 注意：
 *   - 不做前端自动重试；任何验证失败（包括天御容灾 trerror_ 票据）都直接抛错。
 *   - 调用方仍负责拼接最终请求 body（把 payload 注入 captcha_payload 字段），便于不同表单各自定制。
 *
 * 使用示例：
 *   const { submit, isLoading } = useCaptchaSubmit({
 *     captchaRef,
 *     captchaEnabled: () => captchaEnabled.value,
 *     getCachedToken: () => captchaToken.value,
 *     submitFn: async (payload) => {
 *       await authStore.login({ email, password, captcha_payload: payload })
 *     }
 *   })
 *   await submit()
 */

export type CaptchaPayload = Record<string, string>

export interface CaptchaSubmitError extends Error {
  /**
   * 错误归因：
   *   - 'cancelled'：用户取消或 widget error / fallback
   *   - 'submit'：业务 submitFn 抛出的错误（透传 cause）
   */
  reason: 'cancelled' | 'submit'
}

const newCaptchaError = (
  reason: CaptchaSubmitError['reason'],
  message: string,
  cause?: unknown
): CaptchaSubmitError => {
  const err = new Error(message) as CaptchaSubmitError
  err.reason = reason
  if (cause !== undefined) {
    Object.assign(err, { cause })
  }
  return err
}

interface UseCaptchaSubmitOptions {
  /** CaptchaWidget 组件 ref */
  captchaRef: Ref<InstanceType<typeof CaptchaWidget> | null>
  /** 是否启用 captcha；不启用时跳过 execute()，直接以空 payload 调用 submitFn */
  captchaEnabled: () => boolean
  /**
   * 已通过 verify 事件缓存到 view 内部的 token；优先用它构造 {token} 避免反复 execute()。
   * Tencent 场景下应始终返回空（popup 类型每次都要 execute）。
   * 可选：如果 view 不维护这个状态可省略。
   */
  getCachedToken?: () => string
  /** 实际业务调用，拿到 payload 后由调用方拼接到请求 body 并发起 API。 */
  submitFn: (payload: CaptchaPayload) => Promise<void>
}

interface UseCaptchaSubmitReturn {
  /**
   * 状态机驱动的 submit；调用方应在自己的 isLoading 态外层包一层；本 composable 不接管按钮 disabled。
   * 失败时抛 CaptchaSubmitError；reason='submit' 时 cause 为 submitFn 原始错误。
   */
  submit: () => Promise<void>
}

export function useCaptchaSubmit(options: UseCaptchaSubmitOptions): UseCaptchaSubmitReturn {
  const { captchaRef, captchaEnabled, getCachedToken, submitFn } = options

  const acquirePayload = async (): Promise<CaptchaPayload | null> => {
    if (!captchaEnabled()) {
      return {}
    }
    const cached = getCachedToken?.()
    if (cached) {
      return { token: cached }
    }
    const widget = captchaRef.value
    if (!widget) {
      return null
    }
    return await widget.execute()
  }

  const submit = async (): Promise<void> => {
    const payload = await acquirePayload()

    // payload 为 null：widget 返回 null（用户取消 / widget error / fallback），直接抛错。
    if (payload === null) {
      throw newCaptchaError('cancelled', 'captcha cancelled or failed')
    }

    // payload 拿到了，调用 submitFn；submitFn 的错误透传给调用方。
    try {
      await submitFn(payload)
    } catch (err: unknown) {
      throw newCaptchaError('submit', 'captcha submit failed', err)
    }
  }

  return { submit }
}

