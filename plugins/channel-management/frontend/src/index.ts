/**
 * Channel Management plugin — frontend entry point.
 *
 * The host app is expected to:
 *   1. Call setClient() with its axios instance during plugin install
 *   2. Merge i18n messages from ./i18n/{en,zh} into its global locales
 *   3. Mount the route at /admin/channels with ChannelsView as the component
 *
 * Phase 2: file structure only. The host still imports these via the source
 * tree; the Vite library build is wired in a later phase.
 */

export { default as ChannelsView } from './views/ChannelsView.vue'

export { setClient } from './api/client'
export { default as channelsAPI } from './api/channels'
export type {
  BillingMode,
  PricingInterval,
  ChannelModelPricing,
  Channel,
  CreateChannelRequest,
  UpdateChannelRequest,
  ModelDefaultPricing,
} from './api/channels'

export { default as enMessages } from './i18n/en'
export { default as zhMessages } from './i18n/zh'

/**
 * Route definition the host should register. Component is left as the imported
 * Vue component reference so the host can plug it into its own router.
 */
export const channelRoute = {
  path: '/admin/channels',
  name: 'AdminChannels',
  meta: {
    requiresAuth: true,
    requiresAdmin: true,
    title: 'Channel Management',
    titleKey: 'admin.channels.title',
    descriptionKey: 'admin.channels.description',
  },
}
