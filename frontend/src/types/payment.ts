/**
 * Payment System Type Definitions
 */

// ==================== Enums / Union Types ====================

export type OrderStatus =
  | 'PENDING'
  | 'PAID'
  | 'RECHARGING'
  | 'COMPLETED'
  | 'EXPIRED'
  | 'CANCELLED'
  | 'FAILED'
  | 'REFUND_REQUESTED'
  | 'REFUNDING'
  | 'REFUND_PENDING'
  | 'PARTIALLY_REFUNDED'
  | 'REFUNDED'
  | 'REFUND_FAILED'

export type PaymentType = 'alipay' | 'wxpay' | 'alipay_direct' | 'wxpay_direct' | 'stripe' | 'easypay' | 'airwallex'

export type OrderType = 'balance' | 'subscription'

// ==================== Configuration ====================

export interface PaymentConfig {
  payment_enabled: boolean
  min_amount: number
  max_amount: number
  daily_limit: number
  max_pending_orders: number
  order_timeout_minutes: number
  balance_disabled: boolean
  balance_recharge_multiplier: number
  subscription_usd_to_cny_rate: number
  enabled_payment_types: PaymentType[]
  help_image_url: string
  help_text: string
  stripe_publishable_key: string
}

export interface MethodLimit {
  currency?: string
  display_name?: string
  daily_limit: number
  daily_used: number
  daily_remaining: number
  single_min: number
  single_max: number
  fee_rate: number
  available: boolean
}

/** Response from /payment/limits API */
export interface MethodLimitsResponse {
  methods: Record<string, MethodLimit>
  global_min: number  // widest min across all methods; 0 = no minimum
  global_max: number  // widest max across all methods; 0 = no maximum
}

/** Response from /payment/checkout-info API — single call for the payment page */
export interface CheckoutInfoResponse {
  methods: Record<string, MethodLimit>
  global_min: number
  global_max: number
  plans: SubscriptionPlan[]
  balance_disabled: boolean
  balance_recharge_multiplier: number
  /** Subscription CNY conversion rate (1 USD = X CNY); 0 = disabled, plan price is charged as-is */
  subscription_usd_to_cny_rate: number
  recharge_fee_rate: number
  help_text: string
  help_image_url: string
  stripe_publishable_key: string
  /** When true, Alipay payments on mobile always show the QR code instead of redirecting */
  alipay_force_qrcode?: boolean
  /**
   * 充值赠送活动；后端只在活动开启 + 当前时间在窗口内时下发，否则字段为 undefined。
   * 与 balance_recharge_multiplier 是两个独立概念（一个加余额、一个影响消费），
   * 前端不要把两者乘起来。
   */
  recharge_promo?: RechargePromo
}

/** 一个赠送档位：当 pay_amount ≥ min_amount 时按 bonus_rate 赠送（取最高匹配档）。 */
export interface RechargePromoTier {
  min_amount: number
  bonus_rate: number
}

/** 充值赠送活动配置；前端按 tiers 升序、最高匹配档命中。 */
export interface RechargePromo {
  enabled: boolean
  /** ISO8601 起始时间；缺省（null/undefined）视为无下限。 */
  valid_from?: string | null
  /** ISO8601 截止时间；缺省（null/undefined）视为无上限。 */
  valid_until?: string | null
  tiers: RechargePromoTier[]
  /** 后端计算的稳定 hash；红点 dismiss key 由 (userId, version) 组成。 */
  version: string
}

// ==================== Orders ====================

export interface PaymentOrder {
  id: number
  user_id: number
  amount: number
  pay_amount: number
  currency?: string
  fee_rate: number
  payment_type: string
  out_trade_no: string
  status: OrderStatus
  order_type: OrderType
  created_at: string
  expires_at: string
  paid_at?: string
  completed_at?: string
  refund_amount: number
  refund_reason?: string
  refund_requested_at?: string
  refund_requested_by?: number
  refund_request_reason?: string
  plan_id?: number
  provider_instance_id?: string
}

// ==================== Plans & Channels ====================

export interface SubscriptionPlan {
  id: number
  group_id: number
  group_platform?: string
  group_name?: string
  rate_multiplier?: number
  peak_rate_enabled?: boolean
  peak_start?: string
  peak_end?: string
  peak_rate_multiplier?: number
  daily_limit_usd?: number | null
  weekly_limit_usd?: number | null
  monthly_limit_usd?: number | null
  supported_model_scopes?: string[]
  name: string
  description: string
  price: number
  original_price?: number
  validity_days: number
  validity_unit: string
  /** Stored as JSON string in backend; API layer should parse before use */
  features: string[]
  for_sale: boolean
  sort_order: number
}

export interface PaymentChannel {
  id: number
  group_id?: number
  name: string
  platform: string
  rate_multiplier: number
  description: string
  models: string[]
  features: string[]
  enabled: boolean
}

// ==================== Providers ====================

export interface ProviderInstance {
  id: number
  provider_key: string
  name: string
  config: Record<string, string>
  supported_types: string[]
  enabled: boolean
  payment_mode: string
  refund_enabled: boolean
  allow_user_refund: boolean
  limits: string
  sort_order: number
}

// ==================== Request / Response ====================

export interface CreateOrderRequest {
  amount: number
  payment_type: string
  order_type: string
  plan_id?: number
  return_url?: string
  payment_source?: string
  openid?: string
  wechat_resume_token?: string
  is_mobile?: boolean
  /**
   * 前端在点击"创建订单"瞬间、对当前 amount 计算出的赠送预览金额
   * （与后端 ResolveRechargeBonus 同算法 mirror）。后端用它配合
   * 服务器当前时间二次判窗：用户期待 > 0 但服务端不再发任何赠送
   * → 返回 409 RECHARGE_PROMO_EXPIRED 让前端弹二次确认。
   *
   * 仅在 balance 充值且赠送预览 > 0 时上报；订阅 / 未到档 / 无活动
   * 一律不传或 0。详见 PaymentView submitBalanceWithPromoGuard。
   */
  client_expected_bonus?: number
  /**
   * 用户在"活动已结束"二次确认 modal 上点过"继续充值"，重发请求时
   * 携带 true，后端跳过 promo 拦截。fulfillment 仍按服务器时间核账，
   * 不会因此误发赠送。
   */
  promo_expired_acknowledged?: boolean
}

export type CreateOrderResultType = 'order_created' | 'oauth_required' | 'jsapi_ready'

export interface WechatOAuthInfo {
  authorize_url?: string
  appid?: string
  openid?: string
  scope?: string
  state?: string
  redirect_url?: string
}

export interface WechatJSAPIPayload {
  appId?: string
  timeStamp?: string
  nonceStr?: string
  package?: string
  signType?: string
  paySign?: string
}

export interface CreateOrderResult {
  order_id: number
  amount: number
  pay_url?: string
  qr_code?: string
  client_secret?: string
  intent_id?: string
  currency?: string
  country_code?: string
  payment_env?: string
  pay_amount: number
  fee_rate: number
  expires_at: string
  result_type?: CreateOrderResultType
  payment_type?: string
  out_trade_no?: string
  payment_mode?: string
  resume_token?: string
  oauth?: WechatOAuthInfo
  jsapi?: WechatJSAPIPayload
  jsapi_payload?: WechatJSAPIPayload
}

export interface DashboardStats {
  today_amount: number
  total_amount: number
  today_count: number
  total_count: number
  avg_amount: number
  daily_series: { date: string; amount: number; count: number }[]
  payment_methods: { type: string; amount: number; count: number }[]
  top_users: { user_id: number; email: string; amount: number }[]
}
