/**
 * User Payment API endpoints (plugin frontend).
 *
 * Routed through the host axios instance (host already injects /api/v1
 * baseURL + auth interceptors). Paths intentionally re-use the
 * pre-migration host prefix /api/v1/payment/* so external integrations
 * and old bookmarks keep working — the host's per-plugin namespace gate
 * recognises /api/v1/<plugin>/ as the payment plugin's own namespace
 * and the plugin's gin engine registers handlers under that prefix.
 */

import { getClient } from './client'
import type {
  PaymentConfig,
  SubscriptionPlan,
  PaymentChannel,
  MethodLimitsResponse,
  CheckoutInfoResponse,
  CreateOrderRequest,
  CreateOrderResult,
  PaymentOrder,
} from '../types/payment'
import type { BasePaginationResponse } from '../types'

const BASE = '/payment'

export const paymentAPI = {
  /** Get payment configuration (enabled types, limits, etc.) */
  getConfig() {
    return getClient().get<PaymentConfig>(`${BASE}/config`)
  },

  /** Get available subscription plans */
  getPlans() {
    return getClient().get<SubscriptionPlan[]>(`${BASE}/plans`)
  },

  /** Get available payment channels (legacy compat — host may not expose). */
  getChannels() {
    return getClient().get<PaymentChannel[]>(`${BASE}/channels`)
  },

  /** Get all checkout page data in a single call */
  getCheckoutInfo() {
    return getClient().get<CheckoutInfoResponse>(`${BASE}/checkout-info`)
  },

  /** Get payment method limits and fee rates */
  getLimits() {
    return getClient().get<MethodLimitsResponse>(`${BASE}/limits`)
  },

  /** Create a new payment order */
  createOrder(data: CreateOrderRequest) {
    return getClient().post<CreateOrderResult>(`${BASE}/orders`, data)
  },

  /** Get current user's orders */
  getMyOrders(params?: { page?: number; page_size?: number; status?: string }) {
    return getClient().get<BasePaginationResponse<PaymentOrder>>(`${BASE}/orders/my`, { params })
  },

  /** Get a specific order by ID */
  getOrder(id: number) {
    return getClient().get<PaymentOrder>(`${BASE}/orders/${id}`)
  },

  /** Cancel a pending order */
  cancelOrder(id: number) {
    return getClient().post(`${BASE}/orders/${id}/cancel`)
  },

  /** Verify order payment status with upstream provider (auth required) */
  verifyOrder(outTradeNo: string) {
    return getClient().post<PaymentOrder>(`${BASE}/orders/verify`, { out_trade_no: outTradeNo })
  },

  /** Legacy-compatible public order lookup by out_trade_no (no auth) */
  verifyOrderPublic(outTradeNo: string) {
    return getClient().post<PaymentOrder>(`${BASE}/public/orders/verify`, { out_trade_no: outTradeNo })
  },

  /** Resolve an order from a signed resume token (no auth) */
  resolveOrderPublicByResumeToken(resumeToken: string) {
    return getClient().post<PaymentOrder>(`${BASE}/public/orders/resolve`, { resume_token: resumeToken })
  },

  /** Request a refund for a completed order */
  requestRefund(id: number, data: { reason: string }) {
    return getClient().post(`${BASE}/orders/${id}/refund-request`, data)
  },

  /** Get provider instance IDs that allow user refund */
  getRefundEligibleProviders() {
    return getClient().get<{ provider_instance_ids: string[] }>(`${BASE}/orders/refund-eligible-providers`)
  },
}
