import { describe, expect, it REDACTED from 'vitest'
import {
  buildPaymentErrorToastMessage,
  describePaymentScenarioError,
  normalizePaymentMethodForDisplay,
REDACTED from '../paymentUx'

describe('normalizePaymentMethodForDisplay', () => {
  it('collapses visible payment aliases to canonical method ids', () => {
    expect(normalizePaymentMethodForDisplay(' alipay_direct ')).toBe('alipay')
    expect(normalizePaymentMethodForDisplay('wxpay_direct')).toBe('wxpay')
    expect(normalizePaymentMethodForDisplay('wechat_pay')).toBe('wxpay')
  REDACTED)

  it('leaves non-aliased methods untouched', () => {
    expect(normalizePaymentMethodForDisplay('stripe')).toBe('stripe')
  REDACTED)
REDACTED)

describe('describePaymentScenarioError', () => {
  it('maps WeChat H5 authorization errors to explicit in-app guidance', () => {
    expect(describePaymentScenarioError(
      { reason: 'WECHAT_H5_NOT_AUTHORIZED' REDACTED,
      { paymentMethod: 'wxpay', isMobile: true, isWechatBrowser: false REDACTED,
    )).toEqual({
      messageKey: 'payment.errors.wechatH5NotAuthorized',
      hintKey: 'payment.errors.wechatOpenInWeChatHint',
    REDACTED)
  REDACTED)

  it('maps missing WeixinJSBridge to a JSAPI-specific prompt', () => {
    expect(describePaymentScenarioError(
      new Error('WeixinJSBridge is unavailable'),
      { paymentMethod: 'wxpay', isMobile: true, isWechatBrowser: true REDACTED,
    )).toEqual({
      messageKey: 'payment.errors.wechatJsapiUnavailable',
      hintKey: 'payment.errors.wechatOpenInWeChatHint',
    REDACTED)
  REDACTED)

  it('maps the internal JSAPI unavailable marker to the same prompt', () => {
    expect(describePaymentScenarioError(
      new Error('WECHAT_JSAPI_UNAVAILABLE'),
      { paymentMethod: 'wxpay', isMobile: true, isWechatBrowser: true REDACTED,
    )).toEqual({
      messageKey: 'payment.errors.wechatJsapiUnavailable',
      hintKey: 'payment.errors.wechatOpenInWeChatHint',
    REDACTED)
  REDACTED)

  it('maps generic desktop Alipay failures to QR guidance', () => {
    expect(describePaymentScenarioError(
      { reason: 'PAYMENT_GATEWAY_ERROR' REDACTED,
      { paymentMethod: 'alipay', isMobile: false, isWechatBrowser: false REDACTED,
    )).toEqual({
      messageKey: 'payment.errors.alipayDesktopUnavailable',
      hintKey: 'payment.errors.alipayDesktopQrHint',
    REDACTED)
  REDACTED)
REDACTED)

describe('buildPaymentErrorToastMessage', () => {
  it('returns the main message when no hint is present', () => {
    expect(buildPaymentErrorToastMessage('Payment failed')).toBe('Payment failed')
  REDACTED)

  it('appends the hint to the toast body when present', () => {
    expect(buildPaymentErrorToastMessage('Payment failed', 'Open WeChat to continue.')).toBe(
      'Payment failed Open WeChat to continue.'
    )
  REDACTED)
REDACTED)
