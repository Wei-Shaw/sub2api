/**
 * Gateway Antigravity plugin frontend entry (V2 protocol).
 *
 * This plugin only provides account form components (no mountable views).
 * The form runs directly in the host Vue tree via formComponents, not
 * inside a Shadow DOM.
 */
import type { HostSdk, PluginRuntimeAssets } from '@sub2api/plugin-sdk'
import type { AxiosInstance } from 'axios'
import { setClient } from './api/client'
import AntigravityForm from './forms/AntigravityForm.vue'
import enMessages from './i18n/en'
import zhMessages from './i18n/zh'

const I18N_NAMESPACE = 'gateway-antigravity'

function install(sdk: HostSdk): PluginRuntimeAssets {
  // Inject host axios instance for API calls
  setClient(sdk.http.apiClient as unknown as AxiosInstance)

  // Register plugin i18n (currently empty, but ready for future keys)
  sdk.i18n.registerNamespace(I18N_NAMESPACE, {
    en: enMessages,
    zh: zhMessages,
  })

  return {
    mount() {
      throw new Error('[gateway-antigravity] has no mountable views')
    },
    formComponents: {
      'AntigravityForm': AntigravityForm,
    },
  }
}

export default { install }
export { install }
