import { describe, expect, it } from 'vitest'
import type { CreateOrderResult, MethodLimit } from '@/types/payment'
import {
  buildCreateOrderPayload,
  decidePaymentLaunch,
  getVisibleMethods,
  readPaymentRecoverySnapshot,
  type PaymentRecoverySnapshot,
} from '@/components/payment/paymentFlow'

function methodLimit(overrides: Partial<MethodLimit> = {}): MethodLimit {
  return {
    daily_limit: 0,
    daily_used: 0,
    daily_remaining: 0,
    single_min: 0,
    single_max: 0,
    fee_rate: 0,
    available: true,
    ...overrides,
  }
}

function createOrderResult(overrides: Partial<CreateOrderResult> = {}): CreateOrderResult {
  return {
    order_id: 101,
    amount: 88,
    pay_amount: 88,
    fee_rate: 0,
    expires_at: '2099-01-01T00:10:00.000Z',
    ...overrides,
  }
}

describe('getVisibleMethods', () => {
  it('normalizes provider aliases and keeps stripe as a top-level method', () => {
    const visible = getVisibleMethods({
      alipay_direct: methodLimit({ single_min: 5 }),
      wxpay: methodLimit({ single_max: 100 }),
      stripe: methodLimit({ fee_rate: 3 }),
      airwallex: methodLimit({ single_min: 10 }),
    })

    expect(visible).toEqual({
      alipay: methodLimit({ single_min: 5 }),
      wxpay: methodLimit({ single_max: 100 }),
      stripe: methodLimit({ fee_rate: 3 }),
      airwallex: methodLimit({ single_min: 10 }),
    })
  })

  it('prefers canonical visible methods over aliases when both exist', () => {
    const visible = getVisibleMethods({
      alipay: methodLimit({ single_min: 2 }),
      alipay_direct: methodLimit({ single_min: 9 }),
      wxpay_direct: methodLimit({ fee_rate: 1.2 }),
    })

    expect(visible.alipay.single_min).toBe(2)
    expect(visible.wxpay.fee_rate).toBe(1.2)
  })

  it('keeps custom EasyPay methods as visible methods', () => {
    const visible = getVisibleMethods({
      ldc: methodLimit({ single_min: 3 }),
      usdt_trc20: methodLimit({ fee_rate: 1 }),
    })

    expect(visible).toEqual({
      ldc: methodLimit({ single_min: 3 }),
      usdt_trc20: methodLimit({ fee_rate: 1 }),
    })
  })
})

describe('decidePaymentLaunch', () => {
  it('uses Stripe popup waiting flow for desktop Alipay client secret', () => {
    const decision = decidePaymentLaunch(createOrderResult({
      client_secret: 'cs_test',
      resume_token: 'resume-1',
    }), {
      visibleMethod: 'alipay',
      orderType: 'balance',
      isMobile: false,
    })

    expect(decision.kind).toBe('stripe_popup')
    expect(decision.paymentState.paymentType).toBe('alipay')
    expect(decision.stripeMethod).toBe('alipay')
    expect(decision.recovery.resumeToken).toBe('resume-1')
    expect(decision.recovery.outTradeNo).toBe('')
  })

  it('routes Stripe button click to the full Payment Element without a preselected sub-method', () => {
    const decision = decidePaymentLaunch(createOrderResult({
      client_secret: 'cs_test',
    }), {
      visibleMethod: 'stripe',
      orderType: 'balance',
      isMobile: false,
    })

    expect(decision.kind).toBe('stripe_route')
    expect(decision.stripeMethod).toBeUndefined()
  })

  it('uses Stripe route flow for mobile WeChat client secret', () => {
    const decision = decidePaymentLaunch(createOrderResult({
      client_secret: 'cs_test',
    }), {
      visibleMethod: 'wxpay',
      orderType: 'subscription',
      isMobile: true,
    })

    expect(decision.kind).toBe('stripe_route')
    expect(decision.stripeMethod).toBe('wechat_pay')
    expect(decision.paymentState.orderType).toBe('subscription')
  })

  it('routes Airwallex client secrets through the hosted Airwallex page', () => {
    const decision = decidePaymentLaunch(createOrderResult({
      client_secret: 'awx_cs',
      intent_id: 'int_awx',
      currency: 'CNY',
      country_code: 'CN',
      payment_env: 'demo',
      out_trade_no: 'sub2_awx',
    }), {
      visibleMethod: 'airwallex',
      orderType: 'balance',
      isMobile: false,
      airwallexRouteUrl: '/payment/airwallex?order_id=101',
    })

    expect(decision.kind).toBe('airwallex_route')
    expect(decision.paymentState.payUrl).toBe('/payment/airwallex?order_id=101')
    expect(decision.paymentState.intentId).toBe('int_awx')
    expect(decision.paymentState.currency).toBe('CNY')
    expect(decision.paymentState.countryCode).toBe('CN')
    expect(decision.paymentState.paymentEnv).toBe('demo')
  })

  it('keeps hosted redirect metadata for recovery flows', () => {
    const decision = decidePaymentLaunch(createOrderResult({
      pay_url: 'https://pay.example.com/session/abc',
      payment_mode: 'popup',
      resume_token: 'resume-2',
      out_trade_no: 'sub2_abc',
    }), {
      visibleMethod: 'wxpay',
      orderType: 'balance',
      isMobile: false,
    })

    expect(decision.kind).toBe('redirect_waiting')
    expect(decision.paymentState.payUrl).toBe('https://pay.example.com/session/abc')
    expect(decision.recovery.paymentMode).toBe('popup')
    expect(decision.recovery.outTradeNo).toBe('sub2_abc')
    expect(decision.recovery.resumeToken).toBe('resume-2')
  })

  it('prefers redirect on mobile when both pay_url and qr_code are present', () => {
    const decision = decidePaymentLaunch(createOrderResult({
      pay_url: 'https://pay.example.com/mobile/session',
      qr_code: 'https://pay.example.com/qr/session',
    }), {
      visibleMethod: 'alipay',
      orderType: 'balance',
      isMobile: true,
    })

    expect(decision.kind).toBe('redirect_waiting')
    expect(decision.paymentState.payUrl).toBe('https://pay.example.com/mobile/session')
    expect(decision.paymentState.qrCode).toBe('https://pay.example.com/qr/session')
  })

  it('keeps QR flow on desktop when both pay_url and qr_code are present', () => {
    const decision = decidePaymentLaunch(createOrderResult({
      pay_url: 'https://pay.example.com/desktop/session',
      qr_code: 'https://pay.example.com/qr/session',
    }), {
      visibleMethod: 'wxpay',
      orderType: 'balance',
      isMobile: false,
    })

    expect(decision.kind).toBe('qr_waiting')
    expect(decision.paymentState.qrCode).toBe('https://pay.example.com/qr/session')
  })

  it('treats a qr_code identical to checkout_url as a hosted redirect on desktop', () => {
    const checkoutUrl = 'https://checkout.example.com/session/hosted'
    const decision = decidePaymentLaunch(createOrderResult({
      checkout_url: checkoutUrl,
      qr_code: checkoutUrl,
    }), {
      visibleMethod: 'paddle',
      orderType: 'subscription',
      isMobile: false,
    })

    expect(decision.kind).toBe('redirect_waiting')
    expect(decision.paymentState.payUrl).toBe(checkoutUrl)
    expect(decision.paymentState.qrCode).toBe(checkoutUrl)
  })

  it('uses checkout_url as redirect URL when a provider exposes only canonical checkout_url', () => {
    const decision = decidePaymentLaunch(createOrderResult({
      checkout_url: 'https://checkout.example.com/session/abc',
      payment_mode: 'redirect',
    }), {
      visibleMethod: 'alipay',
      orderType: 'balance',
      isMobile: false,
    })

    expect(decision.kind).toBe('redirect_waiting')
    expect(decision.paymentState.payUrl).toBe('https://checkout.example.com/session/abc')
    expect(decision.recovery.payUrl).toBe('https://checkout.example.com/session/abc')
  })

  it('normalizes launch data for SePay, Alipay, WeChat, and Paddle provider contracts', () => {
    const scenarios = [
      {
        name: 'sepay provider-generated QR image URL',
        result: createOrderResult({
          qr_code: 'https://qr.sepay.vn/img?acc=123&bank=MBBank&amount=50000&des=VC123',
          payment_mode: 'qrcode',
        }),
        context: { visibleMethod: 'sepay', orderType: 'balance' as const, isMobile: false },
        expectedKind: 'qr_waiting',
        expectedQrCode: 'https://qr.sepay.vn/img?acc=123&bank=MBBank&amount=50000&des=VC123',
      },
      {
        name: 'alipay canonical hosted checkout URL',
        result: createOrderResult({
          checkout_url: 'https://checkout.alipay.example.com/session/abc',
          payment_mode: 'redirect',
        }),
        context: { visibleMethod: 'alipay', orderType: 'balance' as const, isMobile: false },
        expectedKind: 'redirect_waiting',
        expectedPayUrl: 'https://checkout.alipay.example.com/session/abc',
      },
      {
        name: 'wechat native QR payload',
        result: createOrderResult({
          qr_code: 'weixin://wxpay/bizpayurl?pr=test',
          payment_mode: 'native',
        }),
        context: { visibleMethod: 'wxpay', orderType: 'balance' as const, isMobile: false },
        expectedKind: 'qr_waiting',
        expectedQrCode: 'weixin://wxpay/bizpayurl?pr=test',
      },
      {
        name: 'paddle first-party checkout URL with provider checkout id',
        result: createOrderResult({
          checkout_id: 'txn_123',
          checkout_url: 'https://token.v-claw.org/checkout?provider=paddle&checkout_id=txn_123',
          payment_mode: 'redirect',
        }),
        context: { visibleMethod: 'paddle', orderType: 'subscription' as const, isMobile: false },
        expectedKind: 'redirect_waiting',
        expectedPayUrl: 'https://token.v-claw.org/checkout?provider=paddle&checkout_id=txn_123',
      },
    ]

    scenarios.forEach((scenario) => {
      const decision = decidePaymentLaunch(scenario.result, scenario.context)
      expect(decision.kind, scenario.name).toBe(scenario.expectedKind)
      if (scenario.expectedPayUrl) {
        expect(decision.paymentState.payUrl, scenario.name).toBe(scenario.expectedPayUrl)
      }
      if (scenario.expectedQrCode) {
        expect(decision.paymentState.qrCode, scenario.name).toBe(scenario.expectedQrCode)
      }
    })
  })

  it('keeps legacy inline Paddle launch when the provider exposes checkout_id without a launch URL', () => {
    const decision = decidePaymentLaunch(createOrderResult({
      checkout_id: 'txn_inline_only',
      payment_mode: 'redirect',
    }), {
      visibleMethod: 'paddle',
      orderType: 'subscription',
      isMobile: false,
    })

    expect(decision.kind).toBe('unhandled')
    expect(decision.paymentState.checkoutId).toBe('txn_inline_only')
    expect(decision.paymentState.payUrl).toBe('')
  })

  it('returns wechat oauth launch when backend requires in-app authorization', () => {
    const decision = decidePaymentLaunch(createOrderResult({
      result_type: 'oauth_required',
      payment_type: 'wxpay',
      oauth: {
        authorize_url: '/api/v1/auth/oauth/wechat/payment/start?payment_type=wxpay',
        appid: 'wx123',
        scope: 'snsapi_base',
        redirect_url: '/auth/wechat/payment/callback',
      },
    }), {
      visibleMethod: 'wxpay',
      orderType: 'balance',
      isMobile: true,
    })

    expect(decision.kind).toBe('wechat_oauth')
    expect(decision.oauth?.authorize_url).toContain('/api/v1/auth/oauth/wechat/payment/start')
    expect(decision.paymentState.paymentType).toBe('wxpay')
  })

  it('returns wechat jsapi launch when backend has a jsapi payload ready', () => {
    const decision = decidePaymentLaunch(createOrderResult({
      result_type: 'jsapi_ready',
      payment_type: 'wxpay',
      jsapi: {
        appId: 'wx123',
        timeStamp: '1712345678',
        nonceStr: 'nonce-123',
        package: 'prepay_id=wx123',
        signType: 'RSA',
        paySign: 'signed-payload',
      },
    }), {
      visibleMethod: 'wxpay',
      orderType: 'subscription',
      isMobile: true,
    })

    expect(decision.kind).toBe('wechat_jsapi')
    expect(decision.jsapi?.appId).toBe('wx123')
    expect(decision.paymentState.orderType).toBe('subscription')
  })

  it('forces qr_waiting for mobile alipay when forceQRCode is enabled', () => {
    const decision = decidePaymentLaunch(createOrderResult({
      pay_url: 'https://pay.example.com/mobile/session',
      qr_code: 'https://pay.example.com/qr/session',
    }), {
      visibleMethod: 'alipay',
      orderType: 'balance',
      isMobile: true,
      forceQRCode: true,
    })

    expect(decision.kind).toBe('qr_waiting')
    expect(decision.paymentState.qrCode).toBe('https://pay.example.com/qr/session')
  })

  it('launches the Alipay app for a mobile precreate order', () => {
    const decision = decidePaymentLaunch(createOrderResult({
      qr_code: 'https://qr.alipay.com/dynamic-order-101',
      alipay_mobile_precreate_deep_link: true,
    }), {
      visibleMethod: 'alipay',
      orderType: 'balance',
      isMobile: true,
    })

    expect(decision.kind).toBe('alipay_deep_link')
    expect(decision.paymentState.qrCode).toBe('https://qr.alipay.com/dynamic-order-101')
    expect(decision.paymentState.alipayMobilePrecreateDeepLink).toBe(true)
  })

  it('keeps the desktop Alipay QR flow when a precreate marker is present', () => {
    const decision = decidePaymentLaunch(createOrderResult({
      qr_code: 'https://qr.alipay.com/dynamic-order-102',
      alipay_mobile_precreate_deep_link: true,
    }), {
      visibleMethod: 'alipay',
      orderType: 'balance',
      isMobile: false,
    })

    expect(decision.kind).toBe('qr_waiting')
  })

  it('does not affect non-alipay methods when forceQRCode is enabled', () => {
    const decision = decidePaymentLaunch(createOrderResult({
      pay_url: 'https://pay.example.com/mobile/session',
      qr_code: 'https://pay.example.com/qr/session',
    }), {
      visibleMethod: 'wxpay',
      orderType: 'balance',
      isMobile: true,
      forceQRCode: true,
    })

    // wxpay mobile with pay_url still redirects
    expect(decision.kind).toBe('redirect_waiting')
  })
})

describe('buildCreateOrderPayload', () => {
  it('normalizes visible method aliases and attaches a canonical result URL', () => {
    expect(buildCreateOrderPayload({
      amount: 88,
      paymentType: 'alipay_direct',
      orderType: 'balance',
      origin: 'https://app.example.com/',
      isMobile: true,
      isWechatBrowser: false,
    })).toEqual({
      amount: 88,
      payment_type: 'alipay',
      order_type: 'balance',
      return_url: 'https://app.example.com/payment/result',
      is_mobile: true,
      payment_source: 'hosted_redirect',
    })
  })

  it('uses WeChat in-app resume source for visible WeChat payments in the WeChat browser', () => {
    expect(buildCreateOrderPayload({
      amount: 128,
      paymentType: 'wxpay',
      orderType: 'subscription',
      planId: 7,
      origin: 'https://app.example.com',
      isMobile: false,
      isWechatBrowser: true,
    })).toEqual({
      amount: 128,
      payment_type: 'wxpay',
      order_type: 'subscription',
      plan_id: 7,
      return_url: 'https://app.example.com/payment/result',
      is_mobile: false,
      payment_source: 'wechat_in_app_resume',
    })
  })

  it('passes is_mobile: false when forceQRCode is enabled for alipay', () => {
    expect(buildCreateOrderPayload({
      amount: 50,
      paymentType: 'alipay',
      orderType: 'balance',
      origin: 'https://app.example.com',
      isMobile: true,
      isWechatBrowser: false,
      forceQRCode: true,
    })).toMatchObject({
      is_mobile: false,
    })
  })

  it('keeps is_mobile true when mobile precreate takes priority over forceQRCode', () => {
    expect(buildCreateOrderPayload({
      amount: 50,
      paymentType: 'alipay',
      orderType: 'balance',
      origin: 'https://app.example.com',
      isMobile: true,
      isWechatBrowser: false,
      forceQRCode: true,
      mobilePrecreateDeepLink: true,
    })).toMatchObject({
      is_mobile: true,
    })
  })

  it('still passes is_mobile: true when forceQRCode is enabled for non-alipay methods', () => {
    expect(buildCreateOrderPayload({
      amount: 50,
      paymentType: 'wxpay',
      orderType: 'balance',
      origin: 'https://app.example.com',
      isMobile: true,
      isWechatBrowser: false,
      forceQRCode: true,
    })).toMatchObject({
      is_mobile: true,
    })
  })

  it('includes quote_id for local-currency balance orders when provided', () => {
    expect(buildCreateOrderPayload({
      amount: 255000,
      paymentType: 'sepay',
      orderType: 'balance',
      isMobile: false,
      isWechatBrowser: false,
      amountMode: 'payment',
      paymentCurrency: 'VND',
      quoteId: 'quote-vnd-123',
    })).toEqual({
      amount: 255000,
      amount_mode: 'payment',
      payment_currency: 'VND',
      quote_id: 'quote-vnd-123',
      payment_type: 'sepay',
      order_type: 'balance',
      is_mobile: false,
      payment_source: 'hosted_redirect',
    })
  })
})

describe('readPaymentRecoverySnapshot', () => {
  it('restores an unexpired snapshot when the resume token matches', () => {
    const snapshot: PaymentRecoverySnapshot = {
      orderId: 33,
      amount: 18,
      qrCode: '',
      expiresAt: '2099-01-01T00:10:00.000Z',
      paymentType: 'alipay',
      payUrl: 'https://pay.example.com/session/33',
      outTradeNo: 'sub2_33',
      clientSecret: '',
      intentId: '',
      currency: '',
      countryCode: '',
      paymentEnv: '',
      payAmount: 18,
      orderType: 'balance',
      paymentMode: 'popup',
      resumeToken: 'resume-33',
      createdAt: Date.UTC(2099, 0, 1, 0, 0, 0),
    }

    const restored = readPaymentRecoverySnapshot(JSON.stringify(snapshot), {
      now: Date.UTC(2099, 0, 1, 0, 1, 0),
      resumeToken: 'resume-33',
    })

    expect(restored?.orderId).toBe(33)
  })

  it('drops expired or mismatched recovery snapshots', () => {
    const expiredSnapshot: PaymentRecoverySnapshot = {
      orderId: 55,
      amount: 18,
      qrCode: '',
      expiresAt: '2024-01-01T00:10:00.000Z',
      paymentType: 'wxpay',
      payUrl: 'https://pay.example.com/session/55',
      outTradeNo: 'sub2_55',
      clientSecret: '',
      intentId: '',
      currency: '',
      countryCode: '',
      paymentEnv: '',
      payAmount: 18,
      orderType: 'balance',
      paymentMode: 'popup',
      resumeToken: 'resume-55',
      createdAt: Date.UTC(2024, 0, 1, 0, 0, 0),
    }

    expect(readPaymentRecoverySnapshot(JSON.stringify(expiredSnapshot), {
      now: Date.UTC(2024, 0, 1, 0, 20, 0),
      resumeToken: 'resume-55',
    })).toBeNull()

    expect(readPaymentRecoverySnapshot(JSON.stringify({
      ...expiredSnapshot,
      outTradeNo: 'sub2_55',
      expiresAt: '2099-01-01T00:10:00.000Z',
    }), {
      now: Date.UTC(2099, 0, 1, 0, 1, 0),
      resumeToken: 'other-token',
    })).toBeNull()
  })

  it('keeps backward compatibility with snapshots written before outTradeNo existed', () => {
    const restored = readPaymentRecoverySnapshot(JSON.stringify({
      orderId: 44,
      amount: 18,
      qrCode: '',
      expiresAt: '2099-01-01T00:10:00.000Z',
      paymentType: 'alipay',
      payUrl: 'https://pay.example.com/session/44',
      clientSecret: '',
      payAmount: 18,
      orderType: 'balance',
      paymentMode: 'popup',
      resumeToken: 'resume-44',
      createdAt: Date.UTC(2099, 0, 1, 0, 0, 0),
    }), {
      now: Date.UTC(2099, 0, 1, 0, 1, 0),
      resumeToken: 'resume-44',
    })

    expect(restored?.orderId).toBe(44)
    expect(restored?.outTradeNo).toBe('')
  })

  it('keeps backward compatibility with snapshots written before Airwallex fields existed', () => {
    const restored = readPaymentRecoverySnapshot(JSON.stringify({
      orderId: 45,
      amount: 28,
      qrCode: '',
      expiresAt: '2099-01-01T00:10:00.000Z',
      paymentType: 'airwallex',
      payUrl: '/payment/airwallex?order_id=45',
      outTradeNo: 'sub2_45',
      clientSecret: 'awx_cs',
      payAmount: 28,
      orderType: 'balance',
      paymentMode: '',
      resumeToken: 'resume-45',
      createdAt: Date.UTC(2099, 0, 1, 0, 0, 0),
    }), {
      now: Date.UTC(2099, 0, 1, 0, 1, 0),
      resumeToken: 'resume-45',
    })

    expect(restored?.orderId).toBe(45)
    expect(restored?.intentId).toBe('')
    expect(restored?.currency).toBe('')
    expect(restored?.countryCode).toBe('')
    expect(restored?.paymentEnv).toBe('')
  })
})
