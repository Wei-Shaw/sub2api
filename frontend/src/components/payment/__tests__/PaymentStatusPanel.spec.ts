import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'

const pollOrderStatus = vi.hoisted(() => vi.fn())
const cancelOrder = vi.hoisted(() => vi.fn())
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
})
