/**
 * Module-scoped accessor factories for plugin frontends.
 *
 * Plugins receive their HostSdk and axios client via install(sdk) at runtime.
 * Both need to be reachable from non-Vue contexts (api modules, format helpers,
 * paymentFlow utilities) so module-scope state is the simplest contract.
 *
 * createSdkAccessor / createApiClient remove the boilerplate each plugin used
 * to copy: a let binding, a setter, and a getter that throws with the plugin
 * name on uninitialized access. Call once per plugin and re-export the pair.
 */
import type { AxiosInstance } from 'axios'
import type { HostSdk } from './host-sdk'

export interface SdkAccessor {
  setSdk(instance: HostSdk): void
  getSdk(): HostSdk
}

export interface ApiClientAccessor {
  setClient(instance: AxiosInstance): void
  getClient(): AxiosInstance
}

export function createSdkAccessor(pluginName: string): SdkAccessor {
  let sdk: HostSdk | null = null
  return {
    setSdk(instance: HostSdk): void {
      sdk = instance
    },
    getSdk(): HostSdk {
      if (!sdk) {
        throw new Error(
          `[${pluginName}] HostSdk not initialized. Call setSdk() during plugin install.`,
        )
      }
      return sdk
    },
  }
}

export function createApiClient(pluginName: string): ApiClientAccessor {
  let client: AxiosInstance | null = null
  return {
    setClient(instance: AxiosInstance): void {
      client = instance
    },
    getClient(): AxiosInstance {
      if (!client) {
        throw new Error(
          `[${pluginName}] API client not initialized. Call setClient() during plugin install.`,
        )
      }
      return client
    },
  }
}
