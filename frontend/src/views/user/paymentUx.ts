import { normalizeVisibleMethod } from '@/components/payment/paymentFlow'
import { extractApiErrorCode } from '@/utils/apiError'

export interface PaymentScenarioContext {
  paymentMethod: string
  isMobile: boolean
}

export interface PaymentScenarioErrorDescriptor {
  messageKey: string
  hintKey?: string
}

export function normalizePaymentMethodForDisplay(paymentType: string): string {
  const trimmed = paymentType.trim().toLowerCase()
  return normalizeVisibleMethod(trimmed) || trimmed
}

export function paymentMethodI18nKey(paymentType: string): string {
  return `payment.methods.${normalizePaymentMethodForDisplay(paymentType)}`
}

export function buildPaymentErrorToastMessage(message: string, hint?: string): string {
  if (!hint) return message
  return `${message} ${hint}`.trim()
}

/**
 * Map a create-order failure onto a user-facing explanation.
 *
 * Returns null when the error has no gateway-specific story to tell, in which
 * case the caller falls back to the generic API error message.
 */
export function describePaymentScenarioError(
  error: unknown,
  context: PaymentScenarioContext,
): PaymentScenarioErrorDescriptor | null {
  const method = normalizePaymentMethodForDisplay(context.paymentMethod)
  if (!method) return null

  const code = extractApiErrorCode(error)
  if (code !== 'PAYMENT_GATEWAY_ERROR'
    && code !== 'UNHANDLED_PAYMENT_SCENARIO'
    && code !== 'NO_AVAILABLE_INSTANCE'
    && code !== 'PAYMENT_PROVIDER_MISCONFIGURED') {
    return null
  }

  return {
    messageKey: 'payment.errors.methodUnavailable',
    hintKey: context.isMobile
      ? 'payment.errors.methodRetryMobileHint'
      : 'payment.errors.methodRetryDesktopHint',
  }
}
