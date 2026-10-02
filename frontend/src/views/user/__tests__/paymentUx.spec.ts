import { describe, expect, it } from 'vitest'
import {
  buildPaymentErrorToastMessage,
  describePaymentScenarioError,
  normalizePaymentMethodForDisplay,
  paymentMethodI18nKey,
} from '../paymentUx'

describe('normalizePaymentMethodForDisplay', () => {
  it('keeps each method distinct', () => {
    expect(normalizePaymentMethodForDisplay(' gpmpay_bank_transfer ')).toBe('gpmpay_bank_transfer')
    expect(normalizePaymentMethodForDisplay('NOWPAYMENTS_CRYPTO')).toBe('nowpayments_crypto')
  })

  it('passes an unrecognised method through unchanged', () => {
    expect(normalizePaymentMethodForDisplay('something_else')).toBe('something_else')
    expect(normalizePaymentMethodForDisplay('')).toBe('')
  })

  it('builds the i18n key from the normalised method', () => {
    expect(paymentMethodI18nKey('GPMPAY_BANK_TRANSFER')).toBe('payment.methods.gpmpay_bank_transfer')
  })
})

describe('describePaymentScenarioError', () => {
  it.each([
    'PAYMENT_GATEWAY_ERROR',
    'UNHANDLED_PAYMENT_SCENARIO',
    'NO_AVAILABLE_INSTANCE',
    'PAYMENT_PROVIDER_MISCONFIGURED',
  ])('explains %s as a temporarily unavailable method', (reason) => {
    expect(describePaymentScenarioError(
      { reason },
      { paymentMethod: 'gpmpay_bank_transfer', isMobile: false },
    )).toEqual({
      messageKey: 'payment.errors.methodUnavailable',
      hintKey: 'payment.errors.methodRetryDesktopHint',
    })
  })

  it('gives a mobile-specific hint on mobile', () => {
    expect(describePaymentScenarioError(
      { reason: 'PAYMENT_GATEWAY_ERROR' },
      { paymentMethod: 'nowpayments_crypto', isMobile: true },
    )).toEqual({
      messageKey: 'payment.errors.methodUnavailable',
      hintKey: 'payment.errors.methodRetryMobileHint',
    })
  })

  it('returns null for errors with no gateway-specific story', () => {
    // The caller falls back to the generic API message; claiming the method is
    // unavailable would hide the real reason (e.g. a daily limit).
    expect(describePaymentScenarioError(
      { reason: 'DAILY_LIMIT_EXCEEDED' },
      { paymentMethod: 'gpmpay_bank_transfer', isMobile: false },
    )).toBeNull()
    expect(describePaymentScenarioError(
      new Error('boom'),
      { paymentMethod: 'gpmpay_bank_transfer', isMobile: false },
    )).toBeNull()
  })

  it('returns null when no payment method is known', () => {
    expect(describePaymentScenarioError(
      { reason: 'PAYMENT_GATEWAY_ERROR' },
      { paymentMethod: '  ', isMobile: false },
    )).toBeNull()
  })
})

describe('buildPaymentErrorToastMessage', () => {
  it('returns the main message when no hint is present', () => {
    expect(buildPaymentErrorToastMessage('Payment failed')).toBe('Payment failed')
  })

  it('appends the hint to the toast body when present', () => {
    expect(buildPaymentErrorToastMessage('Payment failed', 'Please try again.')).toBe(
      'Payment failed Please try again.'
    )
  })
})
