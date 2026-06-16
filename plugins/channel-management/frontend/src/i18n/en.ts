/**
 * English i18n aggregator for the channel management plugin.
 *
 * T19 Fix B — content split into `./en/*.ts` per namespace; this barrel
 * preserves the legacy single-default-export shape so existing host
 * importers (`import enMessages from './i18n/en'`) keep working unchanged.
 *
 * Namespace ownership:
 *   - common.ts            → nav.* + common.*
 *   - channels.ts          → admin.channels.*
 *   - channelMonitor.ts    → admin.channelMonitor.*
 *   - monitorCommon.ts     → monitorCommon.* (shared admin + user)
 *   - channelStatus.ts     → channelStatus.* (user-facing)
 *   - availableChannels.ts → availableChannels.* (user-facing)
 */
import common from './en/common'
import channels from './en/channels'
import channelMonitor from './en/channelMonitor'
import monitorCommon from './en/monitorCommon'
import channelStatus from './en/channelStatus'
import availableChannels from './en/availableChannels'

export default {
  ...common,
  admin: {
    channels,
    channelMonitor,
  },
  monitorCommon,
  channelStatus,
  availableChannels,
}
