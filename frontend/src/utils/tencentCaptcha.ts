export interface TencentCaptchaProof {
  ticket: string
  randstr: string
}

export interface TencentCaptchaResult {
  ret: number
  ticket?: string | null
  randstr?: string | null
  errorCode?: number
  errorMessage?: string
}

interface TencentCaptchaInstance {
  show(): void
  destroy(): void
}

type TencentCaptchaConstructor = new (
  appId: string,
  callback: (result: TencentCaptchaResult) => void,
  options?: Record<string, unknown>
) => TencentCaptchaInstance

declare global {
  interface Window {
    TencentCaptcha?: TencentCaptchaConstructor
  }
}

const SCRIPT_SRC = 'https://turing.captcha.qcloud.com/TJCaptcha.js'
let scriptPromise: Promise<TencentCaptchaConstructor> | null = null

export function loadTencentCaptcha(): Promise<TencentCaptchaConstructor> {
  if (window.TencentCaptcha) return Promise.resolve(window.TencentCaptcha)
  if (scriptPromise) return scriptPromise

  scriptPromise = new Promise((resolve, reject) => {
    const script = document.createElement('script')
    script.src = SCRIPT_SRC
    script.async = true
    script.onload = () => {
      if (window.TencentCaptcha) {
        resolve(window.TencentCaptcha)
        return
      }
      scriptPromise = null
      reject(new Error('Tencent Captcha SDK is unavailable'))
    }
    script.onerror = () => {
      scriptPromise = null
      reject(new Error('Failed to load Tencent Captcha SDK'))
    }
    document.head.appendChild(script)
  })

  return scriptPromise
}

export function resetTencentCaptchaLoaderForTest(): void {
  scriptPromise = null
}
