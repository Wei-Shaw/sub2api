import { defineComponent, h } from 'vue'
import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import RechargeSubscriptionView from '../RechargeSubscriptionView.vue'

vi.mock('@/i18n', () => ({
  i18n: {
    global: {
      t: (key: string) => key,
    },
  },
}))

vi.mock('vue-i18n', () => ({
  createI18n: () => ({
    global: {
      locale: { value: 'zh' },
      t: (key: string) => key,
      setLocaleMessage: vi.fn(),
    },
  }),
  useI18n: () => ({
    t: (key: string) =>
      ({
        'rechargeSubscription.title': '充值订阅',
        'rechargeSubscription.description': '统一管理充值、套餐订阅和订单记录',
        'rechargeSubscription.rechargeSection': '充值',
        'rechargeSubscription.subscriptionSection': '订阅',
        'rechargeSubscription.ordersSection': '我的订单',
      })[key] || key,
  }),
}))

const refreshOrders = vi.fn()

describe('RechargeSubscriptionView', () => {
  beforeEach(() => {
    refreshOrders.mockClear()
  })

  it('renders recharge, subscription, and orders as a vertical single page without tabs', () => {
    const wrapper = mount(RechargeSubscriptionView, {
      global: {
        stubs: {
          AppLayout: { template: '<div><slot /></div>' },
          PaymentView: {
            name: 'PaymentView',
            props: ['embedded', 'layoutMode'],
            template: '<section><div data-testid="recharge-panel">充值</div><div data-testid="subscription-panel">订阅</div></section>',
          },
          UserOrdersView: {
            props: ['embedded'],
            template: '<section data-testid="orders-panel">我的订单</section>',
          },
        },
      },
    })

    expect(wrapper.text()).toContain('充值订阅')
    expect(wrapper.findAllComponents({ name: 'PaymentView' })).toHaveLength(1)
    expect(wrapper.get('[data-testid="recharge-panel"]').exists()).toBe(true)
    expect(wrapper.get('[data-testid="subscription-panel"]').exists()).toBe(true)
    expect(wrapper.get('[data-testid="orders-panel"]').exists()).toBe(true)
    expect(wrapper.text()).not.toContain('payment.tabTopUp')
    expect(wrapper.text()).not.toContain('payment.tabSubscribe')
  })

  it('refreshes the embedded orders list when payment completes', async () => {
    const wrapper = mount(RechargeSubscriptionView, {
      global: {
        stubs: {
          AppLayout: { template: '<div><slot /></div>' },
          PaymentView: {
            name: 'PaymentView',
            emits: ['payment-completed'],
            template: '<button data-testid="complete-payment" @click="$emit(\'payment-completed\')">complete</button>',
          },
          UserOrdersView: defineComponent({
            name: 'UserOrdersView',
            props: { embedded: Boolean },
            setup(_, { expose }) {
              expose({ refresh: refreshOrders })
              return () => h('section', { 'data-testid': 'orders-panel' }, '我的订单')
            },
          }),
        },
      },
    })

    await wrapper.get('[data-testid="complete-payment"]').trigger('click')

    expect(refreshOrders).toHaveBeenCalledTimes(1)
  })
})
