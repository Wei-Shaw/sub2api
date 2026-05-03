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
  PluginInstance,
  PluginRuntimeAssets,
  PluginRuntimeContext,
} from '@sub2api/plugin-sdk'
import {
  Icon,
  DataTable,
  BaseDialog,
  ConfirmDialog,
  Select,
  Pagination,
  EmptyState,
  Toggle,
  PlatformIcon,
} from '@sub2api/plugin-sdk'
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

// Auth callback (no-auth route — host must allow public render for it)
import WechatPaymentCallbackView from './views/auth/WechatPaymentCallbackView.vue'

// Admin views
import AdminPaymentDashboardView from './views/admin/AdminPaymentDashboardView.vue'
import AdminOrdersView from './views/admin/AdminOrdersView.vue'
import AdminPaymentPlansView from './views/admin/AdminPaymentPlansView.vue'
// Provider CRUD reuses existing PaymentProviderList component as a wrapper view.
import PaymentProviderList from './components/payment/PaymentProviderList.vue'

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
  'PaymentProvidersView.vue': PaymentProviderList,
  'PaymentConfigView.vue': AdminPaymentDashboardView, // TODO: dedicated config view (Phase 3)
  // User
  'RechargeView.vue': PaymentView,
  // Aliases for additional routes the host may surface (Phase 3 wires them):
  'PaymentView.vue': PaymentView,
  'UserOrdersView.vue': UserOrdersView,
  'PaymentQRCodeView.vue': PaymentQRCodeView,
  'PaymentResultView.vue': PaymentResultView,
  'StripePaymentView.vue': StripePaymentView,
  'StripePopupView.vue': StripePopupView,
  'WechatPaymentCallbackView.vue': WechatPaymentCallbackView,
}

/** SDK-shared components registered globally so SFCs can reference them by name. */
const SDK_GLOBAL_COMPONENTS: Record<string, Component> = {
  Icon,
  DataTable,
  BaseDialog,
  ConfirmDialog,
  Select,
  Pagination,
  EmptyState,
  Toggle,
  PlatformIcon,
}

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
    mount(shadowRoot: ShadowRoot, ctx: PluginRuntimeContext): PluginInstance {
      const view = VIEWS[ctx.componentPath]
      if (!view) {
        const fallback = document.createElement('div')
        fallback.style.padding = '2rem'
        fallback.style.color = '#dc2626'
        fallback.textContent = `[payment] unknown component: ${ctx.componentPath || '(empty)'}`
        const root = shadowRoot.querySelector('.plugin-shadow-root') as HTMLElement | null
        ;(root || shadowRoot).appendChild(fallback)
        return { unmount: () => fallback.remove() }
      }

      const target = shadowRoot.querySelector('.plugin-shadow-root') as HTMLElement | null
      if (!target) {
        throw new Error('[payment] plugin-shadow-root container not found in shadow root')
      }

      const instance = sdk.runtime.createApp(view, target, {
        components: SDK_GLOBAL_COMPONENTS,
      })
      return {
        unmount: () => instance.unmount(),
      }
    },
  }
}

export default { install }
export { install }
