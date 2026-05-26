/**
 * Admin Payment API endpoints (plugin frontend).
 *
 * Routed through the host axios instance with auth/api-version baseURL.
 * Paths re-use the pre-migration host prefix /api/v1/admin/payment/*;
 * the host's per-plugin namespace gate recognises /api/v1/admin/<plugin>/
 * as the payment plugin's own admin namespace and forwards to the
 * plugin gRPC handler unchanged.
 */

import { getClient } from '../client'
import type {
  DashboardStats,
  PaymentOrder,
  PaymentChannel,
  SubscriptionPlan,
  ProviderInstance,
} from '../../types/payment'
import type { BasePaginationResponse } from '../../types'

const BASE = '/admin/payment'

/**
 * Admin-facing payment config returned by GET /admin/config.
 *
 * Money fields (min_amount / max_amount / daily_limit /
 * balance_recharge_multiplier / recharge_fee_rate) are strings because the
 * Go backend serializes shopspring/decimal as a quoted JSON string. The
 * settings page binds these directly to <input type="number"> via string
 * v-model so they never round-trip through JS Number.
 */
export interface AdminPaymentConfig {
  enabled: boolean
  min_amount: string
  max_amount: string
  daily_limit: string
  order_timeout_minutes: number
  max_pending_orders: number
  enabled_payment_types: string[]
  balance_disabled: boolean
  balance_recharge_multiplier: string
  recharge_fee_rate: string
  load_balance_strategy: string
  product_name_prefix: string
  product_name_suffix: string
  help_image_url: string
  help_text: string

  cancel_rate_limit_enabled: boolean
  cancel_rate_limit_max: number
  cancel_rate_limit_window: number
  cancel_rate_limit_unit: string
  cancel_rate_limit_window_mode: string

  visible_method_alipay_source?: string
  visible_method_wxpay_source?: string
  visible_method_alipay_enabled?: boolean
  visible_method_wxpay_enabled?: boolean
}

/** Fields accepted by PUT /admin/config (all optional). */
export interface UpdatePaymentConfigRequest {
  enabled?: boolean
  min_amount?: string
  max_amount?: string
  daily_limit?: string
  order_timeout_minutes?: number
  max_pending_orders?: number
  enabled_payment_types?: string[]
  balance_disabled?: boolean
  balance_recharge_multiplier?: string
  recharge_fee_rate?: string
  load_balance_strategy?: string
  product_name_prefix?: string
  product_name_suffix?: string
  help_image_url?: string
  help_text?: string

  cancel_rate_limit_enabled?: boolean
  cancel_rate_limit_max?: number
  cancel_rate_limit_window?: number
  cancel_rate_limit_unit?: string
  cancel_rate_limit_window_mode?: string

  visible_method_alipay_source?: string
  visible_method_wxpay_source?: string
  visible_method_alipay_enabled?: boolean
  visible_method_wxpay_enabled?: boolean
}

export const adminPaymentAPI = {
  // ==================== Config ====================
  getConfig() {
    return getClient().get<AdminPaymentConfig>(`${BASE}/config`)
  },
  updateConfig(data: UpdatePaymentConfigRequest) {
    return getClient().put(`${BASE}/config`, data)
  },

  // ==================== Dashboard ====================
  getDashboard(days?: number) {
    return getClient().get<DashboardStats>(`${BASE}/dashboard`, {
      params: days ? { days } : undefined,
    })
  },

  // ==================== Orders ====================
  getOrders(params?: {
    page?: number
    page_size?: number
    status?: string
    payment_type?: string
    user_id?: number
    keyword?: string
    start_date?: string
    end_date?: string
    order_type?: string
  }) {
    return getClient().get<BasePaginationResponse<PaymentOrder>>(`${BASE}/orders`, { params })
  },
  getOrder(id: number) {
    return getClient().get<PaymentOrder>(`${BASE}/orders/${id}`)
  },
  cancelOrder(id: number) {
    return getClient().post(`${BASE}/orders/${id}/cancel`)
  },
  retryRecharge(id: number) {
    return getClient().post(`${BASE}/orders/${id}/retry`)
  },
  refundOrder(
    id: number,
    data: { amount: string; reason: string; deduct_balance?: boolean; force?: boolean },
  ) {
    return getClient().post(`${BASE}/orders/${id}/refund`, data)
  },

  // ==================== Channels (legacy) ====================
  getChannels() {
    return getClient().get<PaymentChannel[]>(`${BASE}/channels`)
  },
  createChannel(data: Partial<PaymentChannel>) {
    return getClient().post<PaymentChannel>(`${BASE}/channels`, data)
  },
  updateChannel(id: number, data: Partial<PaymentChannel>) {
    return getClient().put<PaymentChannel>(`${BASE}/channels/${id}`, data)
  },
  deleteChannel(id: number) {
    return getClient().delete(`${BASE}/channels/${id}`)
  },

  // ==================== Subscription Plans ====================
  getPlans() {
    return getClient().get<SubscriptionPlan[]>(`${BASE}/plans`)
  },
  createPlan(data: Record<string, unknown>) {
    return getClient().post<SubscriptionPlan>(`${BASE}/plans`, data)
  },
  updatePlan(id: number, data: Record<string, unknown>) {
    return getClient().put<SubscriptionPlan>(`${BASE}/plans/${id}`, data)
  },
  deletePlan(id: number) {
    return getClient().delete(`${BASE}/plans/${id}`)
  },

  // ==================== Provider Instances ====================
  getProviders() {
    return getClient().get<ProviderInstance[]>(`${BASE}/providers`)
  },
  createProvider(data: Partial<ProviderInstance>) {
    return getClient().post<ProviderInstance>(`${BASE}/providers`, data)
  },
  updateProvider(id: number, data: Partial<ProviderInstance>) {
    return getClient().put<ProviderInstance>(`${BASE}/providers/${id}`, data)
  },
  deleteProvider(id: number) {
    return getClient().delete(`${BASE}/providers/${id}`)
  },
}

export default adminPaymentAPI
