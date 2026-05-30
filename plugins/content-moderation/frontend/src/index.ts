/**
 * Content Moderation plugin frontend runtime entry (V2 — Shadow DOM protocol).
 *
 * Contract (aligned with host loader-runtime.ts + mount-plugin.ts):
 *   - default export must be { install(sdk) }
 *   - install returns { mount(shadowRoot, ctx) -> PluginInstance }
 *   - mount renders the plugin Vue app inside the host-provided ShadowRoot
 *
 * Shared context (provided by host importmap):
 *   - vue / vue-router / vue-i18n / pinia / axios — same singletons as host
 *   - @sub2api/plugin-sdk — host-compiled SDK bundle (incl. shared components)
 *
 * Side effects on install:
 *   - inject host axios into the plugin's API client singleton
 *   - store the SDK handle for view-layer access (notify / i18n / etc.)
 *   - register plugin i18n messages onto the host vue-i18n instance
 */
import type { HostSdk, PluginRuntimeAssets } from '@sub2api/plugin-sdk'
import { SDK_GLOBAL_COMPONENTS, createPluginMount } from '@sub2api/plugin-sdk'
import type { AxiosInstance } from 'axios'
import type { Component } from 'vue'
import './style.css'
import RiskControlView from './views/RiskControlView.vue'
import { setClient } from './api/client'
import { setSdk } from './api/sdk'
import enMessages from './i18n/en'
import zhMessages from './i18n/zh'

/** Dedup key, aligned with the backend manifest I18nNamespaces entry. */
const I18N_NAMESPACE = 'contentModerationPlugin'

/** componentPath -> Vue view component (matches manifest RouteDecl.ComponentPath). */
const VIEWS: Record<string, Component> = {
  'RiskControlView.vue': RiskControlView,
}

function install(sdk: HostSdk): PluginRuntimeAssets {
  // Inject host axios so plugin API calls reuse the host auth header / error
  // interceptors. Runtime is the same axios instance (shared via importmap);
  // the unknown bridge crosses the two nominal AxiosInstance types resolved by
  // host vs plugin node_modules.
  setClient(sdk.http.apiClient as unknown as AxiosInstance)
  setSdk(sdk)

  sdk.i18n.registerNamespace(I18N_NAMESPACE, {
    en: enMessages,
    zh: zhMessages,
  })

  return {
    mount: createPluginMount(sdk, VIEWS, 'plugin-content-moderation', {
      globalComponents: SDK_GLOBAL_COMPONENTS,
    }),
  }
}

export default { install }
export { install }
