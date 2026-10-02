import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'

const pollOrderStatus = vi.hoisted(() => vi.fn())
const cancelOrder = vi.hoisted(() => vi.fn())
const verifyOrder = vi.hoisted(() => vi.fn())
const showError = vi.hoisted(() => vi.fn())
const toCanvas = vi.hoisted(() => vi.fn())

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string) => key,
    }),
  }
})

vi.mock('@/stores/payment', () => ({
  usePaymentStore: () => ({
    pollOrderStatus,
  }),
}))

vi.mock('@/stores', () => ({
  useAppStore: () => ({
    showError,
  }),
}))

vi.mock('@/api/payment', () => ({
  paymentAPI: {
    cancelOrder,
    verifyOrder,
  },
}))

vi.mock('qrcode', () => ({
  default: {
    toCanvas,
  },
}))

import PaymentStatusPanel from '../PaymentStatusPanel.vue'

const orderFactory = (status: string) => ({
  id: 42,
  user_id: 9,
  amount: 88,
  pay_amount: 88,
  fee_rate: 0,
  payment_type: 'alipay',
  out_trade_no: 'sub2_20260420abcd1234',
  status,
  order_type: 'balance',
  created_at: '2026-04-20T12:00:00Z',
  expires_at: '2099-01-01T12:30:00Z',
  refund_amount: 0,
})

describe('PaymentStatusPanel', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    pollOrderStatus.mockReset()
    cancelOrder.mockReset()
    verifyOrder.mockReset()
    showError.mockReset()
    toCanvas.mockReset().mockResolvedValue(undefined)
  })

  afterEach(() => {
    vi.useRealTimers()
  })

  it('treats RECHARGING as a successful terminal state', async () => {
    pollOrderStatus.mockResolvedValue(orderFactory('RECHARGING'))

    const wrapper = mount(PaymentStatusPanel, {
      props: {
        orderId: 42,
        qrCode: 'https://pay.example.com/qr/42',
        expiresAt: '2099-01-01T12:30:00Z',
        paymentType: 'alipay',
        orderType: 'balance',
      },
      global: {
        stubs: {
          Icon: true,
        },
      },
    })

    await flushPromises()
    await vi.advanceTimersByTimeAsync(3000)
    await flushPromises()

    expect(pollOrderStatus).toHaveBeenCalledWith(42)
    expect(wrapper.text()).toContain('payment.result.success')
    expect(wrapper.emitted('success')).toHaveLength(1)
  })

  it('settles in place when the gateway sends the customer back failed', async () => {
    // The panel lives inside the top-up card and is already polling this order.
    // Sending the customer to a separate result page makes them leave what they
    // are looking at to look at the same thing again.
    pollOrderStatus.mockResolvedValue(orderFactory('PENDING'))

    const wrapper = mount(PaymentStatusPanel, {
      props: {
        orderId: 42,
        qrCode: '',
        payUrl: 'https://pay.example.com/session/42',
        expiresAt: '2099-01-01T12:30:00Z',
        paymentType: 'nowpayments_crypto',
        orderType: 'balance',
        gatewayReturnStatus: 'failed',
      },
      global: { stubs: { Icon: true } },
    })

    await flushPromises()

    expect(wrapper.text()).toContain('payment.qr.expired')
    expect(wrapper.emitted('settled')).toEqual([['expired']])

    // Settled means settled: the poll timer must be gone, not just ignored.
    const callsAfterSettle = pollOrderStatus.mock.calls.length
    await vi.advanceTimersByTimeAsync(9000)
    await flushPromises()
    expect(pollOrderStatus.mock.calls.length).toBe(callsAfterSettle)
  })

  it('shows the cancelled state when the gateway reports a cancelled return', async () => {
    pollOrderStatus.mockResolvedValue(orderFactory('PENDING'))

    const wrapper = mount(PaymentStatusPanel, {
      props: {
        orderId: 42,
        qrCode: '',
        expiresAt: '2099-01-01T12:30:00Z',
        paymentType: 'nowpayments_crypto',
        orderType: 'balance',
        gatewayReturnStatus: 'cancelled',
      },
      global: { stubs: { Icon: true } },
    })

    await flushPromises()

    expect(wrapper.text()).toContain('payment.qr.cancelled')
    expect(wrapper.emitted('settled')).toEqual([['cancelled']])
  })

  it('never lets the gateway declare success on its own', async () => {
    // A customer who edits the return URL must not be able to show themselves
    // a paid order. Only the order row decides that.
    pollOrderStatus.mockResolvedValue(orderFactory('PENDING'))

    const wrapper = mount(PaymentStatusPanel, {
      props: {
        orderId: 42,
        qrCode: '',
        expiresAt: '2099-01-01T12:30:00Z',
        paymentType: 'nowpayments_crypto',
        orderType: 'balance',
        gatewayReturnStatus: 'success',
      },
      global: { stubs: { Icon: true } },
    })

    await flushPromises()

    expect(wrapper.text()).not.toContain('payment.result.success')
    expect(wrapper.emitted('settled')).toBeFalsy()
  })

  it('keeps a paid order paid even when the gateway says it failed', async () => {
    // Rare, but it happens: the IPN lands while the customer is bouncing
    // through the error URL. Closing up shop here would read as lost money.
    pollOrderStatus.mockResolvedValue(orderFactory('COMPLETED'))

    const wrapper = mount(PaymentStatusPanel, {
      props: {
        orderId: 42,
        qrCode: '',
        expiresAt: '2099-01-01T12:30:00Z',
        paymentType: 'nowpayments_crypto',
        orderType: 'balance',
        gatewayReturnStatus: 'failed',
      },
      global: { stubs: { Icon: true } },
    })

    await flushPromises()

    expect(wrapper.text()).toContain('payment.result.success')
    expect(wrapper.emitted('settled')).toEqual([['success']])
  })

  it('shows reopen button in QR mode when payUrl is also available', async () => {
    const openSpy = vi.spyOn(window, 'open').mockReturnValue({ closed: false } as Window)

    const wrapper = mount(PaymentStatusPanel, {
      props: {
        orderId: 42,
        qrCode: 'https://pay.example.com/qr/42',
        payUrl: 'https://pay.example.com/session/42',
        expiresAt: '2099-01-01T12:30:00Z',
        paymentType: 'alipay',
        orderType: 'balance',
      },
      global: {
        stubs: {
          Icon: true,
        },
      },
    })

    await flushPromises()
    expect(wrapper.text()).toContain('payment.qr.openPayWindow')

    await wrapper.findAll('button').find(b => b.text() === 'payment.qr.openPayWindow')!.trigger('click')
    expect(openSpy).toHaveBeenCalledWith(
      'https://pay.example.com/session/42',
      'paymentPopup',
      expect.any(String),
    )

    openSpy.mockRestore()
  })

  it('uses generic QR copy for every payment method', async () => {
    const wrapper = mount(PaymentStatusPanel, {
      props: {
        orderId: 42,
        qrCode: 'https://pay.example.com/qr/42',
        expiresAt: '2099-01-01T12:30:00Z',
        paymentType: 'card_alipay',
        orderType: 'balance',
      },
      global: {
        stubs: {
          Icon: true,
        },
      },
    })

    await flushPromises()

    expect(wrapper.text()).toContain('payment.qr.scanToPay')
    expect(wrapper.text()).not.toContain('payment.qr.scanAlipay')
  })

  it('actively verifies a stuck pending order and settles it when upstream confirms payment', async () => {
    pollOrderStatus.mockResolvedValue(orderFactory('PENDING'))
    verifyOrder.mockResolvedValue({
      data: orderFactory('COMPLETED'),
    })

    const wrapper = mount(PaymentStatusPanel, {
      props: {
        orderId: 42,
        qrCode: 'https://pay.example.com/qr/42',
        expiresAt: '2099-01-01T12:30:00Z',
        paymentType: 'wxpay',
        orderType: 'balance',
      },
      global: {
        stubs: {
          Icon: true,
        },
      },
    })

    await flushPromises()
    await vi.advanceTimersByTimeAsync(3000)
    await flushPromises()

    expect(pollOrderStatus).toHaveBeenCalledWith(42)
    expect(verifyOrder).toHaveBeenCalledWith('sub2_20260420abcd1234')
    expect(wrapper.text()).toContain('payment.result.success')
    expect(wrapper.emitted('success')).toHaveLength(1)
  })



})
