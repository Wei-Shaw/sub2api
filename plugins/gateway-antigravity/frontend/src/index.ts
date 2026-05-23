/**
 * Gateway Antigravity plugin frontend entry (V2 protocol).
 *
 * This plugin provides account form components, group config components,
 * and account usage display components. All run directly in the host Vue
 * tree via formComponents / groupConfigComponents / usageComponents, not
 * inside a Shadow DOM.
 */
import type { HostSdk, PluginRuntimeAssets } from '@sub2api/plugin-sdk'
import type { AxiosInstance } from 'axios'
import { setClient } from './api/client'
import { setSdk } from './api/sdk'
import AntigravityForm from './forms/AntigravityForm.vue'
import AntigravityUsageSection from './components/AntigravityUsageSection.vue'
import AntigravityGroupConfig from './components/AntigravityGroupConfig.vue'
import AntigravityTestPanel from './components/AntigravityTestPanel.vue'
import enMessages from './i18n/en'
import zhMessages from './i18n/zh'

const I18N_NAMESPACE = 'gateway-antigravity'

function install(sdk: HostSdk): PluginRuntimeAssets {
  // Inject host axios instance for API calls
  setClient(sdk.http.apiClient as unknown as AxiosInstance)
  setSdk(sdk)

  // Register plugin i18n (currently empty, but ready for future keys)
  sdk.i18n.registerNamespace(I18N_NAMESPACE, {
    en: enMessages,
    zh: zhMessages,
  })

  return {
    mount() {
      throw new Error('[plugin-gateway-antigravity] This plugin does not provide mountable views')
    },
    formComponents: {
      AntigravityForm,
    },
    groupConfigComponents: {
      AntigravityGroupConfig,
    },
    testComponents: {
      AntigravityTestPanel,
    },
    // Account usage display components rendered in host Vue tree
    usageComponents: {
      'antigravity:oauth': AntigravityUsageSection,
    },
  }
}

export default { install }
export { install }