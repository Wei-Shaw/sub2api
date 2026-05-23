/**
 * Gemini gateway plugin frontend entry.
 *
 * This plugin provides the GeminiForm account form component and
 * GeminiGroupConfig group config component. Unlike full Shadow DOM
 * plugins, gateway form plugins only expose formComponents and
 * groupConfigComponents -- the host renders them directly in its Vue tree.
 */
import type { HostSdk, PluginRuntimeAssets } from '@sub2api/plugin-sdk'
import type { AxiosInstance } from 'axios'
import GeminiForm from './forms/GeminiForm.vue'
import GeminiGroupConfig from './components/GeminiGroupConfig.vue'
import GeminiUsageSection from './components/GeminiUsageSection.vue'
import AccountQuotaInfo from './components/AccountQuotaInfo.vue'
import GeminiTestPanel from './components/GeminiTestPanel.vue'
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
      throw new Error('[gateway-gemini] This plugin does not provide mountable views')
    },
    formComponents: {
      GeminiForm,
    },
    groupConfigComponents: {
      GeminiGroupConfig,
    },
    // Account test panel component rendered in host Vue tree
    testComponents: {
      GeminiTestPanel,
    },
    // Account usage display components rendered in host Vue tree
    usageComponents: {
      'gemini:*': GeminiUsageSection,
      'gemini:quota-info': AccountQuotaInfo,
    },
  }
}

export default { install }
export { install }
