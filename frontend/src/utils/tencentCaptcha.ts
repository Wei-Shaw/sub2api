// 腾讯天御验证码站点：cn=中国站（cloud.tencent.com），intl=国际站（tencentcloud.com）。
// 两个站点的 CaptchaAppId 不互通，SDK 脚本与服务端校验接入点都必须成对使用。
export type TencentCaptchaRegion = 'cn' | 'intl'

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

type TencentCaptchaCallback = (result: TencentCaptchaResult) => void

// 两个站点的构造函数签名不同，且不可互换：
// - 中国站 TJCaptcha.js：第一个参数是 CaptchaAppId 字符串（传 DOM 走另一条分支）。
// - 国际站 TJNCaptcha-global.js：第一个参数必须是承载 Robot checkbox 的 DOM 容器，传字符串会直接抛
//   "The parameter of the constructor of the Captcha is passed incorrectly"。
type TencentCaptchaConstructor = {
  new (
    appId: string,
    callback: TencentCaptchaCallback,
    options?: Record<string, unknown>
  ): TencentCaptchaInstance
  new (
    element: HTMLElement,
    appId: string,
    callback: TencentCaptchaCallback,
    options?: Record<string, unknown>
  ): TencentCaptchaInstance
}

declare global {
  interface Window {
    TencentCaptcha?: TencentCaptchaConstructor
    // 国际站入口脚本会置为 true；国内站脚本只读取、从不写入。
    // 用于判定页面上已存在的 TencentCaptcha 属于哪个站点。
    TCaptchaGlobal?: boolean
  }
}

const SCRIPT_SRC: Record<TencentCaptchaRegion, string> = {
  cn: 'https://turing.captcha.qcloud.com/TJCaptcha.js',
  intl: 'https://ca.turing.captcha.qcloud.com/TJNCaptcha-global.js'
}

export function normalizeTencentCaptchaRegion(value?: string | null): TencentCaptchaRegion {
  return value === 'intl' ? 'intl' : 'cn'
}

let scriptPromise: Promise<TencentCaptchaConstructor> | null = null
let loadedRegion: TencentCaptchaRegion | null = null

// existingGlobalRegion 推断页面上已存在的全局构造函数属于哪个站点。
// 两个站点的构造函数签名不兼容（国际站首参必须是 DOM 元素），错配会直接抛错，
// 因此复用已存在的全局前必须先确认站点一致。
function existingGlobalRegion(): TencentCaptchaRegion {
  return window.TCaptchaGlobal === true ? 'intl' : 'cn'
}

export function loadTencentCaptcha(
  region: TencentCaptchaRegion = 'cn'
): Promise<TencentCaptchaConstructor> {
  // 两个站点的 SDK 注册同名全局 TencentCaptcha，缓存必须按站点隔离，
  // 否则管理员切换站点后仍会复用上一个站点的构造函数。
  // loadedRegion 为 null 表示全局不是本模块注入的（如开发期 HMR 后模块状态被重置），
  // 此时靠 TCaptchaGlobal 判定站点，一致才复用。
  const globalRegion = window.TencentCaptcha ? existingGlobalRegion() : null
  if (window.TencentCaptcha && (loadedRegion === region || globalRegion === region)) {
    return Promise.resolve(window.TencentCaptcha)
  }
  if (window.TencentCaptcha && loadedRegion !== null && loadedRegion !== region) {
    // 两个站点共享 TencentCaptcha 全局且构造签名不兼容，不能在同一页面注入第二份 SDK。
    // 管理员切换区域后由部署流程刷新页面，刷新前直接失败可避免全局构造函数被污染。
    return Promise.reject(new Error('Tencent Captcha region changed; reload the page to apply it'))
  }
  if (scriptPromise && loadedRegion === region) return scriptPromise

  loadedRegion = region
  scriptPromise = new Promise((resolve, reject) => {
    const script = document.createElement('script')
    script.src = SCRIPT_SRC[region]
    script.async = true
    script.onload = () => {
      // 入口脚本被重复引入时腾讯会在求值阶段抛错（"请勿多次引用腾讯验证码的接入js"），
      // 此时 onload 仍会触发而全局仍是上一个站点的构造函数。校验站点，避免把
      // 签名不兼容的构造函数当成本站点的返回出去。
      if (window.TencentCaptcha && existingGlobalRegion() === region) {
        resolve(window.TencentCaptcha)
        return
      }
      scriptPromise = null
      loadedRegion = null
      reject(new Error('Tencent Captcha SDK is unavailable'))
    }
    script.onerror = () => {
      scriptPromise = null
      loadedRegion = null
      reject(new Error('Failed to load Tencent Captcha SDK'))
    }
    document.head.appendChild(script)
  })

  return scriptPromise
}

export function resetTencentCaptchaLoaderForTest(): void {
  scriptPromise = null
  loadedRegion = null
}
