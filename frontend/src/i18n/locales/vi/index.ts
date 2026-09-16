import landing from './landing'
import common from './common'
import dashboard from './dashboard'
import channelMonitorV2 from './channelMonitorV2'
import batchImage from './batchImage'
import admin from './admin'
import misc from './misc'
import { mergeLocaleMessages } from './merge'
import landingCompletion from './landing-completion'
import commonCompletion from './common-completion'
import dashboardCompletion from './dashboard-completion'
import miscCompletion from './misc-completion'
import adminOverviewCompletion from './admin/overview-completion'
import adminChannelsCompletion from './admin/channels-completion'
import adminAccountsCompletion from './admin/accounts-completion'
import adminResourcesCompletion from './admin/resources-completion'
import adminOpsCompletion from './admin/ops-completion'
import adminSettingsCompletion from './admin/settings-completion'

const completedAdmin = mergeLocaleMessages(
  admin,
  adminOverviewCompletion,
  adminChannelsCompletion,
  adminAccountsCompletion,
  adminResourcesCompletion,
  adminOpsCompletion,
  adminSettingsCompletion,
)

export default mergeLocaleMessages(
  {
    ...landing,
    ...common,
    ...dashboard,
    ...channelMonitorV2,
    ...batchImage,
    admin: completedAdmin,
    ...misc,
  },
  landingCompletion,
  commonCompletion,
  dashboardCompletion,
  miscCompletion,
)
