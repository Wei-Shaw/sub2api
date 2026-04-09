<template>
  <div class="flex min-h-screen items-center justify-center bg-gray-50 p-4 dark:bg-dark-900">
    <div class="w-full max-w-sm space-y-4">
      <div class="card overflow-hidden text-center">
        <!-- Amount -->
        <div v-if="amount" class="bg-gradient-to-br from-[#635bff] to-[#4f46e5] px-6 py-5">
          <p class="text-3xl font-bold text-white">&yen;{{ amount }}</p>
          <p v-if="orderId" class="mt-1 text-xs text-indigo-200">{{ t('payment.orders.orderId') }}: {{ orderId }}</p>
        </div>
        <!-- Status -->
        <div class="p-6">
          <!-- Error -->
          <div v-if="error" class="space-y-3">
            <div class="rounded-lg border border-red-200 bg-red-50 p-3 text-sm text-red-600 dark:border-red-800 dark:bg-red-900/30 dark:text-red-400">
              {{ error }}
            </div>
            <button class="text-sm text-gray-500 underline dark:text-gray-400" @click="closeWindow">{{ t('common.close') }}</button>
          </div>
          <!-- Success -->
          <div v-else-if="success" class="space-y-2 py-2">
            <div class="mx-auto flex h-12 w-12 items-center justify-center rounded-full bg-green-100 dark:bg-green-900/30">
              <svg class="h-6 w-6 text-green-500" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2"><path stroke-linecap="round" stroke-linejoin="round" d="M5 13l4 4L19 7" /></svg>
            </div>
            <p class="text-sm font-medium text-green-600 dark:text-green-400">{{ t('payment.result.success') }}</p>
          </div>
          <!-- Loading / Redirecting -->
          <div v-else class="flex items-center justify-center gap-3 py-4">
            <div class="h-6 w-6 animate-spin rounded-full border-3 border-gray-200 border-t-[#635bff] dark:border-dark-600 dark:border-t-[#818cf8]"></div>
            <span class="text-sm text-gray-500 dark:text-gray-400">{{ hint }}</span>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted, onUnmounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute } from 'vue-router'

const { t } = useI18n()
const route = useRoute()

const orderId = String(route.query.order_id || '')
const method = String(route.query.method || 'alipay')
const amount = String(route.query.amount || '')

const error = ref('')
const success = ref(false)
const hint = ref(t('payment.stripePopup.redirecting'))

let pollTimer: ReturnType<typeof setInterval> | null = null

function closeWindow() { window.close() }

onMounted(() => {
  const handler = (event: MessageEvent) => {
    if (event.origin !== window.location.origin) return
    if (event.data?.type !== 'STRIPE_POPUP_INIT') return
    window.removeEventListener('message', handler)
    initStripe(event.data.clientSecret, event.data.publishableKey)
  }
  window.addEventListener('message', handler)

  if (window.opener) {
    window.opener.postMessage({ type: 'STRIPE_POPUP_READY' }, window.location.origin)
  }

  setTimeout(() => {
    if (!error.value && !success.value) {
      error.value = t('payment.stripePopup.timeout')
    }
  }, 15000)
})

onUnmounted(() => {
  if (pollTimer) clearInterval(pollTimer)
})

async function initStripe(clientSecret: string, publishableKey: string) {
  if (!clientSecret || !publishableKey) {
    error.value = t('payment.stripeMissingParams')
    return
  }
  try {
    const { loadStripe } = await import('@stripe/stripe-js')
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
      const result = await (stripe as any).confirmWechatPayPayment(clientSecret, {
        payment_method_options: { wechat_pay: { client: 'web' } },
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
    error.value = err instanceof Error ? err.message : t('payment.stripeLoadFailed')
  }
}

function startPolling() {
  pollTimer = setInterval(async () => {
    try {
      const token = document.cookie.split('; ').find(c => c.startsWith('token='))?.split('=')[1]
        || localStorage.getItem('token') || ''
      const res = await fetch('/api/v1/payment/orders/' + orderId, {
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
    } catch { /* ignore */ }
  }, 3000)
}
</script>
