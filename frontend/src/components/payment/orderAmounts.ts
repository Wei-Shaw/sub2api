import type { PaymentOrder } from '@/types/payment'
import { currencySymbol, formatPaymentAmount } from './currency'

export type OrderAmountSource = Pick<PaymentOrder, 'amount' | 'pay_amount' | 'order_type'> &
  Partial<Pick<PaymentOrder, 'currency' | 'fee_rate' | 'bonus_amount' | 'subscription_plan_snapshot' | 'subscription_days'>>

/** amount is a balance credit for recharge orders, and the purchased plan price for subscriptions. */
export function formatOrderAmount(order: OrderAmountSource, amount = order.amount): string {
  if (order.order_type === 'balance') return `${currencySymbol('USD')}${amount.toFixed(2)}`
  const snapshot = order.subscription_plan_snapshot
  if (!snapshot || !snapshot.currency) return amount.toFixed(2)
  return `${currencySymbol(snapshot.currency)}${amount.toFixed(2)}`
}

export function orderAmountRows(order: OrderAmountSource, t: (key: string) => string, locale?: string, compact = false) {
  const rows: { key: string; label: string; value: string }[] = []
  const add = (key: string, value: string) => rows.push({ key, label: t(`payment.orders.${key}`), value })
  const subscription = order.order_type === 'subscription'
  if (subscription) {
    const snapshot = order.subscription_plan_snapshot
    if (snapshot?.name) add('planName', snapshot.name)
    add('planPrice', formatOrderAmount(order) + (snapshot ? '' : ` (${t('payment.orders.currencyNotRecorded')})`))
    if (order.subscription_days != null) add('subscriptionDays', `${order.subscription_days} ${t('payment.days')}`)
  }
  if (!compact) {
    const rate = Number(order.fee_rate) || 0
    const base = rate > 0 ? order.pay_amount / (1 + rate / 100) : order.pay_amount
    if (!subscription || rate > 0) add(subscription ? 'paymentBaseAmount' : 'baseAmount', formatPaymentAmount(base, order.currency, locale))
    if (rate > 0) add('fee', formatPaymentAmount(order.pay_amount - base, order.currency, locale))
    add('payAmount', formatPaymentAmount(order.pay_amount, order.currency, locale))
  }
  if (order.order_type === 'balance') {
    if ((order.bonus_amount ?? 0) > 0) add('bonusAmount', `+${formatOrderAmount(order, order.bonus_amount)}`)
    if (!compact || order.amount !== order.pay_amount || (order.bonus_amount ?? 0) > 0) add('creditedAmount', formatOrderAmount(order))
  }
  return rows
}
