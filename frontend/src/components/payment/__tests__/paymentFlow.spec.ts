import { describe, expect, it } from 'vitest'
import type { CreateOrderResult, MethodLimit } from '@/types/payment'
import {
  buildCreateOrderPayload,
  decidePaymentLaunch,
  getVisibleMethods,
  normalizeVisibleMethod,
  readPaymentRecoverySnapshot,
  type PaymentRecoverySnapshot,
} from '@/components/payment/paymentFlow'
import { SEPAY_BANK_TRANSFER, SEPAY_CARD, SEPAY_NAPAS } from '@/components/payment/providerConfig'

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

describe('normalizeVisibleMethod', () => {
  it('keeps each SePay method as its own visible choice', () => {
    // Folding a sub-method onto the gateway key would make the order use a
    // method the user did not press.
    expect(normalizeVisibleMethod(SEPAY_BANK_TRANSFER)).toBe(SEPAY_BANK_TRANSFER)
    expect(normalizeVisibleMethod('  ' + SEPAY_NAPAS + '  ')).toBe(SEPAY_NAPAS)
    expect(normalizeVisibleMethod(SEPAY_CARD)).toBe(SEPAY_CARD)
  })

  it('rejects a bare gateway key and unknown methods', () => {
    expect(normalizeVisibleMethod('sepay')).toBe('')
    expect(normalizeVisibleMethod('alipay')).toBe('')
    expect(normalizeVisibleMethod('')).toBe('')
  })
})

describe('getVisibleMethods', () => {
  it('keeps every configured method separate', () => {
    const visible = getVisibleMethods({
      [SEPAY_BANK_TRANSFER]: methodLimit({ single_max: 100 }),
      [SEPAY_NAPAS]: methodLimit({ single_max: 200 }),
      [SEPAY_CARD]: methodLimit({ single_max: 300 }),
    })

    expect(Object.keys(visible).sort()).toEqual([SEPAY_BANK_TRANSFER, SEPAY_CARD, SEPAY_NAPAS].sort())
    expect(visible[SEPAY_NAPAS].single_max).toBe(200)
  })

  it('passes an unknown method through under its own key', () => {
    const visible = getVisibleMethods({ ldc: methodLimit({ single_max: 50 }) })

    expect(visible.ldc.single_max).toBe(50)
  })

  it('drops blank keys', () => {
    expect(getVisibleMethods({ '  ': methodLimit() })).toEqual({})
  })
})

describe('decidePaymentLaunch', () => {
  it('redirects to the checkout bridge for a form_post result', () => {
    const decision = decidePaymentLaunch(
      createOrderResult({
        result_type: 'form_post',
        pay_url: '/api/v1/payment/checkout?token=abc',
        out_trade_no: 'sub2_1',
        currency: 'VND',
        payment_env: 'sandbox',
      }),
      { visibleMethod: SEPAY_BANK_TRANSFER, orderType: 'balance', isMobile: false, now: 1_000 },
    )

    expect(decision.kind).toBe('redirect_waiting')
    expect(decision.paymentState.payUrl).toBe('/api/v1/payment/checkout?token=abc')
    expect(decision.paymentState.paymentType).toBe(SEPAY_BANK_TRANSFER)
    expect(decision.paymentState.currency).toBe('VND')
    expect(decision.paymentState.paymentEnv).toBe('sandbox')
    expect(decision.paymentState.createdAt).toBe(1_000)
    expect(decision.recovery).toEqual(decision.paymentState)
  })

  it('prefers the redirect even when a QR payload is also present', () => {
    // A form_post checkout cannot be completed by scanning our QR; the bridge
    // page is the only path that reaches the gateway.
    const decision = decidePaymentLaunch(
      createOrderResult({ result_type: 'form_post', pay_url: '/bridge', qr_code: 'qr-payload' }),
      { visibleMethod: SEPAY_CARD, orderType: 'balance', isMobile: false },
    )

    expect(decision.kind).toBe('redirect_waiting')
  })

  it('honours an explicit qrcode payment mode', () => {
    const decision = decidePaymentLaunch(
      createOrderResult({ qr_code: 'qr-payload', pay_url: '/bridge', payment_mode: 'qrcode' }),
      { visibleMethod: SEPAY_BANK_TRANSFER, orderType: 'balance', isMobile: false },
    )

    expect(decision.kind).toBe('qr_waiting')
  })

  it('prefers redirect on mobile when both pay_url and qr_code are present', () => {
    const decision = decidePaymentLaunch(
      createOrderResult({ qr_code: 'qr-payload', pay_url: '/bridge' }),
      { visibleMethod: SEPAY_NAPAS, orderType: 'balance', isMobile: true },
    )

    expect(decision.kind).toBe('redirect_waiting')
  })

  it('keeps the QR flow on desktop when both are present and no mode is set', () => {
    const decision = decidePaymentLaunch(
      createOrderResult({ qr_code: 'qr-payload', pay_url: '/bridge' }),
      { visibleMethod: SEPAY_NAPAS, orderType: 'balance', isMobile: false },
    )

    expect(decision.kind).toBe('qr_waiting')
  })

  it('falls back to the redirect when only a pay_url is available', () => {
    const decision = decidePaymentLaunch(
      createOrderResult({ pay_url: '/bridge', payment_mode: 'qrcode' }),
      { visibleMethod: SEPAY_CARD, orderType: 'balance', isMobile: false },
    )

    expect(decision.kind).toBe('redirect_waiting')
  })

  it('reports an unhandled scenario when the gateway returned nothing usable', () => {
    const decision = decidePaymentLaunch(
      createOrderResult({}),
      { visibleMethod: SEPAY_CARD, orderType: 'balance', isMobile: false },
    )

    expect(decision.kind).toBe('unhandled')
  })
})

describe('buildCreateOrderPayload', () => {
  it('sends the selected method and a canonical result URL', () => {
    expect(buildCreateOrderPayload({
      amount: 250000,
      paymentType: '  ' + SEPAY_NAPAS + '  ',
      orderType: 'balance',
      origin: 'https://panel.example.com/',
      isMobile: false,
    })).toEqual({
      amount: 250000,
      payment_type: SEPAY_NAPAS,
      order_type: 'balance',
      is_mobile: false,
      payment_source: 'hosted_redirect',
      return_url: 'https://panel.example.com/payment/result',
    })
  })

  it('passes the real mobile signal through', () => {
    expect(buildCreateOrderPayload({
      amount: 10000,
      paymentType: SEPAY_CARD,
      orderType: 'balance',
      isMobile: true,
    }).is_mobile).toBe(true)
  })

  it('attaches the plan id for subscription orders and omits a blank return URL', () => {
    const payload = buildCreateOrderPayload({
      amount: 0,
      paymentType: SEPAY_BANK_TRANSFER,
      orderType: 'subscription',
      planId: 7,
      origin: '   ',
      isMobile: false,
    })

    expect(payload.plan_id).toBe(7)
    expect(payload.return_url).toBeUndefined()
  })
})

describe('readPaymentRecoverySnapshot', () => {
  function snapshot(overrides: Partial<PaymentRecoverySnapshot> = {}): PaymentRecoverySnapshot {
    return {
      orderId: 101,
      amount: 250000,
      qrCode: '',
      expiresAt: '2099-01-01T00:10:00.000Z',
      paymentType: SEPAY_BANK_TRANSFER,
      payUrl: '/api/v1/payment/checkout?token=abc',
      outTradeNo: 'sub2_1',
      currency: 'VND',
      paymentEnv: 'sandbox',
      payAmount: 250000,
      orderType: 'balance',
      paymentMode: 'redirect',
      resumeToken: 'resume-1',
      createdAt: 1_000,
      ...overrides,
    }
  }

  it('restores an unexpired snapshot when the resume token matches', () => {
    const restored = readPaymentRecoverySnapshot(JSON.stringify(snapshot()), {
      now: 2_000,
      resumeToken: 'resume-1',
    })

    expect(restored).toEqual(snapshot())
  })

  it('drops an expired snapshot', () => {
    const expired = snapshot({ expiresAt: '2000-01-01T00:00:00.000Z' })

    expect(readPaymentRecoverySnapshot(JSON.stringify(expired), { now: Date.now() })).toBeNull()
  })

  it('drops a snapshot whose resume token does not match the current route', () => {
    expect(readPaymentRecoverySnapshot(JSON.stringify(snapshot()), {
      now: 2_000,
      resumeToken: 'other-token',
    })).toBeNull()
  })

  it('drops malformed or missing payloads', () => {
    expect(readPaymentRecoverySnapshot(null)).toBeNull()
    expect(readPaymentRecoverySnapshot('')).toBeNull()
    expect(readPaymentRecoverySnapshot('not-json')).toBeNull()
    expect(readPaymentRecoverySnapshot(JSON.stringify({ orderId: 'x' }))).toBeNull()
  })

  it('tolerates snapshots written before the currency fields existed', () => {
    const legacy = { ...snapshot() } as Record<string, unknown>
    delete legacy.currency
    delete legacy.paymentEnv
    delete legacy.outTradeNo

    const restored = readPaymentRecoverySnapshot(JSON.stringify(legacy), { now: 2_000 })

    expect(restored?.currency).toBe('')
    expect(restored?.paymentEnv).toBe('')
    expect(restored?.outTradeNo).toBe('')
  })
})
