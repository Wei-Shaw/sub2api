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
  PluginInstance,
  PluginRuntimeAssets,
  PluginRuntimeContext,
} from '@sub2api/plugin-sdk'
import type { AxiosInstance } from 'axios'
import { setClient } from './api/client'
import { setSdk } from './api/sdk'
import OpenAIForm from './forms/OpenAIForm.vue'
import OpenAIGroupConfig from './components/OpenAIGroupConfig.vue'
import OpenAIUsageSection from './components/OpenAIUsageSection.vue'

function install(sdk: HostSdk): PluginRuntimeAssets {
  setClient(sdk.http.apiClient as unknown as AxiosInstance)
  setSdk(sdk)

  return {
    mount(shadowRoot: ShadowRoot, ctx: PluginRuntimeContext): PluginInstance {
      // gateway-openai has no full-page views -- only formComponents.
      const fallback = document.createElement('div')
      fallback.style.padding = '2rem'
      fallback.style.color = '#dc2626'
      fallback.textContent = `[gateway-openai] no view for: ${ctx.componentPath || '(empty)'}`
      const root = shadowRoot.querySelector('.plugin-shadow-root') as HTMLElement | null
      if (root) root.appendChild(fallback)
      else shadowRoot.appendChild(fallback)
      return { unmount: () => fallback.remove() }
    },

    formComponents: {
      OpenAIForm,
    },

    groupConfigComponents: {
      OpenAIGroupConfig,
    },

    // Account usage display components rendered in host Vue tree
    usageComponents: {
      'openai:oauth': OpenAIUsageSection,
    },
  }
}

export default { install }
export { install }
