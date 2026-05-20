const WECOM_JSSDK_URL = 'https://wwcdn.weixin.qq.com/node/open/js/wecom-jssdk-2.3.4.js'
export type WeComLoginRedirectType = 'callback' | 'top' | 'self'
export type WeComLoginPanelSize = 'middle' | 'small'

export interface WeComLoginInstance {
  unmount(): void
}

export interface WeComLoginPanelParams {
  login_type: 'CorpApp'
  appid: string
  agentid: string
  redirect_uri: string
  state: string
  redirect_type: WeComLoginRedirectType
  panel_size: WeComLoginPanelSize
  lang: 'zh'
}

export interface WeComLoginPanelOptions {
  el: Element
  params: WeComLoginPanelParams
  onCheckWeComLogin?(event: { isWeComLogin: boolean }): void
  onOpenInWecom?(): void
  onLoginSuccess(res: { code: string }): void
  onLoginFail(error: { errMsg?: string }): void
}

export interface WeComSDK {
  createWWLoginPanel(options: WeComLoginPanelOptions): WeComLoginInstance
}

declare global {
  interface Window {
    ww?: WeComSDK
  }
}

let sdkLoading: Promise<WeComSDK> | null = null

export async function loadWeComSDK(): Promise<WeComSDK> {
  if (window.ww) return window.ww
  if (sdkLoading) return sdkLoading
  sdkLoading = appendWeComSDKScript()
  try {
    return await sdkLoading
  } catch (error) {
    sdkLoading = null
    throw error
  }
}

export function buildWeComLoginPanelParams(
  rawURL: string,
  redirectType: WeComLoginRedirectType,
  panelSize: WeComLoginPanelSize
): WeComLoginPanelParams {
  const params = new URL(rawURL).searchParams
  return {
    login_type: 'CorpApp',
    appid: params.get('appid') || '',
    agentid: params.get('agentid') || '',
    redirect_uri: params.get('redirect_uri') || '',
    state: params.get('state') || '',
    redirect_type: redirectType,
    panel_size: panelSize,
    lang: 'zh'
  }
}

function appendWeComSDKScript(): Promise<WeComSDK> {
  return new Promise((resolve, reject) => {
    const script = document.createElement('script')
    script.src = WECOM_JSSDK_URL
    script.async = true
    script.onload = () => {
      if (window.ww) {
        resolve(window.ww)
        return
      }
      reject(new Error('企业微信登录组件加载失败'))
    }
    script.onerror = () => reject(new Error('企业微信登录组件加载失败'))
    document.head.appendChild(script)
  })
}
