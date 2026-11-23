import adminAccessKeys from './adminAccessKeys'
import mcp from './mcp'
import landing from './landing'
import common from './common'
import dashboard from './dashboard'
import channelMonitorV2 from './channelMonitorV2'
import batchImage from './batchImage'
import imageStudio from './imageStudio'
import quantModels from './quantModels'
import aboutAlphaio from './aboutAlphaio'
import pelicanBenchmark from './pelicanBenchmark'
import upstreamPriceMonitor from './upstreamPriceMonitor'
import admin from './admin'
import misc from './misc'

export default {
  adminAccessKeys,
  mcp,
  ...landing,
  ...common,
  ...dashboard,
  ...channelMonitorV2,
  ...batchImage,
  ...imageStudio,
  ...quantModels,
  ...aboutAlphaio,
  ...pelicanBenchmark,
  ...upstreamPriceMonitor,
  admin,
  ...misc,
}
