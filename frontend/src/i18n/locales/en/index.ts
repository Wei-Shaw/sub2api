import landing from './landing'
import common from './common'
import dashboard from './dashboard'
import admin from './admin'
import misc from './misc'
import custom from './custom'
import support from './support'
import { mergeLocaleMessages } from '../merge'

const upstream = {
  ...landing,
  ...common,
  ...dashboard,
  admin,
  ...misc,
}

export default mergeLocaleMessages(mergeLocaleMessages(upstream, custom), support)
