/**
 * Mobile QR fallback detection for payment methods.
 *
 * When a mobile payment (H5/JSAPI) fails, this checks whether we should
 * fall back to showing a desktop-style QR code instead.
 */
import { isMobileDevice } from '../utils/device'
import { normalizeVisibleMethod } from '../components/payment/paymentFlow'

export function shouldFallbackToDesktopQr(err: unknown, paymentMethod: string, attempted: boolean): boolean {
  if (attempted || !isMobileDevice()) return false
  const normalizedMethod = normalizeVisibleMethod(paymentMethod) || paymentMethod
  const reason = typeof err === 'object' && err && 'reason' in err
    && typeof (err as Record<string, unknown>).reason === 'string'
    ? (err as Record<string, string>).reason : ''
  const message = err instanceof Error
    ? err.message
    : (typeof err === 'object' && err && 'message' in err
        && typeof (err as Record<string, unknown>).message === 'string'
      ? (err as Record<string, string>).message : '')
  const normalizedMessage = message.toLowerCase()
  if (normalizedMethod === 'wxpay') {
    return reason === 'WECHAT_H5_NOT_AUTHORIZED' || reason === 'WECHAT_PAYMENT_MP_NOT_CONFIGURED'
      || reason === 'WECHAT_JSAPI_FAILED' || reason === 'PAYMENT_GATEWAY_ERROR'
      || reason === 'UNHANDLED_PAYMENT_SCENARIO'
      || normalizedMessage.includes('weixinjsbridge is unavailable')
      || normalizedMessage.includes('wechat_jsapi_unavailable')
  }
  if (normalizedMethod === 'alipay') {
    return reason === 'PAYMENT_GATEWAY_ERROR' || reason === 'UNHANDLED_PAYMENT_SCENARIO'
  }
  return false
}