/**
 * Payment Store
 * Manages payment configuration, current order state, and subscription plans
 */

import { defineStore } from 'pinia'
import { ref } from 'vue'
import { paymentAPI } from '@/api/payment'
import type { PaymentConfig, PaymentOrder, RechargePromo, SubscriptionPlan, CreateOrderRequest } from '@/types/payment'

export const usePaymentStore = defineStore('payment', () => {
  // ==================== State ====================

  /** Payment configuration from backend */
  const config = ref<PaymentConfig | null>(null)
  /** Currently active order (for payment flow) */
  const currentOrder = ref<PaymentOrder | null>(null)
  /** Available subscription plans */
  const plans = ref<SubscriptionPlan[]>([])
  /**
   * 当前生效的充值赠送活动；后端关闭/不在窗口内时为 null。
   * 用于侧边栏菜单红点等需要在 PaymentView 之外感知活动状态的场景。
   */
  const rechargePromo = ref<RechargePromo | null>(null)

  const configLoading = ref(false)
  const configLoaded = ref(false)
  const rechargePromoLoading = ref(false)
  const rechargePromoLoaded = ref(false)

  // ==================== Actions ====================

  /** Fetch payment configuration */
  async function fetchConfig(force = false): Promise<PaymentConfig | null> {
    if (configLoaded.value && !force) return config.value
    if (configLoading.value) return config.value

    configLoading.value = true
    try {
      const response = await paymentAPI.getConfig()
      config.value = response.data
      configLoaded.value = true
      return config.value
    } catch (error: unknown) {
      console.error('[payment] Failed to fetch config:', error)
      return null
    } finally {
      configLoading.value = false
    }
  }

  /** Fetch available subscription plans */
  async function fetchPlans(): Promise<SubscriptionPlan[]> {
    try {
      const response = await paymentAPI.getPlans()
      // Backend returns features as newline-separated string; parse to array
      plans.value = (response.data || []).map((p: Omit<SubscriptionPlan, 'features'> & { features: string | string[] }) => ({
        ...p,
        features: typeof p.features === 'string'
          ? p.features.split('\n').map((f: string) => f.trim()).filter(Boolean)
          : (p.features || []),
      }))
      return plans.value
    } catch (error: unknown) {
      console.error('[payment] Failed to fetch plans:', error)
      return []
    }
  }

  /** Create a new order and set it as current */
  async function createOrder(params: CreateOrderRequest) {
    const response = await paymentAPI.createOrder(params)
    return response.data
  }

  /** Poll order status by ID (read-only, no upstream check) */
  async function pollOrderStatus(orderId: number): Promise<PaymentOrder | null> {
    try {
      const response = await paymentAPI.getOrder(orderId)
      const order = response.data
      if (currentOrder.value?.id === orderId) {
        currentOrder.value = order
      }
      return order
    } catch (error: unknown) {
      console.error('[payment] Failed to poll order status:', error)
      return null
    }
  }

  /** Clear current order state */
  function clearCurrentOrder() {
    currentOrder.value = null
  }

  /**
   * 拉取当前生效的充值赠送活动。
   *
   * 复用 `/payment/checkout-info`（暂未单独抽接口），首次成功后缓存；
   * 调用方（如 sidebar）可重复调用，store 会按 loaded 标志去重。
   * 失败时不抛错——侧边栏红点缺失不应阻断登录流程。
   */
  async function fetchRechargePromo(force = false): Promise<RechargePromo | null> {
    if (rechargePromoLoaded.value && !force) return rechargePromo.value
    if (rechargePromoLoading.value) return rechargePromo.value

    rechargePromoLoading.value = true
    try {
      const response = await paymentAPI.getCheckoutInfo()
      rechargePromo.value = response.data?.recharge_promo ?? null
      rechargePromoLoaded.value = true
      return rechargePromo.value
    } catch (error: unknown) {
      console.error('[payment] Failed to fetch recharge promo:', error)
      return rechargePromo.value
    } finally {
      rechargePromoLoading.value = false
    }
  }

  /** 直接写入活动配置（PaymentView 已自行拉过 checkout-info 时复用，避免重复请求）。 */
  function setRechargePromo(promo: RechargePromo | null | undefined) {
    rechargePromo.value = promo ?? null
    rechargePromoLoaded.value = true
  }

  /** 登出时清空（调用方自行判断时机）。 */
  function resetRechargePromo() {
    rechargePromo.value = null
    rechargePromoLoaded.value = false
    rechargePromoLoading.value = false
  }

  return {
    config,
    currentOrder,
    plans,
    rechargePromo,
    configLoading,
    configLoaded,
    rechargePromoLoading,
    rechargePromoLoaded,
    fetchConfig,
    fetchPlans,
    createOrder,
    pollOrderStatus,
    clearCurrentOrder,
    fetchRechargePromo,
    setRechargePromo,
    resetRechargePromo,
  }
})
