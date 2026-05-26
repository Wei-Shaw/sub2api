import type { LocationQuery, LocationQueryRaw } from 'vue-router'
import type { SubscriptionPlan } from '../../types/payment'
import { normalizeVisibleMethod } from '../../components/payment/paymentFlow'
import { money } from '../../utils/decimal'

export interface ParsedWechatResumeRoute {
  // Decimal string carried through query params; backend's
  // shopspring/decimal accepts any canonical form ("10", "10.0", "10.00").
  orderAmount: string
  orderType: 'balance' | 'subscription'
  paymentType: string
  planId?: number
  openid?: string
  wechatResumeToken?: string
}

function readQueryString(query: LocationQuery, key: string): string {
  const value = query[key]
  if (Array.isArray(value)) {
    return typeof value[0] === 'string' ? value[0] : ''
  }
  return typeof value === 'string' ? value : ''
}

export function hasWechatResumeQuery(query: LocationQuery): boolean {
  if (readQueryString(query, 'wechat_resume') === '1') {
    return true
  }
  return readQueryString(query, 'wechat_resume_token') !== ''
    || readQueryString(query, 'openid') !== ''
}

export function parseWechatResumeRoute(
  query: LocationQuery,
  plans: SubscriptionPlan[],
  // fallback for `balance` orders is supplied by the caller as a Decimal
  // string (e.g. amount.value.toString() from the user's input).
  fallbackBalanceAmount: string,
): ParsedWechatResumeRoute | null {
  if (!hasWechatResumeQuery(query)) {
    return null
  }

  const wechatResumeToken = readQueryString(query, 'wechat_resume_token')
  const paymentType = normalizeVisibleMethod(readQueryString(query, 'payment_type')) || 'wxpay'
  const planId = Number.parseInt(readQueryString(query, 'plan_id'), 10)
  const hasPlanId = Number.isFinite(planId) && planId > 0
  const orderType = readQueryString(query, 'order_type') === 'subscription' || hasPlanId
    ? 'subscription'
    : 'balance'

  if (wechatResumeToken) {
    return {
      wechatResumeToken,
      paymentType,
      orderType,
      orderAmount: '0',
      planId: hasPlanId ? planId : undefined,
    }
  }

  const openid = readQueryString(query, 'openid')
  if (!openid) {
    return null
  }

  const rawAmountStr = readQueryString(query, 'amount').trim()
  const rawAmount = rawAmountStr ? money(rawAmountStr) : null
  const orderAmount = rawAmount && rawAmount.gt(0)
    ? rawAmount.toString()
    : (orderType === 'subscription'
      ? String(plans.find(plan => plan.id === planId)?.price ?? '0')
      : fallbackBalanceAmount)

  return {
    openid,
    paymentType,
    orderType,
    orderAmount,
    planId: hasPlanId ? planId : undefined,
  }
}

export function stripWechatResumeQuery(query: LocationQuery): LocationQueryRaw {
  const nextQuery: LocationQueryRaw = { ...query }
  delete nextQuery.wechat_resume
  delete nextQuery.wechat_resume_token
  delete nextQuery.openid
  delete nextQuery.state
  delete nextQuery.scope
  delete nextQuery.payment_type
  delete nextQuery.amount
  delete nextQuery.order_type
  delete nextQuery.plan_id
  return nextQuery
}
