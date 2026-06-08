/**
 * Composable: Stripe payment logic for StripePaymentView.
 *
 * Extracted from StripePaymentView.vue to keep the view under 300 lines.
 * Contains Stripe initialization, payment confirmation flows (alipay,
 * wechat_pay, generic Payment Element), light-DOM mount management,
 * and order polling.
 */
import { ref, onUnmounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute, useRouter } from 'vue-router'
import { usePaymentStore } from '../../stores/payment'
import { paymentAPI } from '../../api/payment'
import { extractI18nErrorMessage } from '@sub2api/plugin-sdk'
import { isMobileDevice } from '../../utils/device'
import { normalizePaymentCurrency } from '../../components/payment/currency'
import { PAYMENT_RECOVERY_STORAGE_KEY, readPaymentRecoverySnapshot } from '../../components/payment/paymentFlow'
import type { PaymentOrder } from '../../types/payment'
import type { Stripe, StripeElements } from '@stripe/stripe-js'

export function useStripePayment() {
  const { t } = useI18n()
  const route = useRoute()
  const router = useRouter()
  const paymentStore = usePaymentStore()

  const loading = ref(true)
  const initError = ref('')
  const stripeError = ref('')
  const stripeSubmitting = ref(false)
  const stripeSuccess = ref(false)
  const stripeReady = ref(false)
  const order = ref<PaymentOrder | null>(null)
  const currency = ref('CNY')
  const wechatQrUrl = ref('')
  const redirecting = ref(false)
  const showPaymentElement = ref(false)
  const stripeMountEl = ref<HTMLElement | null>(null)

  let stripeInstance: Stripe | null = null
  let elementsInstance: StripeElements | null = null
  let redirectTimer: ReturnType<typeof setTimeout> | null = null
  let pollTimer: ReturnType<typeof setInterval> | null = null

  // Light-DOM mount for Stripe Elements (Shadow DOM workaround)
  let lightDomMount: HTMLDivElement | null = null
  let placeholderResizeObserver: ResizeObserver | null = null
  let contentResizeObserver: ResizeObserver | null = null

  function syncLightDomMountPosition() {
    if (!lightDomMount || !stripeMountEl.value) return
    const rect = stripeMountEl.value.getBoundingClientRect()
    lightDomMount.style.position = 'fixed'
    lightDomMount.style.top = `${rect.top}px`
    lightDomMount.style.left = `${rect.left}px`
    lightDomMount.style.width = `${rect.width}px`
    lightDomMount.style.zIndex = '10'
  }

  function syncPlaceholderHeight() {
    if (!lightDomMount || !stripeMountEl.value) return
    const measured = lightDomMount.scrollHeight
    if (measured > 0) stripeMountEl.value.style.height = `${measured}px`
  }

  function cleanupLightDomMount() {
    if (placeholderResizeObserver) { placeholderResizeObserver.disconnect(); placeholderResizeObserver = null }
    if (contentResizeObserver) { contentResizeObserver.disconnect(); contentResizeObserver = null }
    window.removeEventListener('resize', syncLightDomMountPosition)
    window.removeEventListener('scroll', syncLightDomMountPosition, true)
    if (lightDomMount) { lightDomMount.remove(); lightDomMount = null }
  }

  function mountPaymentElement(stripe: Stripe, clientSecret: string) {
    if (!stripeMountEl.value) {
      stripeError.value = t('payment.stripeLoadFailed')
      return
    }
    cleanupLightDomMount()
    lightDomMount = document.createElement('div')
    lightDomMount.dataset.stripePaymentMount = '1'
    document.body.appendChild(lightDomMount)
    syncLightDomMountPosition()
    placeholderResizeObserver = new ResizeObserver(syncLightDomMountPosition)
    placeholderResizeObserver.observe(stripeMountEl.value)
    contentResizeObserver = new ResizeObserver(syncPlaceholderHeight)
    contentResizeObserver.observe(lightDomMount)
    window.addEventListener('resize', syncLightDomMountPosition)
    window.addEventListener('scroll', syncLightDomMountPosition, true)

    const isDark = document.documentElement.classList.contains('dark')
    const elements = stripe.elements({
      clientSecret,
      appearance: { theme: isDark ? 'night' : 'stripe', variables: { borderRadius: '8px' } },
    })
    elementsInstance = elements
    const paymentElement = elements.create('payment', {
      layout: 'tabs',
      paymentMethodOrder: ['alipay', 'wechat_pay', 'card', 'link'],
    } as Record<string, unknown>)
    paymentElement.mount(lightDomMount)
    paymentElement.on('ready', () => { stripeReady.value = true })
  }

  async function confirmAlipay(stripe: Stripe, clientSecret: string, orderId: number) {
    redirecting.value = true
    const returnUrl = window.location.origin + '/payment/result?order_id=' + orderId + '&status=success'
    const { error } = await stripe.confirmAlipayPayment(clientSecret, { return_url: returnUrl })
    if (error) {
      redirecting.value = false
      stripeError.value = error.message || t('payment.result.failed')
    }
  }

  async function confirmWechatPay(stripe: Stripe, clientSecret: string) {
    const { paymentIntent, error } = await (stripe as Stripe & {
      confirmWechatPayPayment: (cs: string, opts: Record<string, unknown>) => Promise<{ paymentIntent?: { status: string; next_action?: { wechat_pay_display_qr_code?: { image_data_url?: string } } }; error?: { message?: string } }>
    }).confirmWechatPayPayment(clientSecret, {
      payment_method_options: { wechat_pay: { client: isMobileDevice() ? 'mobile_web' : 'web' } },
    })

    if (error) {
      stripeError.value = error.message || t('payment.result.failed')
      return
    }

    const qrData = paymentIntent?.next_action?.wechat_pay_display_qr_code?.image_data_url
    if (qrData) {
      wechatQrUrl.value = qrData
      startPolling()
    } else if (paymentIntent?.status === 'succeeded') {
      stripeSuccess.value = true
      scheduleClose()
    } else {
      stripeError.value = t('payment.result.failed')
    }
  }

  async function handleGenericPay() {
    if (!stripeInstance || !elementsInstance || stripeSubmitting.value) return
    stripeSubmitting.value = true
    stripeError.value = ''
    try {
      const { error } = await stripeInstance.confirmPayment({
        elements: elementsInstance,
        confirmParams: {
          return_url: window.location.origin + '/payment/result?order_id=' + route.query.order_id + '&status=success',
        },
        redirect: 'if_required',
      })
      if (error) {
        stripeError.value = error.message || t('payment.result.failed')
      } else {
        stripeSuccess.value = true
        scheduleClose()
      }
    } catch (err: unknown) {
      stripeError.value = extractI18nErrorMessage(err, t, 'payment.errors', t('payment.result.failed'))
    } finally {
      stripeSubmitting.value = false
    }
  }

  function startPolling() {
    const orderId = Number(route.query.order_id)
    if (!orderId) return
    pollTimer = setInterval(async () => {
      const o = await paymentStore.pollOrderStatus(orderId)
      if (!o) return
      if (o.status === 'COMPLETED' || o.status === 'PAID') {
        if (pollTimer) { clearInterval(pollTimer); pollTimer = null }
        stripeSuccess.value = true
        wechatQrUrl.value = ''
        scheduleClose()
      }
    }, 3000)
  }

  function scheduleClose() {
    if (window.opener) {
      redirectTimer = setTimeout(() => { window.close() }, 2000)
    } else {
      redirectTimer = setTimeout(() => {
        router.push({ path: '/payment/result', query: { order_id: String(route.query.order_id || ''), status: 'success' } })
      }, 2000)
    }
  }

  async function initStripe() {
    const orderId = Number(route.query.order_id)
    const clientSecret = String(route.query.client_secret || '')
    const method = String(route.query.method || '')
    const resumeToken = typeof route.query.resume_token === 'string' ? route.query.resume_token : undefined

    if (!orderId || !clientSecret) {
      loading.value = false
      initError.value = t('payment.stripeMissingParams')
      return
    }

    try {
      if (typeof window !== 'undefined') {
        const restored = readPaymentRecoverySnapshot(
          window.localStorage.getItem(PAYMENT_RECOVERY_STORAGE_KEY),
          { resumeToken },
        )
        if (restored?.orderId === orderId) {
          currency.value = normalizePaymentCurrency(restored.currency)
        }
      }
      const res = await paymentAPI.getOrder(orderId)
      order.value = res.data
      if (res.data.currency) currency.value = normalizePaymentCurrency(res.data.currency)

      await paymentStore.fetchConfig()
      const publishableKey = paymentStore.config?.stripe_publishable_key
      if (!publishableKey) { initError.value = t('payment.stripeNotConfigured'); return }

      const { loadStripe } = await import('@stripe/stripe-js')
      const stripe = await loadStripe(publishableKey)
      if (!stripe) { initError.value = t('payment.stripeLoadFailed'); return }

      stripeInstance = stripe
      loading.value = false

      if (method === 'alipay') {
        await confirmAlipay(stripe, clientSecret, orderId)
      } else if (method === 'wechat_pay') {
        await confirmWechatPay(stripe, clientSecret)
      } else {
        showPaymentElement.value = true
        // nextTick handled by caller after setting showPaymentElement
      }
    } catch (err: unknown) {
      initError.value = extractI18nErrorMessage(err, t, 'payment.errors', t('payment.stripeLoadFailed'))
    } finally {
      loading.value = false
    }
  }

  onUnmounted(() => {
    if (redirectTimer) clearTimeout(redirectTimer)
    if (pollTimer) clearInterval(pollTimer)
    cleanupLightDomMount()
  })

  return {
    loading,
    initError,
    stripeError,
    stripeSubmitting,
    stripeSuccess,
    stripeReady,
    order,
    currency,
    wechatQrUrl,
    redirecting,
    showPaymentElement,
    stripeMountEl,
    initStripe,
    mountPaymentElement: () => {
      if (stripeInstance) {
        const clientSecret = String(route.query.client_secret || '')
        mountPaymentElement(stripeInstance, clientSecret)
      }
    },
    handleGenericPay,
  }
}
