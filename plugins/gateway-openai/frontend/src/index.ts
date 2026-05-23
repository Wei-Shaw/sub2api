/**
 * Gateway OpenAI plugin frontend runtime entry.
 *
 * This plugin provides form components for OpenAI account management
 * and group configuration. Unlike full-page plugins, this plugin only
 * provides formComponents and groupConfigComponents (rendered in host
 * Vue tree, not Shadow DOM).
 */
import type {
  HostSdk,
  PluginRuntimeAssets,
} from '@sub2api/plugin-sdk'
import type { AxiosInstance } from 'axios'
import { setClient } from './api/client'
import { setSdk } from './api/sdk'
import OpenAIForm from './forms/OpenAIForm.vue'
import OpenAIGroupConfig from './components/OpenAIGroupConfig.vue'
import OpenAIUsageSection from './components/OpenAIUsageSection.vue'
import OpenAITestPanel from './components/OpenAITestPanel.vue'
import enMessages from './i18n/en'
import zhMessages from './i18n/zh'

const I18N_NAMESPACE = 'gateway-openai'

function install(sdk: HostSdk): PluginRuntimeAssets {
  setClient(sdk.http.apiClient as unknown as AxiosInstance)
  setSdk(sdk)

  // Register plugin i18n namespace (currently empty — placeholder so future
  // keys can be added without touching install()).
  sdk.i18n.registerNamespace(I18N_NAMESPACE, {
    en: enMessages,
    zh: zhMessages,
  })

  return {
    mount() {
      throw new Error('[gateway-openai] This plugin does not provide mountable views')
    },

    formComponents: {
      OpenAIForm,
    },

    groupConfigComponents: {
      OpenAIGroupConfig,
    },

    // Account test panel components rendered in host Vue tree
    testComponents: {
      OpenAITestPanel,
    },

    // Account usage display components rendered in host Vue tree
    usageComponents: {
      'openai:oauth': OpenAIUsageSection,
    },
  }
}

export default { install }
export { install }
