import { i18n } from '@/i18n'
import { resolveSiteName } from '@/constants/site'
import type { RouteLocationNormalizedLoaded } from 'vue-router'
import type { CustomMenuItem } from '@/types'
import type { SiteBillingMode } from '@/utils/siteBillingMode'

/**
 * 统一生成页面标题，避免多处写入 document.title 产生覆盖冲突。
 * 优先使用 titleKey 通过 i18n 翻译，fallback 到静态 routeTitle。
 */
export function resolveDocumentTitle(routeTitle: unknown, siteName?: string, titleKey?: string): string {
  const normalizedSiteName = resolveSiteName(siteName)

  if (typeof titleKey === 'string' && titleKey.trim()) {
    const translated = i18n.global.t(titleKey)
    if (translated && translated !== titleKey) {
      return `${translated} - ${normalizedSiteName}`
    }
  }

  if (typeof routeTitle === 'string' && routeTitle.trim()) {
    return `${routeTitle.trim()} - ${normalizedSiteName}`
  }

  return normalizedSiteName
}

export interface RouteTitleOptions {
  /**
   * 站点计费模式（见 utils/billingMode.ts）。/purchase 的标题与描述随之切换：
   * 仅充值 → 「充值」，仅订阅 → 「订阅」，缺省/两者都有 → 「充值/订阅」。
   */
  billingMode?: SiteBillingMode
}

export interface RouteMetaKeys {
  titleKey?: string
  descriptionKey?: string
}

/** /purchase 路由名，与 router/index.ts 中的声明保持一致。 */
export const PURCHASE_ROUTE_NAME = 'PurchaseSubscription'

/**
 * 解析路由的 i18n 标题/描述 key。绝大多数路由直接取 meta；
 * /purchase 随站点计费模式切换文案，AppHeader 与 document.title 共用这一处判断。
 */
export function resolveRouteMetaKeys(
  route: Pick<RouteLocationNormalizedLoaded, 'name' | 'meta'>,
  options: RouteTitleOptions = {},
): RouteMetaKeys {
  if (route.name === PURCHASE_ROUTE_NAME) {
    if (options.billingMode === 'recharge_only') {
      return { titleKey: 'nav.recharge', descriptionKey: 'purchase.rechargeDescription' }
    }
    if (options.billingMode === 'subscription_only') {
      return { titleKey: 'nav.subscribe', descriptionKey: 'purchase.subscriptionDescription' }
    }
  }
  return {
    titleKey: typeof route.meta.titleKey === 'string' ? route.meta.titleKey : undefined,
    descriptionKey: typeof route.meta.descriptionKey === 'string' ? route.meta.descriptionKey : undefined,
  }
}

export function resolveRouteDocumentTitle(
  route: Pick<RouteLocationNormalizedLoaded, 'name' | 'params' | 'meta'>,
  siteName: string | undefined,
  customMenuItems: CustomMenuItem[] = [],
  options: RouteTitleOptions = {},
): string {
  const id = typeof route.params.id === 'string' ? route.params.id : ''
  const menuItem = route.name === 'CustomPage' && id
    ? customMenuItems.find((item) => item.id === id)
    : undefined
  const menuTitle = menuItem?.label.trim()
  const { titleKey } = resolveRouteMetaKeys(route, options)

  return resolveDocumentTitle(menuTitle || route.meta.title, siteName, menuTitle ? undefined : titleKey)
}
