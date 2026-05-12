/// <reference types="vite/client" />

interface ImportMetaEnv {
  readonly VITE_API_BASE_URL: string
  readonly BASE_URL: string
}

interface ImportMeta {
  readonly env: ImportMetaEnv
}

declare module '*.vue' {
  import type { DefineComponent } from 'vue'
  const component: DefineComponent<{}, {}, any>
  export default component
}

declare module '@airwallex/components-sdk' {
  export interface AirwallexInitOptions {
    env: 'demo' | 'prod'
    enabledElements?: string[]
    locale?: string
  }

  export interface AirwallexCheckoutOptions {
    intent_id: string
    client_secret: string
    currency: string
    country_code: string
    successUrl: string
  }

  export interface AirwallexPayments {
    redirectToCheckout(options: AirwallexCheckoutOptions): string | void | Promise<string | void>
  }

  export interface AirwallexInitResult {
    payments?: AirwallexPayments
  }

  export function init(options: AirwallexInitOptions): Promise<AirwallexInitResult>
}
