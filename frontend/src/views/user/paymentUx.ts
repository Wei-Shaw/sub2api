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
 * Turns a create-order failure into a message plus, where we can offer one, a
 * hint about what the payer should do instead.
 *
 * Only failures whose remedy differs from "try again" earn an entry here — a
 * misconfigured gateway is the admin's problem, and telling the payer to retry
 * a channel that cannot work wastes their time.
 */
export function describePaymentScenarioError(
  error: unknown,
  context: PaymentScenarioContext,
): PaymentScenarioErrorDescriptor | null {
  const method = normalizePaymentMethodForDisplay(context.paymentMethod)
  const code = extractApiErrorCode(error)

  if (code === 'PAYMENT_PROVIDER_MISCONFIGURED' || code === 'PAYMENT_GATEWAY_ERROR') {
    return {
      messageKey: 'payment.errors.providerUnavailable',
      hintKey: method === 'sepay'
        ? 'payment.errors.trySepayLaterHint'
        : 'payment.errors.tryOtherMethodHint',
    }
  }

  if (code === 'NO_AVAILABLE_INSTANCE') {
    return {
      messageKey: 'payment.errors.methodNotConfigured',
      hintKey: 'payment.errors.tryOtherMethodHint',
    }
  }

  return null
}
