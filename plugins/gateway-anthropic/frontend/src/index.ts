/**
 * Gateway Anthropic plugin frontend entry.
 *
 * This plugin provides account form components, group config components,
 * and account usage display components for the Anthropic platform.
 * All are rendered in the host Vue tree (not Shadow DOM) via the
 * formComponents / groupConfigComponents / usageComponents contracts.
 */
import type { HostSdk, PluginRuntimeAssets } from '@sub2api/plugin-sdk'
import type { AxiosInstance } from 'axios'
import './style.css'
import AnthropicForm from './forms/AnthropicForm.vue'
import AnthropicUsageSection from './components/AnthropicUsageSection.vue'
import AnthropicGroupConfig from './components/group-config/AnthropicGroupConfig.vue'
import AnthropicTestPanel from './components/AnthropicTestPanel.vue'
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
    // Plugin only contributes form/group/usage/test components rendered in the
    // host Vue tree; calling mount() means the host routed to a non-existent
    // shadow-root view, which is a configuration error — fail fast.
    mount() {
      throw new Error('[gateway-anthropic] This plugin does not provide mountable views')
    },
    // Account form components rendered in host Vue tree
    formComponents: {
      AnthropicForm,
    },
    // Group config components rendered in host Vue tree
    groupConfigComponents: {
      AnthropicGroupConfig,
    },
    // Account usage display components rendered in host Vue tree
    usageComponents: {
      'anthropic:oauth': AnthropicUsageSection,
      'anthropic:setup-token': AnthropicUsageSection,
    },
    // Account test panel components rendered in host Vue tree
    testComponents: {
      AnthropicTestPanel,
    },
  }
}

export default { install }
export { install }
