import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import AdminOrdersView from '../AdminOrdersView.vue'

const { getOrders, getOrder, rejectRefundRequest, showError, showSuccess } = vi.hoisted(() => ({
  getOrders: vi.fn(),
  getOrder: vi.fn(),
  rejectRefundRequest: vi.fn(),
  showError: vi.fn(),
  showSuccess: vi.fn(),
}))

vi.mock('@/api/admin/payment', () => {
  const adminPaymentAPI = {
    getOrders,
    getOrder,
    cancelOrder: vi.fn(),
    retryRecharge: vi.fn(),
    refundOrder: vi.fn(),
    queryRefund: vi.fn(),
    rejectRefundRequest,
  }
  return { adminPaymentAPI, default: adminPaymentAPI }
})

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({ showError, showSuccess }),
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({ t: (key: string) => key }),
  }
})

const refundRequestedOrder = {
  id: 157,
  user_id: 9,
  user_email: 'refund@example.com',
  user_name: 'refund-user',
  amount: 10,
  pay_amount: 10,
  fee_rate: 0,
  currency: 'CNY',
  out_trade_no: 'sub2_refund_157',
  payment_type: 'alipay',
  payment_trade_no: 'gateway-157',
  order_type: 'balance',
  status: 'REFUND_REQUESTED',
  refund_amount: 10,
  refund_requested_at: '2026-07-30T00:00:00Z',
  refund_request_reason: 'duplicate payment',
  refund_requested_by: 'r:abcdefghijklmnop',
  created_at: '2026-07-30T00:00:00Z',
  updated_at: '2026-07-30T00:00:00Z',
  expires_at: '2026-07-30T01:00:00Z',
}

const OrderTableStub = {
  props: ['orders'],
  template: `
    <div>
      <div v-for="row in orders" :key="row.id">
        <slot name="actions" :row="row" />
      </div>
    </div>
  `,
}

const BaseDialogStub = {
  props: ['show', 'title'],
  emits: ['close'],
  template: `
    <div v-if="show" data-test="base-dialog">
      <div>{{ title }}</div>
      <slot />
      <slot name="footer" />
    </div>
  `,
}

function mountView() {
  return mount(AdminOrdersView, {
    global: {
      stubs: {
        AppLayout: { template: '<div><slot /></div>' },
        OrderTable: OrderTableStub,
        BaseDialog: BaseDialogStub,
        Pagination: true,
        Select: true,
        Icon: true,
        AdminRefundDialog: true,
        OrderStatusBadge: true,
      },
    },
  })
}

describe('AdminOrdersView refund rejection', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    getOrders.mockResolvedValue({
      data: { items: [refundRequestedOrder], total: 1, page: 1, page_size: 20 },
    })
    getOrder.mockResolvedValue({ data: refundRequestedOrder })
    rejectRefundRequest.mockResolvedValue({ data: { message: 'ok' } })
  })

  it('requires a reason and submits the selected refund request', async () => {
    const wrapper = mountView()
    await flushPromises()

    const rejectButton = wrapper.findAll('button').find((button) =>
      button.text().includes('payment.admin.rejectRefund'),
    )
    expect(rejectButton).toBeTruthy()
    await rejectButton!.trigger('click')

    expect(wrapper.text()).toContain('payment.admin.rejectRefundTitle')
    const confirmButton = wrapper.findAll('button').find((button) =>
      button.text().includes('payment.admin.confirmRejectRefund'),
    )
    expect(confirmButton).toBeTruthy()
    await confirmButton!.trigger('click')
    expect(rejectRefundRequest).not.toHaveBeenCalled()
    expect(wrapper.text()).toContain('payment.admin.rejectRefundReasonRequired')

    await wrapper.get('#refund-rejection-reason').setValue('不符合退款条件')
    await confirmButton!.trigger('click')
    await flushPromises()

    expect(rejectRefundRequest).toHaveBeenCalledWith(157, '不符合退款条件')
    expect(showSuccess).toHaveBeenCalledWith('payment.admin.rejectRefundSuccess')
    expect(getOrders).toHaveBeenCalledTimes(2)
  })

  it('shows the requesting user ID instead of the internal refund generation token', async () => {
    const wrapper = mountView()
    await flushPromises()

    const viewButton = wrapper.findAll('button').find((button) =>
      button.text().includes('common.view'),
    )
    expect(viewButton).toBeTruthy()
    await viewButton!.trigger('click')
    await flushPromises()

    expect(wrapper.text()).toContain('#9')
    expect(wrapper.text()).not.toContain('r:abcdefghijklmnop')
  })
})
