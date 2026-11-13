import overview from './overview'
import channels from './channels'
import accounts from './accounts'
import resources from './resources'
import ops from './ops'
import settings from './settings'
import audit from './audit'
import promptAudit from './promptAudit'
import plugins from './plugins'
import traceRequest from './traceRequest'
import traceAccount from './traceAccount'
import traceRenewal from './traceRenewal'

export default {
  ...overview,
  ...channels,
  ...accounts,
  ...resources,
  ...ops,
  ...settings,
  ...audit,
  ...promptAudit,
  ...plugins,
  ...traceRequest,
  ...traceAccount,
  ...traceRenewal,
}
