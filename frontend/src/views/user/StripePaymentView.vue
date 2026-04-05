<template>
  <AppLayout>
    <div class="mx-auto max-w-lg space-y-6 py-8">
      <div v-if="loading" class="flex items-center justify-center py-20">
        <div class="h-8 w-8 animate-spin rounded-full border-4 border-primary-500 border-t-transparent"></div>
      </div>
      <div v-else-if="initError" class="card p-8 text-center">
        <div class="mx-auto mb-4 flex h-16 w-16 items-center justify-center rounded-full bg-red-100 dark:bg-red-900/30">
          <Icon name="exclamationCircle" size="xl" class="text-red-500" />
        </div>
        <h3 class="text-lg font-semibold text-gray-900 dark:text-white">{{ t('payment.stripeLoadFailed') }}</h3>
        <p class="mt-2 text-sm text-gray-500 dark:text-gray-400">{{ initError }}</p>
        <button class="btn btn-primary mt-6" @click="router.push('/purchase')">{{ t('payment.result.backToRecharge') }}</button>
      </div>
      <template v-else>
        <div v-if="order" class="card overflow-hidden">
          <div class="bg-gradient-to-br from-[#635bff] to-[#4f46e5] px-6 py-6 text-center">
            <p class="text-sm font-medium text-indigo-200">{{ t('payment.actualPay') }}</p>
            <p class="mt-1 text-3xl font-bold text-white">&#165;{{ order.pay_amount.toFixed(2) }}</p>
          </div>
        </div>
        <div class="card p-6">
          <div id="stripe-payment-element" class="min-h-[200px]"></div>
          <p v-if="stripeError" class="mt-4 text-sm text-red-600 dark:text-red-400">{{ stripeError }}</p>
          <div v-if="stripeSuccess" class="mt-4 flex items-center gap-2 text-emerald-600 dark:text-emerald-400">
            <Icon name="checkCircle" size="md" />
            <span class="text-sm font-medium">{{ t('payment.stripeSuccessProcessing') }}</span>
          </div>
          <button v-if="!stripeSuccess" class="btn btn-primary mt-6 w-full py-3" :disabled="stripeSubmitting || !stripeReady" @click="handlePay">
            <span v-if="stripeSubmitting" class="flex items-center justify-center gap-2">
              <span class="h-4 w-4 animate-spin rounded-full border-2 border-white border-t-transparent"></span>
              {{ t('common.processing') }}
            </span>
            <span v-else>{{ t('payment.stripePay') }}</span>
          </button>
        </div>
        <div class="text-center">
          <button class="btn btn-secondary" @click="router.push('/purchase')">{{ t('payment.result.backToRecharge') }}</button>
        </div>
      </template>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { ref, onMounted, onUnmounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute, useRouter } from 'vue-router'
import { usePaymentStore } from '@/stores/payment'
import { paymentAPI } from '@/api/payment'
import type { PaymentOrder } from '@/types/payment'
import AppLayout from '@/components/layout/AppLayout.vue'
import Icon from '@/components/icons/Icon.vue'

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

let stripeInstance: any = null
let redirectTimer: ReturnType<typeof setTimeout> | null = null
let elementsInstance: any = null

onMounted(async () => {
  const orderId = Number(route.query.order_id)
  const clientSecret = route.query.client_secret as string

  if (!orderId || !clientSecret) {
    loading.value = false
    initError.value = 'Missing order ID or client secret'
    return
  }

  try {
    const res = await paymentAPI.getOrder(orderId)
    order.value = res.data

    await paymentStore.fetchConfig()
    const publishableKey = paymentStore.config?.stripe_publishable_key
    if (!publishableKey) { initError.value = 'Stripe is not configured'; return }

    const { loadStripe } = await import('@stripe/stripe-js')
    const stripe = await loadStripe(publishableKey)
    if (!stripe) { initError.value = t('payment.stripeLoadFailed'); return }

    stripeInstance = stripe
    const isDark = document.documentElement.classList.contains('dark')
    const elements = stripe.elements({
      clientSecret,
      appearance: { theme: isDark ? 'night' : 'stripe', variables: { borderRadius: '8px' } },
    })
    elementsInstance = elements
    const paymentElement = elements.create('payment')
    paymentElement.mount('#stripe-payment-element')
    paymentElement.on('ready', () => { stripeReady.value = true })
  } catch (err: any) {
    initError.value = err.message || t('payment.stripeLoadFailed')
  } finally {
    loading.value = false
  }
})

onUnmounted(() => {
  if (redirectTimer) clearTimeout(redirectTimer)
})

async function handlePay() {
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
      redirectTimer = setTimeout(() => {
        router.push({ path: '/payment/result', query: { order_id: route.query.order_id as string, status: 'success' } })
      }, 2000)
    }
  } catch (err: any) {
    stripeError.value = err.message || t('payment.result.failed')
  } finally {
    stripeSubmitting.value = false
  }
}
</script>
