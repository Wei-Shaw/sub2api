declare module '@airwallex/components-sdk' {
  type Env = 'dev' | 'staging' | 'demo' | 'prod' | 'sandbox'
  type ElementGroup = 'payments' | 'payouts' | 'onboarding' | 'risk'

  interface Payments {
    redirectToCheckout: (props: {
      intent_id: string
      client_secret: string
      currency: string
      country_code: string
      successUrl?: string
      [k: string]: unknown
    }) => void | string
  }

  type InitResult = {
    payments?: Payments
    sca?: {
      getScaToken: () => Promise<string>
    }
  }

  type Options = {
    env: Env
    enabledElements: ElementGroup[]
    locale?: string
    [k: string]: unknown
  }

  export function init(options: Options): Promise<InitResult>
}
