/**
 * Chinese i18n aggregator for the channel management plugin.
 *
 * T19 Fix B — content split into `./zh/*.ts` per namespace; this barrel
 * preserves the legacy single-default-export shape so existing host
 * importers (`import zhMessages from './i18n/zh'`) keep working unchanged.
 *
 * Namespace ownership: see `./en.ts` for the canonical description.
 */
import common from './zh/common'
import channels from './zh/channels'
import channelMonitor from './zh/channelMonitor'
import monitorCommon from './zh/monitorCommon'
import channelStatus from './zh/channelStatus'
import availableChannels from './zh/availableChannels'

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
