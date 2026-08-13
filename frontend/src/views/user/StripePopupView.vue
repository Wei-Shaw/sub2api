<template>
  <!--
    The Stripe popup window. It has no app shell, no navigation and exactly one
    job: hold the tab open while Stripe redirects or shows its own QR dialog.

    What it used to do instead: paint the amount in the payment method's brand
    hue (`#00AEEF` / `#07C160` / `#635bff`) via an inline `:style`, and colour the
    spinner and the close link with it too. That gave a bare popup three
    different accents depending on which method the user picked, and it left the
    amount — the one number that matters here — competing with a link. The method
    is now named in words, which is unambiguous in a way a hue never is, and the
    amount is the largest thing on screen because it is the largest, not because
    it is coloured.
  -->
  <div class="flex min-h-screen items-start justify-center bg-canvas p-4 pt-16">
    <div class="w-full max-w-sm space-y-5 rounded border border-line bg-surface p-5">
      <!-- Amount + Order ID -->
      <div v-if="amount">
        <p class="text-2xs font-medium uppercase tracking-[0.04em] text-ink-tertiary">
          {{ methodLabel }}
        </p>
        <p class="mt-1 flex items-baseline gap-1">
          <span class="text-sm text-ink-tertiary">{{ currencySymbolText }}</span>
          <span class="font-mono text-2xl font-semibold tabular-nums slashed-zero text-ink">
            <NumCell :value="amount" :precision="amountPrecision" />
          </span>
        </p>
        <!--
          An order id is an identifier: mono tabular typography, but never
          `NumCell`, which would run `Intl.NumberFormat` over it and print
          `1,234`.
        -->
        <p v-if="orderId" class="mt-1 text-xs text-ink-tertiary">
          {{ t('payment.orders.orderId') }}
          <span class="font-mono tabular-nums slashed-zero text-ink-secondary">{{ orderId }}</span>
        </p>
      </div>

      <!-- Error -->
      <div v-if="error" class="space-y-4">
        <div aria-live="polite">
          <p class="text-2xs font-medium uppercase tracking-[0.08em] text-danger">
            {{ t('payment.result.failed') }}
          </p>
          <p class="mt-2 text-sm text-ink">{{ error }}</p>
        </div>
        <Button variant="outline" size="md" block @click="closeWindow">
          {{ t('common.close') }}
        </Button>
      </div>

      <!-- Success -->
      <div v-else-if="success" class="space-y-4">
        <div aria-live="polite">
          <p class="text-2xs font-medium uppercase tracking-[0.08em] text-success">
            {{ t('payment.status.completed') }}
          </p>
          <p class="mt-2 text-sm font-medium text-ink">{{ t('payment.result.success') }}</p>
        </div>
        <Button variant="outline" size="md" block @click="closeWindow">
          {{ t('common.close') }}
        </Button>
      </div>

      <!-- Loading / Redirecting -->
      <div v-else aria-live="polite" aria-busy="true">
        <p class="flex items-center gap-2 text-2xs font-medium uppercase tracking-[0.08em] text-ink-tertiary">
          <span class="spinner h-3 w-3 shrink-0" aria-hidden="true" />
          {{ t('common.processing') }}
        </p>
        <p class="mt-2 text-sm text-ink-secondary">{{ hint }}</p>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, onMounted, onUnmounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute } from 'vue-router'
import { extractI18nErrorMessage } from '@/utils/apiError'
import { isMobileDevice } from '@/utils/device'
import { buildApiUrl } from '@/api/client'
import {
  currencySymbol,
  paymentCurrencyFractionDigits,
} from '@/components/payment/currency'
// Direct paths, never the `components/common` barrel — it pulls `createI18n`
// into the graph and breaks partial `vue-i18n` factory mocks.
import Button from '@/components/common/Button.vue'
import NumCell from '@/components/common/NumCell.vue'

interface StripeWithWechatPay {
  confirmWechatPayPayment(clientSecret: string, options: Record<string, unknown>): Promise<{ error?: { message?: string }; paymentIntent?: { status: string } }>
}

const { t } = useI18n()
const route = useRoute()

const orderId = String(route.query.order_id || '')
const method = String(route.query.method || 'alipay')
const amountText = String(route.query.amount || '')

/**
 * `currency` used to be absent from this URL and the template hardcoded `¥`, so
 * a Stripe order settled in USD or HKD showed the user a yuan sign on the
 * confirmation screen. The parameter is optional and
 * `normalizePaymentCurrency` falls back to CNY, so old popup URLs still render
 * exactly as before.
 */
const currencyCode = String(route.query.currency || '')
const currencySymbolText = computed(() => currencySymbol(currencyCode))
const amountPrecision = computed(() => paymentCurrencyFractionDigits(currencyCode))

/**
 * `null` rather than `0` when the query param is missing or unparseable:
 * `NumCell` then renders an en dash instead of claiming the charge is zero.
 */
const amount = computed<number | null>(() => {
  const raw = amountText.trim()
  if (!raw) return null
  const parsed = Number(raw)
  return Number.isFinite(parsed) ? parsed : null
})

const methodLabel = computed(() => t(`payment.methods.${method}`, method))

const error = ref('')
const success = ref(false)
const hint = ref(t('payment.stripePopup.redirecting'))

let pollTimer: ReturnType<typeof setInterval> | null = null
let initTimeoutTimer: ReturnType<typeof setTimeout> | null = null
let messageHandler: ((event: MessageEvent) => void) | null = null

function closeWindow() { window.close() }

function clearInitTimeout() {
  if (initTimeoutTimer) {
    clearTimeout(initTimeoutTimer)
    initTimeoutTimer = null
  }
}

onMounted(() => {
  messageHandler = (event: MessageEvent) => {
    if (event.origin !== window.location.origin) return
    if (event.data?.type !== 'STRIPE_POPUP_INIT') return
    // INIT 已到达，取消兜底超时，避免长时间的扫码支付被误判为超时。
    clearInitTimeout()
    if (messageHandler) {
      window.removeEventListener('message', messageHandler)
      messageHandler = null
    }
    initStripe(event.data.clientSecret, event.data.publishableKey)
  }
  window.addEventListener('message', messageHandler)

  if (window.opener) {
    window.opener.postMessage({ type: 'STRIPE_POPUP_READY' }, window.location.origin)
  }

  // 仅兜底“父窗口始终未发 STRIPE_POPUP_INIT”的场景。
  initTimeoutTimer = setTimeout(() => {
    if (!error.value && !success.value) {
      error.value = t('payment.stripePopup.timeout')
    }
  }, 15000)
})

onUnmounted(() => {
  if (pollTimer) { clearInterval(pollTimer); pollTimer = null }
  clearInitTimeout()
  if (messageHandler) {
    window.removeEventListener('message', messageHandler)
    messageHandler = null
  }
})

async function initStripe(clientSecret: string, publishableKey: string) {
  if (!clientSecret || !publishableKey) {
    error.value = t('payment.stripeMissingParams')
    return
  }
  try {
    const { loadStripe } = await import('@stripe/stripe-js/pure')
    const stripe = await loadStripe(publishableKey)
    if (!stripe) { error.value = t('payment.stripeLoadFailed'); return }

    const returnUrl = window.location.origin + '/payment/result?order_id=' + orderId + '&status=success'

    if (method === 'alipay') {
      // Alipay: redirect this popup to Alipay payment page
      const { error: err } = await stripe.confirmAlipayPayment(clientSecret, { return_url: returnUrl })
      if (err) error.value = err.message || t('payment.result.failed')
    } else if (method === 'wechat_pay') {
      // WeChat: Stripe shows its built-in QR dialog, user scans, promise resolves
      hint.value = t('payment.stripePopup.loadingQr')
      const result = await (stripe as unknown as StripeWithWechatPay).confirmWechatPayPayment(clientSecret, {
        payment_method_options: { wechat_pay: { client: isMobileDevice() ? 'mobile_web' : 'web' } },
      })
      if (result.error) {
        error.value = result.error.message || t('payment.result.failed')
      } else if (result.paymentIntent?.status === 'succeeded') {
        success.value = true
        setTimeout(closeWindow, 2000)
      } else {
        // Payment not completed (user closed QR dialog)
        startPolling()
      }
    }
  } catch (err: unknown) {
    error.value = extractI18nErrorMessage(err, t, 'payment.errors', t('payment.stripeLoadFailed'))
  }
}

function startPolling() {
  let inFlight = false
  pollTimer = setInterval(async () => {
    // 防重入：接口响应慢于轮询间隔时避免并发重叠请求。
    if (inFlight) return
    inFlight = true
    try {
      // access token 存储在 localStorage 的 'auth_token' 键下（见 api/client.ts），
      // 之前误读 'token' 导致轮询请求不带认证、永远 401，支付成功无法被检测到。
      const token = localStorage.getItem('auth_token') || ''
      const res = await fetch(buildApiUrl(`/payment/orders/${orderId}`), {
        headers: token ? { Authorization: 'Bearer ' + token } : {},
        credentials: 'include',
      })
      if (!res.ok) return
      const data = await res.json()
      const status = data?.data?.status
      if (status === 'COMPLETED' || status === 'PAID') {
        if (pollTimer) { clearInterval(pollTimer); pollTimer = null }
        success.value = true
        setTimeout(closeWindow, 2000)
      }
    } catch { /* ignore */ } finally {
      inFlight = false
    }
  }, 3000)
}
</script>
