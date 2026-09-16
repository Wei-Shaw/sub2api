export const ALIPAY_DEEP_LINK_FALLBACK_DELAY_MS = 2200
export const ALIPAY_EMBEDDED_BROWSER_FALLBACK_DELAY_MS = 300

export type AlipayDeepLinkState = 'idle' | 'launching' | 'backgrounded' | 'fallback'

const ALIPAY_DEEP_LINK_PREFIX = 'alipays://platformapi/startapp?saId=10000007&qrcode='

export function buildAlipayDeepLink(qrCode: string): string {
  const dynamicQRCode = qrCode.trim()
  if (!dynamicQRCode) return ''
  return `${ALIPAY_DEEP_LINK_PREFIX}${encodeURIComponent(dynamicQRCode)}`
}

export function isAlipaySchemeRestrictedBrowser(userAgent: string): boolean {
  return /MicroMessenger|MQQBrowser|\bQQ\//i.test(userAgent)
}

interface EventTargetLike {
  addEventListener(type: string, listener: EventListener): void
  removeEventListener(type: string, listener: EventListener): void
}

interface VisibilityDocumentLike extends EventTargetLike {
  readonly hidden: boolean
}

export interface AlipayDeepLinkLauncherOptions {
  qrCode: string
  document: VisibilityDocumentLike
  lifecycleTarget: EventTargetLike
  userAgent: string
  assignLocation: (url: string) => void
  onStateChange: (state: AlipayDeepLinkState) => void
  setTimer?: typeof setTimeout
  clearTimer?: typeof clearTimeout
}

export interface AlipayDeepLinkLauncher {
  launch(): void
  dispose(): void
}

export function createAlipayDeepLinkLauncher(options: AlipayDeepLinkLauncherOptions): AlipayDeepLinkLauncher {
  const setTimer = options.setTimer ?? setTimeout
  const clearTimer = options.clearTimer ?? clearTimeout
  let timer: ReturnType<typeof setTimeout> | null = null
  let disposed = false

  const setState = (state: AlipayDeepLinkState) => {
    if (!disposed) options.onStateChange(state)
  }
  const clearFallbackTimer = () => {
    if (timer) {
      clearTimer(timer)
      timer = null
    }
  }
  const markBackgrounded = () => {
    clearFallbackTimer()
    setState('backgrounded')
  }
  const handleVisibilityChange: EventListener = () => {
    if (options.document.hidden) markBackgrounded()
  }
  const handlePageHide: EventListener = () => markBackgrounded()

  options.document.addEventListener('visibilitychange', handleVisibilityChange)
  options.lifecycleTarget.addEventListener('pagehide', handlePageHide)

  return {
    launch() {
      if (disposed) return
      clearFallbackTimer()
      const deepLink = buildAlipayDeepLink(options.qrCode)
      if (!deepLink) {
        setState('fallback')
        return
      }

      setState('launching')
      try {
        options.assignLocation(deepLink)
      } catch {
        setState('fallback')
        return
      }

      const delay = isAlipaySchemeRestrictedBrowser(options.userAgent)
        ? ALIPAY_EMBEDDED_BROWSER_FALLBACK_DELAY_MS
        : ALIPAY_DEEP_LINK_FALLBACK_DELAY_MS
      timer = setTimer(() => {
        timer = null
        if (options.document.hidden) {
          setState('backgrounded')
          return
        }
        setState('fallback')
      }, delay)
    },
    dispose() {
      clearFallbackTimer()
      options.document.removeEventListener('visibilitychange', handleVisibilityChange)
      options.lifecycleTarget.removeEventListener('pagehide', handlePageHide)
      disposed = true
    },
  }
}
