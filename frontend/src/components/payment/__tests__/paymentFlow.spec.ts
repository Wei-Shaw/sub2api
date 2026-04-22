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
  it('filters hidden provider methods and normalizes aliases', () => {
    const visible = getVisibleMethods({
      alipay_direct: methodLimit({ single_min: 5 }),
      wxpay: methodLimit({ single_max: 100 }),
      stripe: methodLimit({ fee_rate: 3 }),
    })

    expect(visible).toEqual({
      alipay: methodLimit({ single_min: 5 }),
      wxpay: methodLimit({ single_max: 100 }),
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
})

describe('buildCreateOrderPayload', () => {
  it('normalizes visible method aliases and attaches a canonical result URL', () => {
    expect(buildCreateOrderPayload({
      amount: 88,
      paymentType: 'alipay_direct',
      orderType: 'balance',
      origin: 'https://app.example.com/',
      isWechatBrowser: false,
    })).toEqual({
      amount: 88,
      payment_type: 'alipay',
      order_type: 'balance',
      return_url: 'https://app.example.com/payment/result',
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
      isWechatBrowser: true,
    })).toEqual({
      amount: 128,
      payment_type: 'wxpay',
      order_type: 'subscription',
      plan_id: 7,
      return_url: 'https://app.example.com/payment/result',
      payment_source: 'wechat_in_app_resume',
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
})
