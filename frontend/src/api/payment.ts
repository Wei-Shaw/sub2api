/**
 * User Payment API endpoints
 * Handles payment operations for regular users
 */

import { apiClient REDACTED from './client'
import type {
  PaymentConfig,
  SubscriptionPlan,
  PaymentChannel,
  MethodLimitsResponse,
  CheckoutInfoResponse,
  CreateOrderRequest,
  CreateOrderResult,
  PaymentOrder
REDACTED from '@/types/payment'
import type { BasePaginationResponse REDACTED from '@/types'

export interface PublicOrderVerifyResult {
  out_trade_no: string
  status: string
  paid: boolean
  created_at: string
  expires_at: string
REDACTED

export const paymentAPI = {
  /** Get payment configuration (enabled types, limits, etc.) */
  getConfig() {
    return apiClient.get<PaymentConfig>('/payment/config')
  REDACTED,

  /** Get available subscription plans */
  getPlans() {
    return apiClient.get<SubscriptionPlan[]>('/payment/plans')
  REDACTED,

  /** Get available payment channels */
  getChannels() {
    return apiClient.get<PaymentChannel[]>('/payment/channels')
  REDACTED,

  /** Get all checkout page data in a single call */
  getCheckoutInfo() {
    return apiClient.get<CheckoutInfoResponse>('/payment/checkout-info')
  REDACTED,

  /** Get payment method limits and fee rates */
  getLimits() {
    return apiClient.get<MethodLimitsResponse>('/payment/limits')
  REDACTED,

  /** Create a new payment order */
  createOrder(data: CreateOrderRequest) {
    return apiClient.post<CreateOrderResult>('/payment/orders', data)
  REDACTED,

  /** Get current user's orders */
  getMyOrders(params?: { page?: number; page_size?: number; status?: string REDACTED) {
    return apiClient.get<BasePaginationResponse<PaymentOrder>>('/payment/orders/my', { params REDACTED)
  REDACTED,

  /** Get a specific order by ID */
  getOrder(id: number) {
    return apiClient.get<PaymentOrder>(`/payment/orders/${idREDACTED`)
  REDACTED,

  /** Cancel a pending order */
  cancelOrder(id: number) {
    return apiClient.post(`/payment/orders/${idREDACTED/cancel`)
  REDACTED,

  /** Verify order payment status with upstream provider */
  verifyOrder(outTradeNo: string) {
    return apiClient.post<PaymentOrder>('/payment/orders/verify', { out_trade_no: outTradeNo REDACTED)
  REDACTED,

  /** Legacy-compatible public order lookup by out_trade_no */
  verifyOrderPublic(outTradeNo: string) {
    return apiClient.post<PublicOrderVerifyResult>('/payment/public/orders/verify', { out_trade_no: outTradeNo REDACTED)
  REDACTED,

  /** Resolve an order from a signed resume token without auth */
  resolveOrderPublicByResumeToken(resumeToken: string) {
    return apiClient.post<PublicOrderVerifyResult>('/payment/public/orders/resolve', { resume_token: resumeToken REDACTED)
  REDACTED,

  /** Request a refund for a completed order */
  requestRefund(id: number, data: { reason: string REDACTED) {
    return apiClient.post(`/payment/orders/${idREDACTED/refund-request`, data)
  REDACTED,

  /** Get provider instance IDs that allow user refund */
  getRefundEligibleProviders() {
    return apiClient.get<{ provider_instance_ids: string[] REDACTED>('/payment/orders/refund-eligible-providers')
  REDACTED
REDACTED
