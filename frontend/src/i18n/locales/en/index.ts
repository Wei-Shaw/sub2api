import { adminLocalePatches, mergeLocalePatch } from '../adminLocalePatches'
import landing from './landing'
import common from './common'
import dashboard from './dashboard'
import batchImage from './batchImage'
import admin from './admin'
import misc from './misc'

const messages = {
  ...landing,
  ...common,
  ...dashboard,
  ...batchImage,
  admin,
  ...misc,
}

export default mergeLocalePatch(messages, adminLocalePatches.en)
