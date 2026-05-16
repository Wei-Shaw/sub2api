/**
 * Gateway Anthropic plugin frontend entry.
 *
 * This plugin provides the account form component for the Anthropic platform.
 * The form is rendered in the host Vue tree (not Shadow DOM) via the
 * formComponents contract in PluginRuntimeAssets.
 */
import type { HostSdk, PluginRuntimeAssets } from '@sub2api/plugin-sdk'
import type { AxiosInstance } from 'axios'
import './style.css'
import AnthropicForm from './forms/AnthropicForm.vue'
import { setClient } from './api/client'
import { setSdk } from './api/sdk'
import enMessages from './i18n/en'
import zhMessages from './i18n/zh'

const I18N_NAMESPACE = 'gateway-anthropic'

function install(sdk: HostSdk): PluginRuntimeAssets {
  // Inject host axios instance for API calls
  setClient(sdk.http.apiClient as unknown as AxiosInstance)
  setSdk(sdk)

  // Register plugin i18n messages
  sdk.i18n.registerNamespace(I18N_NAMESPACE, {
    en: enMessages,
    zh: zhMessages,
  })

  return {
    // No Shadow DOM views -- this plugin only provides form components
    mount() {
      return { unmount() {} }
    },
    // Account form components rendered in host Vue tree
    formComponents: {
      AnthropicForm,
    },
  }
}

export default { install }
export { install }
