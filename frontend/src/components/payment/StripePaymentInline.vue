<template>
  <div class="space-y-4">
    <div v-if="loading" class="flex items-center justify-center py-12">
      <div class="h-8 w-8 animate-spin rounded-full border-4 border-primary-500 border-t-transparent"></div>
    </div>
    <div v-else-if="initError" class="card p-6 text-center">
      <p class="text-sm text-red-600 dark:text-red-400">{{ initError }}</p>
      <button class="btn btn-secondary mt-4" @click="$emit('back')">{{ t('payment.result.backToRecharge') }}</button>
    </div>
    <template v-else>
      <!-- Amount -->
      <div class="card overflow-hidden">
        <div class="bg-gradient-to-br from-[#635bff] to-[#4f46e5] px-6 py-5 text-center">
          <p class="text-sm font-medium text-indigo-200">{{ t('payment.actualPay') }}</p>
          <p class="mt-1 text-3xl font-bold text-white">&yen;{{ payAmount.toFixed(2) }}</p>
        </div>
      </div>
      <!-- Stripe Payment Element -->
      <div class="card p-6">
        <div ref="stripeMount" class="min-h-[200px]"></div>
        <p v-if="error" class="mt-4 text-sm text-red-600 dark:text-red-400">{{ error }}</p>
        <div v-if="success" class="mt-4 flex items-center gap-2 text-emerald-600 dark:text-emerald-400">
          <Icon name="checkCircle" size="md" />
          <span class="text-sm font-medium">{{ t('payment.stripeSuccessProcessing') }}</span>
        </div>
        <button v-if="!success" class="btn btn-stripe mt-6 w-full py-3 text-base" :disabled="submitting || !ready" @click="handlePay">
          <span v-if="submitting" class="flex items-center justify-center gap-2">
            <span class="h-4 w-4 animate-spin rounded-full border-2 border-white border-t-transparent"></span>
            {{ t('common.processing') }}
          </span>
          <span v-else>{{ t('payment.stripePay') }}</span>
        </button>
      </div>
      <!-- Cancel order -->
      <button v-if="!success" class="btn btn-secondary w-full" :disabled="cancelling" @click="handleCancel">
        {{ cancelling ? t('common.processing') : t('payment.qr.cancelOrder') }}
      </button>
    </template>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted, onUnmounted, nextTick } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'
import { extractApiErrorMessage } from '@/utils/apiError'
import { paymentAPI } from '@/api/payment'
import { useAppStore } from '@/stores'
import { POPUP_WINDOW_FEATURES } from '@/components/payment/providerConfig'
import type { Stripe, StripeElements } from '@stripe/stripe-js'
import Icon from '@/components/icons/Icon.vue'

// Stripe payment methods that require full-page redirect (cannot complete inline)
const REDIRECT_METHODS = new Set(['alipay'])

const props = defineProps<{
  orderId: number
  clientSecret: string
  publishableKey: string
  payAmount: number
}>()

const emit = defineEmits<{ success: []; back: []; redirect: [orderId: number, payUrl: string] }>()

const { t } = useI18n()
const router = useRouter()
const appStore = useAppStore()

const stripeMount = ref<HTMLElement | null>(null)
const loading = ref(true)
const initError = ref('')
const error = ref('')
const submitting = ref(false)
const cancelling = ref(false)
const success = ref(false)
const ready = ref(false)
const selectedType = ref('')

let stripeInstance: Stripe | null = null
let elementsInstance: StripeElements | null = null
let successTimer: ReturnType<typeof setTimeout> | null = null

onMounted(async () => {
  try {
    const { loadStripe } = await import('@stripe/stripe-js')
    const stripe = await loadStripe(props.publishableKey)
    if (!stripe) { initError.value = t('payment.stripeLoadFailed'); return }

    stripeInstance = stripe
    loading.value = false
    await nextTick()
    if (!stripeMount.value) return

    const isDark = document.documentElement.classList.contains('dark')
    const elements = stripe.elements({
      clientSecret: props.clientSecret,
      appearance: { theme: isDark ? 'night' : 'stripe', variables: { borderRadius: '8px' } },
    })
    elementsInstance = elements
    const paymentElement = elements.create('payment', {
      layout: 'tabs',
      paymentMethodOrder: ['alipay', 'wechat_pay', 'card', 'link'],
    } as Record<string, unknown>)
    paymentElement.mount(stripeMount.value)
    paymentElement.on('ready', () => { ready.value = true })
    paymentElement.on('change', (event: { value: { type: string } }) => {
      selectedType.value = event.value.type
    })
  } catch (err: unknown) {
    initError.value = extractApiErrorMessage(err, t('payment.stripeLoadFailed'))
  } finally {
    loading.value = false
  }
})

onUnmounted(() => {
  if (successTimer) clearTimeout(successTimer)
})

async function handlePay() {
  if (!stripeInstance || !elementsInstance || submitting.value) return

  // For redirect-based methods (Alipay): open StripePaymentView in new window,
  // let the redirect happen there instead of navigating the main page away
  if (REDIRECT_METHODS.has(selectedType.value)) {
    const stripeUrl = router.resolve({
      path: '/payment/stripe',
      query: { order_id: String(props.orderId), client_secret: props.clientSecret },
    }).href
    window.open(stripeUrl, 'paymentPopup', POPUP_WINDOW_FEATURES)
    emit('redirect', props.orderId, stripeUrl)
    return
  }

  // For inline methods (card, WeChat popup): confirm directly
  submitting.value = true
  error.value = ''
  try {
    const { error: stripeError } = await stripeInstance.confirmPayment({
      elements: elementsInstance,
      confirmParams: {
        return_url: window.location.origin + '/payment/result?order_id=' + props.orderId + '&status=success',
      },
      redirect: 'if_required',
    })
    if (stripeError) {
      error.value = stripeError.message || t('payment.result.failed')
    } else {
      success.value = true
      successTimer = setTimeout(() => emit('success'), 1500)
    }
  } catch (err: unknown) {
    error.value = extractApiErrorMessage(err, t('payment.result.failed'))
  } finally {
    submitting.value = false
  }
}

async function handleCancel() {
  if (!props.orderId || cancelling.value) return
  cancelling.value = true
  try {
    await paymentAPI.cancelOrder(props.orderId)
    emit('back')
  } catch (err: unknown) {
    appStore.showError(extractApiErrorMessage(err, t('common.error')))
  } finally {
    cancelling.value = false
  }
}
</script>
