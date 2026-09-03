import { describe, expect, it } from 'vitest'
import {
  buildPaymentErrorToastMessage,
  describePaymentScenarioError,
  normalizePaymentMethodForDisplay,
  paymentMethodI18nKey,
} from '../paymentUx'

describe('normalizePaymentMethodForDisplay', () => {
  it('keeps each SePay method distinct', () => {
    expect(normalizePaymentMethodForDisplay(' sepay_bank_transfer ')).toBe('sepay_bank_transfer')
    expect(normalizePaymentMethodForDisplay('SEPAY_NAPAS')).toBe('sepay_napas')
    expect(normalizePaymentMethodForDisplay('sepay_card')).toBe('sepay_card')
  })

  it('passes an unrecognised method through unchanged', () => {
    expect(normalizePaymentMethodForDisplay('something_else')).toBe('something_else')
    expect(normalizePaymentMethodForDisplay('')).toBe('')
  })

  it('builds the i18n key from the normalised method', () => {
    expect(paymentMethodI18nKey('SEPAY_CARD')).toBe('payment.methods.sepay_card')
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
      { paymentMethod: 'sepay_bank_transfer', isMobile: false },
    )).toEqual({
      messageKey: 'payment.errors.methodUnavailable',
      hintKey: 'payment.errors.methodRetryDesktopHint',
    })
  })

  it('gives a mobile-specific hint on mobile', () => {
    expect(describePaymentScenarioError(
      { reason: 'PAYMENT_GATEWAY_ERROR' },
      { paymentMethod: 'sepay_card', isMobile: true },
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
      { paymentMethod: 'sepay_napas', isMobile: false },
    )).toBeNull()
    expect(describePaymentScenarioError(
      new Error('boom'),
      { paymentMethod: 'sepay_napas', isMobile: false },
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
