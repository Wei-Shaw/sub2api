/**
 * Plugin host SDK accessor.
 *
 * install() saves the host-injected SDK to module scope; views & utilities
 * grab it via getSdk() to access notify / auth / theme / i18n. Module-scope
 * state (rather than provide/inject) keeps lookup synchronous and works for
 * code outside the Vue render context (e.g. format.ts, paymentFlow.ts).
 */
import type { HostSdk } from '@sub2api/plugin-sdk'

let sdk: HostSdk | null = null

export function setSdk(instance: HostSdk): void {
  sdk = instance
}

export function getSdk(): HostSdk {
  if (!sdk) {
    throw new Error('[plugin-payment] HostSdk not initialized. Call setSdk() during plugin install.')
  }
  return sdk
}
