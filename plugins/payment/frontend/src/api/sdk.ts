/**
 * Plugin host SDK accessor.
 *
 * install() saves the host-injected SDK to module scope; views & utilities
 * grab it via getSdk() to access notify / auth / theme / i18n. Module-scope
 * state (rather than provide/inject) keeps lookup synchronous and works for
 * code outside the Vue render context (e.g. format.ts, paymentFlow.ts).
 */
import { createSdkAccessor } from '@sub2api/plugin-sdk'

export const { setSdk, getSdk } = createSdkAccessor('plugin-payment')
