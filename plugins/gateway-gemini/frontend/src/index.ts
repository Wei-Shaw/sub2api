/**
 * Gemini gateway plugin frontend entry.
 *
 * This plugin provides the GeminiForm account form component. Unlike full
 * Shadow DOM plugins (payment, channel-management), gateway form plugins
 * only expose formComponents — the host renders them directly in its Vue
 * tree (EditAccountModal / CreateAccountModal).
 *
 * Contract:
 *   - default export { install(sdk) }
 *   - install returns { formComponents: { 'GeminiForm': Component } }
 *   - No mount() needed (form is rendered in host tree, not Shadow DOM)
 */
import type { HostSdk, PluginRuntimeAssets } from '@sub2api/plugin-sdk'
import type { AxiosInstance } from 'axios'
import GeminiForm from './forms/GeminiForm.vue'
import { setClient } from './api/client'
import { setSdk } from './api/sdk'
import enMessages from './i18n/en'
import zhMessages from './i18n/zh'

const I18N_NAMESPACE = 'gateway-gemini'

function install(sdk: HostSdk): PluginRuntimeAssets {
  setClient(sdk.http.apiClient as unknown as AxiosInstance)
  setSdk(sdk)

  sdk.i18n.registerNamespace(I18N_NAMESPACE, {
    en: enMessages,
    zh: zhMessages,
  })

  return {
    mount() {
      throw new Error('[gateway-gemini] This plugin does not support Shadow DOM mounting. Use formComponents instead.')
    },
    formComponents: {
      GeminiForm,
    },
  }
}

export default { install }
export { install }
