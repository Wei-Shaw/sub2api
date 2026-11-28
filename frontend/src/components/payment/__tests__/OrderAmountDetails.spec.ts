import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import { createI18n } from 'vue-i18n'
import OrderAmountDetails from '../OrderAmountDetails.vue'
import { orderAmountRows, type OrderAmountSource } from '../orderAmounts'
import { formatPaymentAmount } from '../currency'

const subscription: OrderAmountSource = {
  order_type: 'subscription', amount: 100, pay_amount: 672.98, currency: 'CNY', fee_rate: 0,
  subscription_plan_snapshot: { name: 'Monthly at purchase', currency: 'USD' }, subscription_days: 30,
}
const t = (key: string) => key

describe('order amount semantics', () => {
  it('renders the purchase snapshot and payment separately without any balance credit', () => {
    const wrapper = mount(OrderAmountDetails, {
      props: { order: subscription },
      global: { plugins: [createI18n({ legacy: false, locale: 'en', missingWarn: false, fallbackWarn: false, messages: {} })] },
    })
    expect(wrapper.get('[data-amount="planName"]').text()).toContain('Monthly at purchase')
    expect(wrapper.get('[data-amount="planPrice"]').text()).toContain('$100.00')
    expect(wrapper.get('[data-amount="payAmount"]').text()).toContain(formatPaymentAmount(672.98, 'CNY', 'en'))
    expect(wrapper.find('[data-amount="creditedAmount"]').exists()).toBe(false)
    expect(wrapper.find('[data-amount="baseAmount"]').exists()).toBe(false)
  })

  it('never guesses the plan currency from the gateway for legacy orders', () => {
    const rows = orderAmountRows({ ...subscription, subscription_plan_snapshot: null }, t)
    expect(rows.find(row => row.key === 'planPrice')?.value).toBe('100.00 (payment.orders.currencyNotRecorded)')
    expect(rows.some(row => row.key === 'planName')).toBe(false)
  })

  it('preserves an explicitly empty display currency without inventing USD', () => {
    const rows = orderAmountRows({ ...subscription, subscription_plan_snapshot: { name: 'No label', currency: '' } }, t)
    expect(rows.find(row => row.key === 'planPrice')?.value).toBe('100.00')
  })

  it.each(['CNY', 'HKD', 'JPY'])('uses %s for fees and payment but the snapshot for the plan price', currency => {
    const rows = orderAmountRows({ ...subscription, currency, pay_amount: 103, fee_rate: 3, subscription_plan_snapshot: { name: 'Plan', currency: 'NZD' } }, t, 'en-US')
    expect(rows.find(row => row.key === 'planPrice')?.value).toBe('NZ$100.00')
    expect(rows.find(row => row.key === 'fee')?.value).toBe(formatPaymentAmount(3, currency, 'en-US'))
    expect(rows.find(row => row.key === 'paymentBaseAmount')?.value).toBe(formatPaymentAmount(100, currency, 'en-US'))
  })

  it('keeps recharge bonuses and credits in USD independently of the payment currency', () => {
    const rows = orderAmountRows({ order_type: 'balance', amount: 110, pay_amount: 672.98, currency: 'CNY', bonus_amount: 10 }, t)
    expect(rows.find(row => row.key === 'creditedAmount')?.value).toBe('$110.00')
    expect(rows.find(row => row.key === 'bonusAmount')?.value).toBe('+$10.00')
    expect(rows.some(row => row.key === 'planPrice')).toBe(false)
  })

  it('shows subscription price even if its numeric value equals payment, including compact tables', () => {
    const rows = orderAmountRows({ ...subscription, pay_amount: 100 }, t, 'en-US', true)
    expect(rows.find(row => row.key === 'planPrice')?.value).toBe('$100.00')
    expect(rows.some(row => row.key === 'payAmount')).toBe(false)
  })
})
