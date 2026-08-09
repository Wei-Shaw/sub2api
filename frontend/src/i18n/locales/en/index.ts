import landing from './landing'
import common from './common'
import dashboard from './dashboard'
import batchImage from './batchImage'
import admin from './admin'
import misc from './misc'
import custom from './custom'
import support from './support'
import inbox from './inbox'
import organization from './organization'
import videoModels from './videoModels'
import { mergeLocaleMessages } from '../merge'

const upstream = {
  ...landing,
  ...common,
  ...dashboard,
  ...batchImage,
  admin,
  ...misc,
  ...inbox,
  ...videoModels,
}

export default mergeLocaleMessages(mergeLocaleMessages(mergeLocaleMessages(upstream, custom), support), organization)
