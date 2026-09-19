export type BestloongNavIcon =
  | 'dashboard'
  | 'key'
  | 'usage'
  | 'modelPlaza'
  | 'channelStatus'
  | 'redeem'
  | 'balance'
  | 'tutorials'
  | 'about'
  | 'profile'

export interface BestloongNavItem {
  readonly path: string
  readonly labelKey: string
  readonly icon: BestloongNavIcon
}

// Customer navigation is a product contract. Keep the order aligned with R1.
export const BESTLOONG_USER_NAV_ITEMS: readonly BestloongNavItem[] = Object.freeze([
  { path: '/dashboard', labelKey: 'nav.dashboard', icon: 'dashboard' },
  { path: '/keys', labelKey: 'nav.apiKeys', icon: 'key' },
  { path: '/usage', labelKey: 'nav.usage', icon: 'usage' },
  { path: '/model-plaza', labelKey: 'nav.modelPlaza', icon: 'modelPlaza' },
  { path: '/monitor', labelKey: 'nav.channelStatus', icon: 'channelStatus' },
  { path: '/redeem', labelKey: 'nav.redeem', icon: 'redeem' },
  { path: '/balance', labelKey: 'nav.myBalance', icon: 'balance' },
  { path: '/tutorials', labelKey: 'nav.tutorials', icon: 'tutorials' },
  { path: '/about', labelKey: 'nav.aboutUs', icon: 'about' },
  { path: '/profile', labelKey: 'nav.profile', icon: 'profile' },
])

const HIDDEN_CUSTOMER_PATH_PREFIXES = Object.freeze([
  '/affiliate',
  '/available-channels',
  '/batch-image',
  '/custom',
  '/orders',
  '/payment',
  '/purchase',
  '/subscriptions',
])

export function isBestloongHiddenCustomerPath(path: string): boolean {
  return HIDDEN_CUSTOMER_PATH_PREFIXES.some(
    (prefix) => path === prefix || path.startsWith(`${prefix}/`),
  )
}
