export const VIDEO_GATEWAY_DEMO_MODE = 'video_gateway_demo'
export const PUBLIC_AUTH_BRAND_NAME = '无界互娱'
export const PUBLIC_AUTH_PRODUCT_NAME = 'AI 生产控制台'
export const PUBLIC_AUTH_PRODUCT_SUBTITLE = '中剧 / 短剧生产网关'
export const PUBLIC_AUTH_NETWORK_LABEL = '内部使用'
export const PUBLIC_AUTH_SAFE_DEMO_LABEL = '试跑任务'
export const PUBLIC_AUTH_PRODUCTION_STATUS = '正式上线未开启'
export const PUBLIC_AUTH_COMMERCIAL_STATUS = '商业化未开启'
export const PUBLIC_AUTH_REAL_PROVIDER_STATUS = '真实生成通道未接入'
export const VIDEO_GATEWAY_PRODUCT_NAME = PUBLIC_AUTH_PRODUCT_NAME
export const VIDEO_GATEWAY_ADMIN_NAME = '无界管理台'
export const VIDEO_GATEWAY_HOME_PATH = '/admin/video/dashboard'

export const isVideoGatewayDemoMode = import.meta.env.VITE_PRODUCT_MODE !== 'standard'

const demoRoutePrefixes = [
  '/internal-pilot',
  '/admin/video',
  '/admin/console',
  '/admin/generation-content',
  '/keys',
  '/login',
  '/setup',
  '/auth',
  '/payment/result',
]

export function isVideoGatewayDemoRoute(path: string): boolean {
  return path === '/' || path === '/admin' || demoRoutePrefixes.some((prefix) => path === prefix || path.startsWith(`${prefix}/`))
}

export function videoGatewayDisplayProvider(provider: string, displayName?: string): string {
  if (isVideoGatewayDemoMode) {
    if (provider === 'mock') return '试跑任务'
    if (provider === 'seedance') return 'Seedance 2.0'
    if (provider === 'kling') return 'Kling'
    return provider || '生成通道'
  }

  if (displayName?.trim()) return displayName.trim()
  if (provider === 'seedance') return 'Seedance 2.0'
  if (provider === 'kling') return 'Kling'
  return provider || '生成通道'
}
