import BellIcon from './BellIcon.vue'
import BatchImageIcon from './BatchImageIcon.vue'
import ChannelIcon from './ChannelIcon.vue'
import ChartIcon from './ChartIcon.vue'
import CogIcon from './CogIcon.vue'
import CreditCardIcon from './CreditCardIcon.vue'
import DashboardIcon from './DashboardIcon.vue'
import FolderIcon from './FolderIcon.vue'
import GiftIcon from './GiftIcon.vue'
import GlobeIcon from './GlobeIcon.vue'
import KeyIcon from './KeyIcon.vue'
import OrderIcon from './OrderIcon.vue'
import OrderListIcon from './OrderListIcon.vue'
import PriceTagIcon from './PriceTagIcon.vue'
import RechargeIcon from './RechargeIcon.vue'
import ServerIcon from './ServerIcon.vue'
import ShieldIcon from './ShieldIcon.vue'
import SignalIcon from './SignalIcon.vue'
import TicketIcon from './TicketIcon.vue'
import UserIcon from './UserIcon.vue'
import UsersIcon from './UsersIcon.vue'

/**
 * Name -> component for everything the sidebar can render.
 *
 * `navTree.ts` stores the NAME, not the component, so the nav tree stays plain
 * data: it can be unit-tested without a DOM, and a nav item is comparable and
 * serialisable. This map is the only place that turns a name into markup.
 */
export const navIcons = {
  batchImage: BatchImageIcon,
  bell: BellIcon,
  channel: ChannelIcon,
  chart: ChartIcon,
  cog: CogIcon,
  creditCard: CreditCardIcon,
  dashboard: DashboardIcon,
  folder: FolderIcon,
  gift: GiftIcon,
  globe: GlobeIcon,
  key: KeyIcon,
  order: OrderIcon,
  orderList: OrderListIcon,
  priceTag: PriceTagIcon,
  recharge: RechargeIcon,
  server: ServerIcon,
  shield: ShieldIcon,
  signal: SignalIcon,
  ticket: TicketIcon,
  user: UserIcon,
  users: UsersIcon,
} as const

export type NavIconName = keyof typeof navIcons

export { default as NavIconBase } from './NavIconBase.vue'
