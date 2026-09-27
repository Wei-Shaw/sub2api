import landing from './landing'
import common from './common'
import dashboard from './dashboard'
import channelMonitorV2 from './channelMonitorV2'
import batchImage from './batchImage'
import imageStudio from './imageStudio'
import playground from './playground'
import admin from './admin'
import misc from './misc'

// Everything except the `admin` namespace (~72% of the bundle). The app loads this first and
// merges `./admin` lazily (see i18n/index.ts); the default export stays the full bundle.
export const baseMessages = {
  ...landing,
  ...common,
  ...dashboard,
  ...channelMonitorV2,
  ...batchImage,
  ...imageStudio,
  ...playground,
  ...misc,
}

export default { ...baseMessages, admin }
