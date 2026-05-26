/**
 * Payment plugin frontend runtime entry (V2 Shadow DOM protocol).
 *
 * Mirrors plugins/channel-management/frontend/src/index.ts:
 *   - default export { install(sdk) } → returns { mount(shadowRoot, ctx) }
 *   - mount creates a Vue app via sdk.runtime.createApp() inside the host's
 *     ShadowRoot, dispatching to the appropriate root view based on
 *     ctx.componentPath (route → Vue component file mapping).
 *
 * Shared singletons (vue/pinia/i18n/axios/plugin-sdk) come from the host's
 * importmap, so the plugin bundle stays small.
 */
import type {
  HostSdk,
  PluginRuntimeAssets,
} from '@sub2api/plugin-sdk'
import { SDK_GLOBAL_COMPONENTS, createPluginMount } from '@sub2api/plugin-sdk'
import type { AxiosInstance } from 'axios'
import type { Component } from 'vue'
import './style.css'

// User-facing views
import PaymentView from './views/user/PaymentView.vue'
import UserOrdersView from './views/user/UserOrdersView.vue'
import PaymentQRCodeView from './views/user/PaymentQRCodeView.vue'
import PaymentResultView from './views/user/PaymentResultView.vue'
import StripePaymentView from './views/user/StripePaymentView.vue'
import StripePopupView from './views/user/StripePopupView.vue'
import AirwallexPaymentView from './views/user/AirwallexPaymentView.vue'

// Auth callback (no-auth route — host must allow public render for it)
import WechatPaymentCallbackView from './views/auth/WechatPaymentCallbackView.vue'

// Admin views
import AdminPaymentDashboardView from './views/admin/AdminPaymentDashboardView.vue'
import AdminOrdersView from './views/admin/AdminOrdersView.vue'
import AdminPaymentPlansView from './views/admin/AdminPaymentPlansView.vue'
// Provider CRUD wrapper view. PaymentProviderList alone is a child component
// (requires `providers` prop); the wrapper owns state, fetches, and dialogs.
import PaymentProvidersView from './views/admin/PaymentProvidersView.vue'
// Custom settings UI — host PluginSettingsForm mounts this when the manifest
// declares SettingsComponentPath = "PaymentSettingsView.vue". See plugin.go
// Manifest() and frontend/src/components/admin/PluginSettingsForm.vue.
import PaymentSettingsView from './views/admin/PaymentSettingsView.vue'

import { setClient } from './api/client'
import { setSdk } from './api/sdk'
import enMessages from './i18n/en'
import zhMessages from './i18n/zh'

/** Plugin namespace key — must match the value declared in plugins/payment/manifest_decls.go. */
const I18N_NAMESPACE = 'payment'

/**
 * componentPath → root view component map. Keys mirror the ComponentPath
 * declared in plugins/payment/manifest_decls.go routeDecls.
 */
const VIEWS: Record<string, Component> = {
  // Admin
  'PaymentDashboardView.vue': AdminPaymentDashboardView,
  'PaymentOrdersView.vue': AdminOrdersView,
  'PaymentPlansView.vue': AdminPaymentPlansView,
  'PaymentProvidersView.vue': PaymentProvidersView,
  'PaymentSettingsView.vue': PaymentSettingsView,
  // User
  'RechargeView.vue': PaymentView,
  // Aliases for additional routes the host may surface (Phase 3 wires them):
  'PaymentView.vue': PaymentView,
  'UserOrdersView.vue': UserOrdersView,
  'PaymentQRCodeView.vue': PaymentQRCodeView,
  'PaymentResultView.vue': PaymentResultView,
  'StripePaymentView.vue': StripePaymentView,
  'StripePopupView.vue': StripePopupView,
  'AirwallexPaymentView.vue': AirwallexPaymentView,
  'WechatPaymentCallbackView.vue': WechatPaymentCallbackView,
}

/** SDK-shared components registered globally are imported from the SDK. See SDK_GLOBAL_COMPONENTS. */

function install(sdk: HostSdk): PluginRuntimeAssets {
  // 1. Inject host axios. Type cast: axios may resolve to two different
  //    nominal types between host and plugin tsconfigs; runtime is the same
  //    instance via importmap.
  setClient(sdk.http.apiClient as unknown as AxiosInstance)

  // 2. Stash full SDK for views/utilities to access notify / auth / theme.
  setSdk(sdk)

  // 3. Merge plugin i18n into host vue-i18n (idempotent across reinstalls).
  sdk.i18n.registerNamespace(I18N_NAMESPACE, {
    en: enMessages,
    zh: zhMessages,
  })

  return {
    mount: createPluginMount(sdk, VIEWS, 'plugin-payment', {
      globalComponents: SDK_GLOBAL_COMPONENTS,
    }),
  }
}

export default { install }
export { install }
