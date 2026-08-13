import type { NavIconName } from '@/components/icons/nav'
import type { CustomMenuItem } from '@/types'

/**
 * The sidebar nav tree, as data.
 *
 * This used to live inside AppSidebar.vue among ~500 lines of inline icon
 * render functions, which made two things impossible: unit-testing the
 * feature-flag and simple-mode filtering, and asserting that onboarding tour
 * anchors exist. Both are now properties of plain objects.
 *
 * Nothing here imports a component. `icon` is a NAME resolved by
 * `components/icons/nav/index.ts` at render time, so this module has no DOM
 * dependency and a nav item stays comparable and serialisable.
 */

/**
 * Feature-flag getter. Returning `undefined` means "settings have not loaded
 * yet" and MUST be treated as visible — see `applyFeatureFlags`.
 */
export type NavFlag = () => boolean | undefined

export interface NavItem {
  path: string
  label: string
  /** Name in the nav icon registry, or `null` when `iconSvg` supplies markup. */
  icon: NavIconName | null
  /** Raw SVG for admin-configured custom menu entries. Sanitised at render. */
  iconSvg?: string
  hideInSimpleMode?: boolean
  children?: NavItem[]
  /**
   * When true the parent only toggles expand/collapse and does NOT navigate to
   * its `path`; the path is purely a stable key. All four groups set it today.
   * `/admin/orders` is the one to watch: its path is both a group key and a
   * real route that one of its own children points at, so clearing this flag
   * quietly turns the parent row into a second way to reach that page.
   */
  expandOnly?: boolean
  featureFlag?: NavFlag
  /** `id` attribute, when an onboarding step targets this item by id. */
  domId?: string
  /** `data-tour` attribute, when a step targets it by attribute. */
  tourAnchor?: string
}

export interface NavFlags {
  channelMonitor: NavFlag
  payment: NavFlag
  availableChannels: NavFlag
  affiliate: NavFlag
  riskControl: NavFlag
  opsMonitoring: NavFlag
  adminPayment: NavFlag
  batchImageAccess: NavFlag
}

export interface NavContext {
  /** i18n lookup. Labels resolve eagerly so the tree stays a plain value. */
  t: (key: string) => string
  flags: NavFlags
  /** Public-settings custom entries; source for `visibility: 'user'`. */
  userCustomMenuItems: CustomMenuItem[]
  /** Admin-settings custom entries; source for `visibility: 'admin'`. */
  adminCustomMenuItems: CustomMenuItem[]
  simpleMode: boolean
}

/**
 * Drop items whose flag returns exactly `false`, recursing into children.
 *
 * The `!== false` comparison is load-bearing: `undefined` means public settings
 * have not arrived yet, and hiding on `undefined` makes the whole menu flash
 * empty on every cold load. Do not reduce this to a truthiness check.
 */
export function applyFeatureFlags(items: NavItem[]): NavItem[] {
  const out: NavItem[] = []
  for (const item of items) {
    if (item.featureFlag && item.featureFlag() === false) continue
    if (item.children) {
      out.push({ ...item, children: applyFeatureFlags(item.children) })
    } else {
      out.push(item)
    }
  }
  return out
}

/** Feature-flag filtering plus the simple-mode cut, in that order. */
export function finalizeNav(items: NavItem[], simpleMode: boolean): NavItem[] {
  const visible = applyFeatureFlags(items)
  return simpleMode ? visible.filter((item) => !item.hideInSimpleMode) : visible
}

function customEntries(items: CustomMenuItem[], visibility: 'user' | 'admin'): NavItem[] {
  return items
    .filter((item) => item.visibility === visibility)
    .sort((a, b) => a.sort_order - b.sort_order)
    .map((item) => ({
      path: `/custom/${item.id}`,
      label: item.label,
      icon: null,
      iconSvg: item.icon_svg,
    }))
}

/**
 * The personal navigation. Shared by the regular-user menu and by the admin
 * "My Account" section, which is why the dashboard entry is a parameter — the
 * admin section sits under an admin dashboard already.
 *
 * Order: keys, usage, available channels, channel status, subscription and
 * payment, redeem, profile. Available Channels sits directly above Channel
 * Status so the reading order is "what can I use" then "how is it doing".
 */
export function buildSelfNavItems(ctx: NavContext, withDashboard: boolean): NavItem[] {
  const { t, flags } = ctx
  const items: NavItem[] = []

  if (withDashboard) {
    items.push({ path: '/dashboard', label: t('nav.dashboard'), icon: 'dashboard' })
  }

  items.push(
    { path: '/keys', label: t('nav.apiKeys'), icon: 'key', tourAnchor: 'sidebar-my-keys' },
    { path: '/batch-image', label: t('nav.batchImage'), icon: 'batchImage', hideInSimpleMode: true, featureFlag: flags.batchImageAccess },
    { path: '/usage', label: t('nav.usage'), icon: 'chart', hideInSimpleMode: true },
    { path: '/available-channels', label: t('nav.availableChannels'), icon: 'channel', hideInSimpleMode: true, featureFlag: flags.availableChannels },
    { path: '/monitor', label: t('nav.channelStatus'), icon: 'signal', featureFlag: flags.channelMonitor },
    { path: '/subscriptions', label: t('nav.mySubscriptions'), icon: 'creditCard', hideInSimpleMode: true },
    { path: '/purchase', label: t('nav.buySubscription'), icon: 'recharge', hideInSimpleMode: true, featureFlag: flags.payment },
    { path: '/orders', label: t('nav.myOrders'), icon: 'orderList', hideInSimpleMode: true, featureFlag: flags.payment },
    { path: '/redeem', label: t('nav.redeem'), icon: 'gift', hideInSimpleMode: true },
    { path: '/affiliate', label: t('nav.affiliate'), icon: 'users', hideInSimpleMode: true, featureFlag: flags.affiliate },
    { path: '/profile', label: t('nav.profile'), icon: 'user' },
    ...customEntries(ctx.userCustomMenuItems, 'user'),
  )

  return items
}

/**
 * The admin navigation, already filtered.
 *
 * Simple mode is not only a filter: it also appends an API-keys entry the
 * standard menu does not have, because the personal section that normally
 * carries it is hidden there. That entry deliberately has no tour anchor — the
 * onboarding tour does not run in simple mode.
 */
export function buildAdminNavItems(ctx: NavContext): NavItem[] {
  const { t, flags } = ctx

  const baseItems: NavItem[] = [
    { path: '/admin/dashboard', label: t('nav.dashboard'), icon: 'dashboard' },
    { path: '/admin/ops', label: t('nav.ops'), icon: 'chart', featureFlag: flags.opsMonitoring },
    { path: '/admin/users', label: t('nav.users'), icon: 'users', hideInSimpleMode: true },
    { path: '/admin/groups', label: t('nav.groups'), icon: 'folder', hideInSimpleMode: true, domId: 'sidebar-group-manage' },
    {
      path: '/admin/channels',
      label: t('nav.channelManagement'),
      icon: 'channel',
      hideInSimpleMode: true,
      expandOnly: true,
      children: [
        { path: '/admin/channels/pricing', label: t('nav.channelPricing'), icon: 'priceTag' },
        { path: '/admin/channels/monitor', label: t('nav.channelMonitor'), icon: 'signal', featureFlag: flags.channelMonitor },
      ],
    },
    { path: '/admin/subscriptions', label: t('nav.subscriptions'), icon: 'creditCard', hideInSimpleMode: true },
    { path: '/admin/accounts', label: t('nav.accounts'), icon: 'globe', domId: 'sidebar-channel-manage' },
    { path: '/admin/announcements', label: t('nav.announcements'), icon: 'bell' },
    { path: '/admin/proxies', label: t('nav.proxies'), icon: 'server' },
    {
      path: '/admin/security-audit',
      label: t('nav.securityAudit'),
      icon: 'shield',
      expandOnly: true,
      featureFlag: flags.riskControl,
      children: [
        { path: '/admin/risk-control', label: t('nav.contentModeration'), icon: 'shield' },
        { path: '/admin/prompt-audit', label: t('nav.promptAudit'), icon: 'shield' },
      ],
    },
    { path: '/admin/redeem', label: t('nav.redeemCodes'), icon: 'ticket', hideInSimpleMode: true, domId: 'sidebar-wallet' },
    { path: '/admin/promo-codes', label: t('nav.promoCodes'), icon: 'gift', hideInSimpleMode: true },
    {
      path: '/admin/affiliates',
      label: t('nav.affiliateManagement'),
      icon: 'users',
      hideInSimpleMode: true,
      expandOnly: true,
      featureFlag: flags.affiliate,
      children: [
        { path: '/admin/affiliates/invites', label: t('nav.affiliateInviteRecords'), icon: 'users' },
        { path: '/admin/affiliates/rebates', label: t('nav.affiliateRebateRecords'), icon: 'order' },
        { path: '/admin/affiliates/transfers', label: t('nav.affiliateTransferRecords'), icon: 'creditCard' },
      ],
    },
    {
      path: '/admin/orders',
      label: t('nav.orderManagement'),
      icon: 'order',
      hideInSimpleMode: true,
      expandOnly: true,
      featureFlag: flags.adminPayment,
      children: [
        { path: '/admin/orders/dashboard', label: t('nav.paymentDashboard'), icon: 'chart' },
        { path: '/admin/orders', label: t('nav.orderManagement'), icon: 'order' },
        { path: '/admin/orders/plans', label: t('nav.paymentPlans'), icon: 'creditCard' },
      ],
    },
    { path: '/admin/usage', label: t('nav.usage'), icon: 'chart' },
    { path: '/admin/audit-logs', label: t('nav.auditLogs'), icon: 'shield', hideInSimpleMode: true },
  ]

  const visible = applyFeatureFlags(baseItems)
  const tail: NavItem[] = [
    { path: '/admin/settings', label: t('nav.settings'), icon: 'cog' },
    ...customEntries(ctx.adminCustomMenuItems, 'admin'),
  ]

  if (ctx.simpleMode) {
    return [
      ...visible.filter((item) => !item.hideInSimpleMode),
      { path: '/keys', label: t('nav.apiKeys'), icon: 'key' },
      ...tail,
    ]
  }

  return [...visible, ...tail]
}

/**
 * The selector an onboarding step would use to reach this item, or null.
 *
 * `useOnboardingTour` advances when the user clicks the element the current
 * step points at, so the sidebar has to ask "was the item just clicked the
 * current target". That mapping used to be a hardcoded three-entry object in
 * the click handler, kept in sync by hand with three ternaries in the template.
 */
export function tourSelectorFor(item: NavItem): string | null {
  if (item.domId) return `#${item.domId}`
  if (item.tourAnchor) return `[data-tour="${item.tourAnchor}"]`
  return null
}
