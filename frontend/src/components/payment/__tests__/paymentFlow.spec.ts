import { describe, expect, it REDACTED from 'vitest'
import type { CreateOrderResult, MethodLimit REDACTED from '@/types/payment'
import {
  decidePaymentLaunch,
  getVisibleMethods,
  readPaymentRecoverySnapshot,
  type PaymentRecoverySnapshot,
REDACTED from '@/components/payment/paymentFlow'

function methodLimit(overrides: Partial<MethodLimit> = {REDACTED): MethodLimit {
  return {
    daily_limit: 0,
    daily_used: 0,
    daily_remaining: 0,
    single_min: 0,
    single_max: 0,
    fee_rate: 0,
    available: true,
    ...overrides,
  REDACTED
REDACTED

function createOrderResult(overrides: Partial<CreateOrderResult> = {REDACTED): CreateOrderResult {
  return {
    order_id: 101,
    amount: 88,
    pay_amount: 88,
    fee_rate: 0,
    expires_at: '2099-01-01T00:10:00.000Z',
    ...overrides,
  REDACTED
REDACTED

describe('getVisibleMethods', () => {
  it('filters hidden provider methods and normalizes aliases', () => {
    const visible = getVisibleMethods({
      alipay_direct: methodLimit({ single_min: 5 REDACTED),
      wxpay: methodLimit({ single_max: 100 REDACTED),
      stripe: methodLimit({ fee_rate: 3 REDACTED),
    REDACTED)

    expect(visible).toEqual({
      alipay: methodLimit({ single_min: 5 REDACTED),
      wxpay: methodLimit({ single_max: 100 REDACTED),
    REDACTED)
  REDACTED)

  it('prefers canonical visible methods over aliases when both exist', () => {
    const visible = getVisibleMethods({
      alipay: methodLimit({ single_min: 2 REDACTED),
      alipay_direct: methodLimit({ single_min: 9 REDACTED),
      wxpay_direct: methodLimit({ fee_rate: 1.2 REDACTED),
    REDACTED)

    expect(visible.alipay.single_min).toBe(2)
    expect(visible.wxpay.fee_rate).toBe(1.2)
  REDACTED)
REDACTED)

describe('decidePaymentLaunch', () => {
  it('uses Stripe popup waiting flow for desktop Alipay client secret', () => {
    const decision = decidePaymentLaunch(createOrderResult({
      client_secret: 'cs_test',
      resume_token: 'resume-1',
    REDACTED), {
      visibleMethod: 'alipay',
      orderType: 'balance',
      isMobile: false,
    REDACTED)

    expect(decision.kind).toBe('stripe_popup')
    expect(decision.paymentState.paymentType).toBe('alipay')
    expect(decision.stripeMethod).toBe('alipay')
    expect(decision.recovery.resumeToken).toBe('resume-1')
  REDACTED)

  it('uses Stripe route flow for mobile WeChat client secret', () => {
    const decision = decidePaymentLaunch(createOrderResult({
      client_secret: 'cs_test',
    REDACTED), {
      visibleMethod: 'wxpay',
      orderType: 'subscription',
      isMobile: true,
    REDACTED)

    expect(decision.kind).toBe('stripe_route')
    expect(decision.stripeMethod).toBe('wechat_pay')
    expect(decision.paymentState.orderType).toBe('subscription')
  REDACTED)

  it('keeps hosted redirect metadata for recovery flows', () => {
    const decision = decidePaymentLaunch(createOrderResult({
      pay_url: 'https://pay.example.com/session/abc',
      payment_mode: 'popup',
      resume_token: 'resume-2',
    REDACTED), {
      visibleMethod: 'wxpay',
      orderType: 'balance',
      isMobile: false,
    REDACTED)

    expect(decision.kind).toBe('redirect_waiting')
    expect(decision.paymentState.payUrl).toBe('https://pay.example.com/session/abc')
    expect(decision.recovery.paymentMode).toBe('popup')
    expect(decision.recovery.resumeToken).toBe('resume-2')
  REDACTED)
REDACTED)

describe('readPaymentRecoverySnapshot', () => {
  it('restores an unexpired snapshot when the resume token matches', () => {
    const snapshot: PaymentRecoverySnapshot = {
      orderId: 33,
      amount: 18,
      qrCode: '',
      expiresAt: '2099-01-01T00:10:00.000Z',
      paymentType: 'alipay',
      payUrl: 'https://pay.example.com/session/33',
      clientSecret: '',
      payAmount: 18,
      orderType: 'balance',
      paymentMode: 'popup',
      resumeToken: 'resume-33',
      createdAt: Date.UTC(2099, 0, 1, 0, 0, 0),
    REDACTED

    const restored = readPaymentRecoverySnapshot(JSON.stringify(snapshot), {
      now: Date.UTC(2099, 0, 1, 0, 1, 0),
      resumeToken: 'resume-33',
    REDACTED)

    expect(restored?.orderId).toBe(33)
  REDACTED)

  it('drops expired or mismatched recovery snapshots', () => {
    const expiredSnapshot: PaymentRecoverySnapshot = {
      orderId: 55,
      amount: 18,
      qrCode: '',
      expiresAt: '2024-01-01T00:10:00.000Z',
      paymentType: 'wxpay',
      payUrl: 'https://pay.example.com/session/55',
      clientSecret: '',
      payAmount: 18,
      orderType: 'balance',
      paymentMode: 'popup',
      resumeToken: 'resume-55',
      createdAt: Date.UTC(2024, 0, 1, 0, 0, 0),
    REDACTED

    expect(readPaymentRecoverySnapshot(JSON.stringify(expiredSnapshot), {
      now: Date.UTC(2024, 0, 1, 0, 20, 0),
      resumeToken: 'resume-55',
    REDACTED)).toBeNull()

    expect(readPaymentRecoverySnapshot(JSON.stringify({
      ...expiredSnapshot,
      expiresAt: '2099-01-01T00:10:00.000Z',
    REDACTED), {
      now: Date.UTC(2099, 0, 1, 0, 1, 0),
      resumeToken: 'other-token',
    REDACTED)).toBeNull()
  REDACTED)
REDACTED)
