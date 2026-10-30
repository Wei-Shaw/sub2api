import type { PublicSettings } from '@/types'
import { FeatureFlags, resolveFeatureFlag } from '@/utils/featureFlags'

/**
 * 站点计费模式（后台「站点类型」单选），由两个后端开关派生：
 * - `subscription_enabled`（opt-out，缺省视为开启）：用户端订阅面（侧边栏、购买页订阅 tab、顶栏徽章、/subscriptions）
 * - `payment_balance_disabled`（strict true）：支付配置里的 BALANCE_PAYMENT_DISABLED，关闭余额充值下单
 *
 * 两者都关闭是接口层可表达但 UI 不提供的组合，这里归为「仅充值」；后台选择器保存时会把余额充值重新打开。
 */
export type SiteBillingMode = 'recharge_and_subscription' | 'recharge_only' | 'subscription_only'

export const SITE_BILLING_MODES: readonly SiteBillingMode[] = [
  'recharge_and_subscription',
  'recharge_only',
  'subscription_only',
]

/** i18n 子键（admin.settings.features.siteBillingMode.options / hints）。 */
export const SITE_BILLING_MODE_I18N_KEYS: Record<SiteBillingMode, string> = {
  recharge_and_subscription: 'rechargeAndSubscription',
  recharge_only: 'rechargeOnly',
  subscription_only: 'subscriptionOnly',
}

export interface BillingModeSettings {
  subscription_enabled?: boolean
  payment_balance_disabled?: boolean
}

export function resolveSiteBillingMode(settings: BillingModeSettings | null | undefined): SiteBillingMode {
  const subscriptionEnabled = resolveFeatureFlag(
    { subscription_enabled: settings?.subscription_enabled } as Partial<PublicSettings>,
    FeatureFlags.subscription,
  )
  if (!subscriptionEnabled) return 'recharge_only'
  if (settings?.payment_balance_disabled === true) return 'subscription_only'
  return 'recharge_and_subscription'
}

export function billingModeToSettings(mode: SiteBillingMode): Required<BillingModeSettings> {
  switch (mode) {
    case 'recharge_only':
      return { subscription_enabled: false, payment_balance_disabled: false }
    case 'subscription_only':
      return { subscription_enabled: true, payment_balance_disabled: true }
    default:
      return { subscription_enabled: true, payment_balance_disabled: false }
  }
}
