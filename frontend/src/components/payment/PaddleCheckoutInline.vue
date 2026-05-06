<template>
  <div class="card overflow-hidden p-0">
    <div class="border-b border-gray-100 bg-gradient-to-br from-primary-50 via-white to-yellow-50 p-6 dark:border-dark-700 dark:from-primary-950/30 dark:via-dark-800 dark:to-yellow-950/20">
      <div class="flex flex-col gap-4 sm:flex-row sm:items-start sm:justify-between">
        <div class="flex items-start gap-4">
          <div class="flex h-16 w-16 shrink-0 items-center justify-center rounded-2xl bg-yellow-100 shadow-sm ring-1 ring-yellow-200 dark:bg-yellow-900/30 dark:ring-yellow-800/50">
            <img src="@/assets/icons/paddle.png" alt="Paddle" class="h-9 w-9 rounded-md" />
          </div>
          <div>
            <p class="text-xs font-semibold uppercase tracking-wide text-primary-600 dark:text-primary-300">
              {{ t('payment.hostedCheckout.provider', { provider: t('payment.methods.paddle') }) }}
            </p>
            <h2 class="mt-1 text-xl font-bold text-gray-900 dark:text-white">
              {{ t('payment.hostedCheckout.title') }}
            </h2>
            <p class="mt-1 max-w-2xl text-sm leading-6 text-gray-600 dark:text-gray-300">
              {{ statusMessage }}
            </p>
          </div>
        </div>
        <span class="inline-flex items-center gap-2 rounded-full px-3 py-1 text-xs font-semibold" :class="statusBadgeClass">
          <span class="h-2 w-2 rounded-full" :class="statusDotClass"></span>
          {{ statusLabel }}
        </span>
      </div>
    </div>

    <div class="space-y-5 p-6">
      <div class="rounded-2xl border border-gray-100 bg-gray-50 p-4 dark:border-dark-700 dark:bg-dark-800/70">
        <div class="mb-3 flex items-center justify-between gap-3">
          <h3 class="text-sm font-semibold text-gray-900 dark:text-white">
            {{ t('payment.hostedCheckout.orderSummary') }}
          </h3>
          <span v-if="displayOrderType" class="rounded-full bg-white px-2.5 py-1 text-xs font-medium text-gray-600 ring-1 ring-gray-200 dark:bg-dark-700 dark:text-gray-300 dark:ring-dark-600">
            {{ orderTypeLabel }}
          </span>
        </div>

        <div class="space-y-3 text-sm">
          <div class="flex items-start justify-between gap-4">
            <span class="text-gray-500 dark:text-gray-400">{{ t('payment.hostedCheckout.product') }}</span>
            <span class="text-right font-semibold text-gray-900 dark:text-white">{{ productLabel }}</span>
          </div>
          <div v-if="displayAmount" class="flex items-start justify-between gap-4">
            <span class="text-gray-500 dark:text-gray-400">{{ t('payment.hostedCheckout.amountDue') }}</span>
            <span class="text-right text-lg font-bold text-primary-600 dark:text-primary-300">{{ displayAmount }}</span>
          </div>
          <div v-if="displayCredit" class="flex items-start justify-between gap-4">
            <span class="text-gray-500 dark:text-gray-400">{{ t('payment.hostedCheckout.creditedBalance') }}</span>
            <span class="text-right font-semibold text-gray-900 dark:text-white">{{ displayCredit }}</span>
          </div>
          <div v-if="displayOrderNo" class="flex items-start justify-between gap-4 border-t border-gray-200 pt-3 dark:border-dark-600">
            <span class="text-gray-500 dark:text-gray-400">{{ t('payment.hostedCheckout.orderNo') }}</span>
            <span class="break-all text-right font-mono text-xs font-semibold text-gray-800 dark:text-gray-200">{{ displayOrderNo }}</span>
          </div>
          <div v-if="displayExpiresAt" class="flex items-start justify-between gap-4">
            <span class="text-gray-500 dark:text-gray-400">{{ t('payment.hostedCheckout.expiresAt') }}</span>
            <span class="text-right font-medium text-gray-800 dark:text-gray-200">{{ displayExpiresAt }}</span>
          </div>
        </div>

        <p v-if="!hasOrderDetails" class="mt-3 rounded-xl border border-amber-200 bg-amber-50 px-3 py-2 text-xs text-amber-700 dark:border-amber-800/50 dark:bg-amber-900/20 dark:text-amber-200">
          {{ t('payment.hostedCheckout.missingOrderDetails') }}
        </p>
      </div>

      <div class="rounded-2xl border border-primary-100 bg-primary-50 px-4 py-3 text-sm text-primary-800 dark:border-primary-900/60 dark:bg-primary-950/30 dark:text-primary-200">
        {{ t('payment.hostedCheckout.secureHint') }}
      </div>

      <div
        v-if="errorMessage"
        class="rounded-xl border border-red-200 bg-red-50 p-3 text-sm text-red-700 dark:border-red-800/50 dark:bg-red-900/20 dark:text-red-300"
      >
        {{ errorMessage }}
      </div>

      <div class="flex flex-wrap gap-3">
        <button type="button" class="btn btn-primary flex-1" :disabled="opening || completed" @click="openCheckout">
          <span v-if="opening">{{ t('common.processing') }}</span>
          <span v-else-if="completed">{{ t('payment.result.processing') }}</span>
          <span v-else>{{ checkoutButtonLabel }}</span>
        </button>
        <button type="button" class="btn btn-secondary" @click="emit('back')">{{ t('common.cancel') }}</button>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import type { CurrencyMeta, PaymentOrder, SubscriptionPlan } from '@/types/payment'
import { formatMoney } from '@/utils/money'

declare global {
  interface Window {
    Paddle?: {
      Environment?: { set: (env: string) => void }
      Initialize?: (options: { token: string; eventCallback?: (event: { name?: string }) => void }) => void
      Checkout?: { open: (options: { transactionId: string; settings?: Record<string, unknown> }) => void }
    }
    __paddleInitializedToken?: string
  }
}

const props = withDefaults(defineProps<{
  checkoutId: string
  clientToken: string
  environment?: string
  order?: PaymentOrder | null
  plan?: SubscriptionPlan | null
  currencyMeta?: Record<string, CurrencyMeta>
}>(), {
  environment: '',
  order: null,
  plan: null,
  currencyMeta: () => ({}),
})

const emit = defineEmits<{
  back: []
  completed: []
}>()

const { t } = useI18n()
const opening = ref(false)
const loaded = ref(false)
const errorMessage = ref('')
const completed = ref(false)

const displayOrderType = computed(() => props.order?.order_type || '')
const displayOrderNo = computed(() => props.order?.out_trade_no || '')
const productLabel = computed(() => {
  if (props.plan?.name) return props.plan.name
  if (displayOrderType.value === 'subscription') return t('payment.hostedCheckout.subscriptionProduct')
  return t('payment.hostedCheckout.topUpProduct')
})
const orderTypeLabel = computed(() => {
  if (displayOrderType.value === 'subscription') return t('payment.hostedCheckout.orderTypes.subscription')
  if (displayOrderType.value === 'balance') return t('payment.hostedCheckout.orderTypes.balance')
  return displayOrderType.value
})
const hasOrderDetails = computed(() => Boolean(displayOrderNo.value || props.order?.id || props.order?.pay_amount || props.order?.amount))
const displayAmount = computed(() => {
  const currency = props.order?.payment_currency || props.order?.ledger_currency || 'USD'
  const amount = Number(props.order?.pay_amount || props.order?.payment_amount || props.order?.amount || 0)
  if (!amount) return ''
  return `${formatMoney(amount, currency, props.currencyMeta)} ${currency}`
})
const displayCredit = computed(() => {
  if (displayOrderType.value !== 'balance') return ''
  const currency = props.order?.ledger_currency || 'USD'
  const amount = Number(props.order?.ledger_amount || props.order?.amount || 0)
  if (!amount) return ''
  return `${formatMoney(amount, currency, props.currencyMeta)} ${currency}`
})
const displayExpiresAt = computed(() => formatDateTime(props.order?.expires_at || ''))

const statusLabel = computed(() => {
  if (errorMessage.value) return t('payment.hostedCheckout.status.failed')
  if (completed.value) return t('payment.hostedCheckout.status.completed')
  if (loaded.value) return t('payment.hostedCheckout.status.ready')
  return t('payment.hostedCheckout.status.loading')
})
const statusMessage = computed(() => {
  if (errorMessage.value) return t('payment.result.failed')
  if (completed.value) return t('payment.paddleWaitingWebhook')
  return loaded.value ? t('payment.paddleCheckoutReady') : t('payment.paddleLoading')
})
const statusBadgeClass = computed(() => {
  if (errorMessage.value) return 'bg-red-100 text-red-700 dark:bg-red-900/30 dark:text-red-200'
  if (completed.value) return 'bg-blue-100 text-blue-700 dark:bg-blue-900/30 dark:text-blue-200'
  if (loaded.value) return 'bg-green-100 text-green-700 dark:bg-green-900/30 dark:text-green-200'
  return 'bg-gray-100 text-gray-600 dark:bg-dark-700 dark:text-gray-300'
})
const statusDotClass = computed(() => {
  if (errorMessage.value) return 'bg-red-500'
  if (completed.value) return 'bg-blue-500'
  if (loaded.value) return 'bg-green-500'
  return 'bg-gray-400'
})
const checkoutButtonLabel = computed(() => loaded.value ? t('payment.hostedCheckout.reopenButton') : t('payment.hostedCheckout.openButton'))

onMounted(async () => {
  try {
    await ensurePaddleLoaded()
    loaded.value = true
    await openCheckout()
  } catch (error) {
    errorMessage.value = error instanceof Error ? error.message : t('payment.result.failed')
  }
})

async function openCheckout() {
  if (opening.value || completed.value) return
  errorMessage.value = ''
  opening.value = true
  try {
    await ensurePaddleLoaded()
    const paddle = window.Paddle
    if (!paddle?.Checkout?.open) {
      throw new Error(t('payment.paddleLoadFailed'))
    }
    paddle.Checkout.open({
      transactionId: props.checkoutId,
      settings: {
        displayMode: 'overlay',
        theme: document.documentElement.classList.contains('dark') ? 'dark' : 'light',
      },
    })
  } catch (error) {
    errorMessage.value = error instanceof Error ? error.message : t('payment.paddleLoadFailed')
  } finally {
    opening.value = false
  }
}

async function ensurePaddleLoaded() {
  if (!props.clientToken) {
    throw new Error(t('payment.paddleNotConfigured'))
  }
  if (!window.Paddle) {
    await loadScript('https://cdn.paddle.com/paddle/v2/paddle.js')
  }
  const paddle = window.Paddle
  if (!paddle?.Initialize || !paddle?.Checkout) {
    throw new Error(t('payment.paddleLoadFailed'))
  }
  const env = (props.environment || '').trim().toLowerCase()
  if (env === 'sandbox') {
    paddle.Environment?.set?.('sandbox')
  }
  if (window.__paddleInitializedToken !== props.clientToken) {
    paddle.Initialize({
      token: props.clientToken,
      eventCallback: (event) => {
        if (event?.name === 'checkout.completed') {
          completed.value = true
          emit('completed')
        }
      },
    })
    window.__paddleInitializedToken = props.clientToken
  }
}

function loadScript(src: string) {
  return new Promise<void>((resolve, reject) => {
    const existing = document.querySelector(`script[src="${src}"]`) as HTMLScriptElement | null
    if (existing) {
      if (existing.dataset.loaded === 'true') {
        resolve()
        return
      }
      existing.addEventListener('load', () => resolve(), { once: true })
      existing.addEventListener('error', () => reject(new Error(t('payment.paddleLoadFailed'))), { once: true })
      return
    }
    const script = document.createElement('script')
    script.src = src
    script.async = true
    script.onload = () => {
      script.dataset.loaded = 'true'
      resolve()
    }
    script.onerror = () => reject(new Error(t('payment.paddleLoadFailed')))
    document.head.appendChild(script)
  })
}

function formatDateTime(value: string): string {
  if (!value) return ''
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return value
  return date.toLocaleString(undefined, {
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
  })
}
</script>
