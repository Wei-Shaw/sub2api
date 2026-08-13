import landing from './landing'
import common from './common'
import dashboard from './dashboard'
import channelMonitorV2 from './channelMonitorV2'
import batchImage from './batchImage'
import admin from './admin'
import misc from './misc'
import custom from './custom'
import support from './support'
import inbox from './inbox'
import organization from './organization'
import videoModels from './videoModels'
import materials from './materials'
import { mergeLocaleMessages } from '../merge'

const upstream = {
  ...landing,
  ...common,
  ...dashboard,
  ...channelMonitorV2,
  ...batchImage,
  admin,
  ...misc,
  ...inbox,
  ...videoModels,
  ...materials,
}

export default mergeLocaleMessages(mergeLocaleMessages(mergeLocaleMessages(upstream, custom), support), organization)
