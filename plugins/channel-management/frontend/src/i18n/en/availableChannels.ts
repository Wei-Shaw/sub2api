/**
 * English i18n — `availableChannels.*` namespace (V5 W9 user-facing list).
 *
 * T19 Fix B — split out from monolithic `en.ts`.
 */
export default {
  title: 'Available Channels',
  description: 'Channels you can access, along with their supported models and pricing',
  searchPlaceholder: 'Search channels or models...',
  empty: 'No available channels',
  noModels: 'No models configured',
  noPricing: 'Pricing not configured',
  exclusive: 'Exclusive',
  public: 'Public',
  exclusiveTooltip: 'Exclusive groups granted to you by an admin',
  publicTooltip: 'Groups open to all users',
  columns: {
    name: 'Channel',
    description: 'Description',
    platform: 'Platform',
    groups: 'Your Accessible Groups',
    supportedModels: 'Supported Models',
  },
  pricing: {
    billingMode: 'Billing Mode',
    billingModeToken: 'Per Token',
    billingModePerRequest: 'Per Request',
    billingModeImage: 'Per Image',
    inputPrice: 'Input',
    outputPrice: 'Output',
    cacheWritePrice: 'Cache Write',
    cacheReadPrice: 'Cache Read',
    imageOutputPrice: 'Image Output',
    perRequestPrice: 'Per Request',
    intervals: 'Tiered Pricing',
    unitPerMillion: '/ 1M tokens',
    unitPerRequest: '/ request',
  },
}
