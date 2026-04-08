<template>
  <AppLayout>
    <div class="mx-auto max-w-4xl space-y-6">
      <div v-if="loading" class="flex items-center justify-center py-20">
        <div class="h-8 w-8 animate-spin rounded-full border-4 border-primary-500 border-t-transparent"></div>
      </div>
      <template v-else>
        <!-- Balance Card -->
        <div class="card overflow-hidden">
          <div class="bg-gradient-to-br from-primary-500 to-primary-600 px-6 py-6 text-center">
            <p class="text-sm font-medium text-primary-100">{{ t('payment.currentBalance') }}</p>
            <p class="mt-1 text-3xl font-bold text-white">${{ user?.balance?.toFixed(2) || '0.00' }}</p>
          </div>
        </div>
        <!-- Tab Switcher -->
        <div v-if="tabs.length > 1" class="flex space-x-1 rounded-xl bg-gray-100 p-1 dark:bg-dark-800">
          <button v-for="tab in tabs" :key="tab.key"
            class="flex-1 rounded-lg px-4 py-2.5 text-sm font-medium transition-all"
            :class="activeTab === tab.key ? 'bg-white text-gray-900 shadow dark:bg-dark-700 dark:text-white' : 'text-gray-500 hover:text-gray-700 dark:text-gray-400 dark:hover:text-gray-300'"
            @click="activeTab = tab.key">{{ tab.label }}</button>
        </div>
        <!-- Top-up Tab -->
        <template v-if="activeTab === 'recharge'">
          <!-- No payment methods available -->
          <div v-if="enabledMethods.length === 0" class="card py-16 text-center">
            <p class="text-gray-500 dark:text-gray-400">{{ t('payment.notAvailable') }}</p>
          </div>
          <template v-else>
          <div class="card p-6">
            <AmountInput
              v-model="amount"
              :amounts="[10, 20, 50, 100, 200, 500, 1000, 2000, 5000]"
              :min="activeMinAmount"
              :max="activeMaxAmount"
            />
            <p v-if="amountError" class="mt-2 text-xs text-amber-600 dark:text-amber-300">{{ amountError }}</p>
          </div>
          <div v-if="enabledMethods.length >= 1" class="card p-6">
            <PaymentMethodSelector
              :methods="methodOptions"
              :selected="selectedMethod"
              @select="selectedMethod = $event"
            />
          </div>
          <div v-if="feeRate > 0 && validAmount > 0" class="card p-6">
            <div class="space-y-2 text-sm">
              <div class="flex justify-between">
                <span class="text-gray-500 dark:text-gray-400">{{ t('payment.amountLabel') }}</span>
                <span class="text-gray-900 dark:text-white">¥{{ validAmount.toFixed(2) }}</span>
              </div>
              <div class="flex justify-between">
                <span class="text-gray-500 dark:text-gray-400">{{ t('payment.fee') }} ({{ feeRate }}%)</span>
                <span class="text-gray-900 dark:text-white">¥{{ feeAmount.toFixed(2) }}</span>
              </div>
              <div class="flex justify-between border-t border-gray-200 pt-2 dark:border-dark-600">
                <span class="font-medium text-gray-700 dark:text-gray-300">{{ t('payment.actualPay') }}</span>
                <span class="text-lg font-bold text-primary-600 dark:text-primary-400">¥{{ totalAmount.toFixed(2) }}</span>
              </div>
            </div>
          </div>
          <button class="btn btn-primary w-full py-3 text-base font-medium" :disabled="!canSubmit || submitting" @click="handleSubmitRecharge">
            <span v-if="submitting" class="flex items-center justify-center gap-2">
              <span class="h-4 w-4 animate-spin rounded-full border-2 border-white border-t-transparent"></span>
              {{ t('common.processing') }}
            </span>
            <span v-else>{{ t('payment.createOrder') }} ¥{{ (feeRate > 0 && validAmount > 0 ? totalAmount : validAmount).toFixed(2) }}</span>
          </button>
          <div v-if="errorMessage" class="rounded-xl border border-red-200 bg-red-50 p-4 dark:border-red-800/50 dark:bg-red-900/20">
            <p class="text-sm text-red-700 dark:text-red-400">{{ errorMessage }}</p>
          </div>
          </template>
        </template>
        <!-- Subscribe Tab -->
        <template v-else-if="activeTab === 'subscription'">
          <div v-if="checkout.plans.length === 0" class="card py-16 text-center">
            <Icon name="gift" size="xl" class="mx-auto mb-3 text-gray-300 dark:text-dark-600" />
            <p class="text-gray-500 dark:text-gray-400">{{ t('payment.noPlans') }}</p>
          </div>
          <div v-else :class="planGridClass">
            <SubscriptionPlanCard v-for="plan in checkout.plans" :key="plan.id" :plan="plan" @select="openSubscribeDialog" />
          </div>
        </template>
        <div v-if="checkout.help_text || checkout.help_image_url" class="card p-4">
          <div class="flex flex-col items-center gap-3">
            <img v-if="checkout.help_image_url" :src="checkout.help_image_url" alt=""
              class="h-40 max-w-full cursor-pointer rounded-lg object-contain transition-opacity hover:opacity-80"
              @click="previewImage = checkout.help_image_url" />
            <p v-if="checkout.help_text" class="text-center text-sm text-gray-500 dark:text-gray-400">{{ checkout.help_text }}</p>
          </div>
        </div>
      </template>
    </div>
    <!-- Subscription Confirm Dialog -->
    <BaseDialog :show="!!selectedPlan" :title="t('payment.confirmSubscription')" @close="selectedPlan = null">
      <div v-if="selectedPlan" class="space-y-4">
        <div class="rounded-xl border border-gray-100 bg-gray-50 p-5 dark:border-dark-700 dark:bg-dark-800">
          <p class="font-semibold text-gray-900 dark:text-white">{{ selectedPlan.name }}</p>
          <div class="mt-2 flex items-baseline gap-1.5">
            <span class="text-3xl font-extrabold text-primary-600 dark:text-primary-400">&yen;{{ selectedPlan.price }}</span>
            <span v-if="selectedPlan.original_price" class="text-sm text-gray-400 line-through">&yen;{{ selectedPlan.original_price }}</span>
          </div>
          <p v-if="selectedPlan.description" class="mt-2 text-sm text-gray-500 dark:text-dark-400">{{ selectedPlan.description }}</p>
        </div>
        <PaymentMethodSelector
          v-if="enabledMethods.length > 1"
          :methods="methodOptions"
          :selected="selectedMethod"
          @select="selectedMethod = $event"
        />
      </div>
      <template #footer>
        <div class="flex justify-end gap-3">
          <button class="btn btn-secondary" @click="selectedPlan = null">{{ t('common.cancel') }}</button>
          <button class="btn btn-primary" :disabled="submitting" @click="confirmSubscribe">
            {{ submitting ? t('common.processing') : t('payment.createOrder') }}
          </button>
        </div>
      </template>
    </BaseDialog>
    <!-- Inline QR Payment Dialog -->
    <PaymentQRDialog
      :show="qrDialog.show"
      :order-id="qrDialog.orderId"
      :qr-code="qrDialog.qrCode"
      :expires-at="qrDialog.expiresAt"
      :payment-type="qrDialog.paymentType"
      :pay-url="qrDialog.payUrl"
      @close="qrDialog.show = false"
      @success="authStore.refreshUser()"
    />
    <!-- Image Preview Overlay -->
    <Teleport to="body">
      <Transition name="modal">
        <div v-if="previewImage" class="fixed inset-0 z-[60] flex items-center justify-center bg-black/70 backdrop-blur-sm" @click="previewImage = ''">
          <img :src="previewImage" alt="" class="max-h-[85vh] max-w-[90vw] rounded-xl object-contain shadow-2xl" />
        </div>
      </Transition>
    </Teleport>
  </AppLayout>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'
import { useAuthStore } from '@/stores/auth'
import { usePaymentStore } from '@/stores/payment'
import { useAppStore } from '@/stores'
import { paymentAPI } from '@/api/payment'
import { extractApiErrorMessage } from '@/utils/apiError'
import type { SubscriptionPlan, CheckoutInfoResponse } from '@/types/payment'
import AppLayout from '@/components/layout/AppLayout.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import AmountInput from '@/components/payment/AmountInput.vue'
import PaymentMethodSelector from '@/components/payment/PaymentMethodSelector.vue'
import { METHOD_ORDER, POPUP_WINDOW_FEATURES } from '@/components/payment/providerConfig'
import SubscriptionPlanCard from '@/components/payment/SubscriptionPlanCard.vue'
import PaymentQRDialog from '@/components/payment/PaymentQRDialog.vue'
import Icon from '@/components/icons/Icon.vue'
import type { PaymentMethodOption } from '@/components/payment/PaymentMethodSelector.vue'

const { t } = useI18n()
const router = useRouter()
const authStore = useAuthStore()
const paymentStore = usePaymentStore()
const appStore = useAppStore()

const user = computed(() => authStore.user)

const loading = ref(true)
const submitting = ref(false)
const errorMessage = ref('')
const activeTab = ref<'recharge' | 'subscription'>('recharge')
const amount = ref<number | null>(null)
const selectedMethod = ref('')
const selectedPlan = ref<SubscriptionPlan | null>(null)
const previewImage = ref('')

// Inline QR payment dialog state
const qrDialog = ref({ show: false, orderId: 0, qrCode: '', expiresAt: '', paymentType: '', payUrl: '' })

// All checkout data from single API call
const checkout = ref<CheckoutInfoResponse>({
  methods: {}, global_min: 0, global_max: 0,
  plans: [], balance_disabled: false, help_text: '', help_image_url: '', stripe_publishable_key: '',
})

const tabs = computed(() => {
  const result: { key: 'recharge' | 'subscription'; label: string }[] = []
  if (!checkout.value.balance_disabled) result.push({ key: 'recharge', label: t('payment.tabTopUp') })
  result.push({ key: 'subscription', label: t('payment.tabSubscribe') })
  return result
})

const enabledMethods = computed(() => Object.keys(checkout.value.methods))
const validAmount = computed(() => amount.value ?? 0)

// Adaptive grid: center single card, 2-col for 2 plans, 3-col for 3+
const planGridClass = computed(() => {
  const n = checkout.value.plans.length
  if (n === 1) return 'mx-auto grid max-w-sm grid-cols-1 gap-5'
  if (n === 2) return 'mx-auto grid max-w-2xl grid-cols-1 gap-5 sm:grid-cols-2'
  return 'grid grid-cols-1 gap-5 sm:grid-cols-2 lg:grid-cols-3'
})

// Check if an amount fits a method's [min, max]. 0 = no limit.
function amountFitsMethod(amt: number, methodType: string): boolean {
  if (amt <= 0) return true
  const ml = checkout.value.methods[methodType]
  if (!ml) return false
  if (ml.single_min > 0 && amt < ml.single_min) return false
  if (ml.single_max > 0 && amt > ml.single_max) return false
  return true
}

// Amount range: use selected method's limits when available, fallback to global
const activeMinAmount = computed(() => {
  const ml = selectedLimit.value
  return ml?.single_min && ml.single_min > 0 ? ml.single_min : checkout.value.global_min
})
const activeMaxAmount = computed(() => {
  const ml = selectedLimit.value
  return ml?.single_max && ml.single_max > 0 ? ml.single_max : checkout.value.global_max
})

// Selected method's limits (for validation and error messages)
const selectedLimit = computed(() => checkout.value.methods[selectedMethod.value])

const methodOptions = computed<PaymentMethodOption[]>(() =>
  enabledMethods.value.map((type) => {
    const ml = checkout.value.methods[type]
    return {
      type,
      fee_rate: ml?.fee_rate ?? 0,
      available: ml?.available !== false && amountFitsMethod(validAmount.value, type),
    }
  })
)

const feeRate = computed(() => selectedLimit.value?.fee_rate ?? 0)
const feeAmount = computed(() =>
  feeRate.value > 0 && validAmount.value > 0
    ? Math.ceil(((validAmount.value * feeRate.value) / 100) * 100) / 100
    : 0
)
const totalAmount = computed(() =>
  feeRate.value > 0 && validAmount.value > 0
    ? Math.round((validAmount.value + feeAmount.value) * 100) / 100
    : validAmount.value
)

const amountError = computed(() => {
  if (validAmount.value <= 0) return ''
  // No method can handle this amount
  if (!enabledMethods.value.some((m) => amountFitsMethod(validAmount.value, m))) {
    return t('payment.amountNoMethod')
  }
  // Selected method can't handle this amount (but others can)
  const ml = selectedLimit.value
  if (ml) {
    if (ml.single_min > 0 && validAmount.value < ml.single_min) return t('payment.amountTooLow', { min: ml.single_min })
    if (ml.single_max > 0 && validAmount.value > ml.single_max) return t('payment.amountTooHigh', { max: ml.single_max })
  }
  return ''
})

const canSubmit = computed(() =>
  validAmount.value > 0
    && amountFitsMethod(validAmount.value, selectedMethod.value)
    && selectedLimit.value?.available !== false
)

// Auto-switch to first available method when current selection can't handle the amount
watch(() => [validAmount.value, selectedMethod.value] as const, ([amt, method]) => {
  if (amt <= 0 || amountFitsMethod(amt, method)) return
  const available = enabledMethods.value.find((m) => amountFitsMethod(amt, m))
  if (available) selectedMethod.value = available
})

function openSubscribeDialog(plan: SubscriptionPlan) {
  selectedPlan.value = plan
}

async function handleSubmitRecharge() {
  if (!canSubmit.value || submitting.value) return
  await createOrder(validAmount.value, 'balance')
}

async function confirmSubscribe() {
  if (!selectedPlan.value || submitting.value) return
  await createOrder(selectedPlan.value.price, 'subscription', selectedPlan.value.id)
  selectedPlan.value = null
}

async function createOrder(orderAmount: number, orderType: string, planId?: number) {
  submitting.value = true
  errorMessage.value = ''
  try {
    const result = await paymentStore.createOrder({
      amount: orderAmount,
      payment_type: selectedMethod.value,
      order_type: orderType,
      plan_id: planId,
    })
    if (result.client_secret) {
      // Stripe: open in popup window, show waiting dialog on main page
      const stripeUrl = router.resolve({
        path: '/payment/stripe',
        query: { order_id: String(result.order_id), client_secret: result.client_secret },
      }).href
      window.open(stripeUrl, 'paymentPopup', POPUP_WINDOW_FEATURES)
      qrDialog.value = {
        show: true,
        orderId: result.order_id,
        qrCode: '',
        expiresAt: '',
        paymentType: selectedMethod.value,
        payUrl: stripeUrl,
      }
    } else if (result.qr_code) {
      // QR mode: show inline dialog, no page navigation
      qrDialog.value = {
        show: true,
        orderId: result.order_id,
        qrCode: result.qr_code,
        expiresAt: result.expires_at || '',
        paymentType: selectedMethod.value,
        payUrl: '',
      }
    } else if (result.pay_url) {
      // Redirect mode: open in popup window, show waiting dialog on main page
      window.open(result.pay_url, 'paymentPopup', POPUP_WINDOW_FEATURES)
      qrDialog.value = {
        show: true,
        orderId: result.order_id,
        qrCode: '',
        expiresAt: result.expires_at || '',
        paymentType: selectedMethod.value,
        payUrl: result.pay_url,
      }
    } else {
      errorMessage.value = t('payment.result.failed')
      appStore.showError(errorMessage.value)
    }
  } catch (err: unknown) {
    const apiErr = err as Record<string, unknown>
    if (apiErr.reason === 'TOO_MANY_PENDING') {
      const metadata = apiErr.metadata as Record<string, unknown> | undefined
      errorMessage.value = t('payment.errors.tooManyPending', { max: metadata?.max || '' })
    } else if (apiErr.reason === 'CANCEL_RATE_LIMITED') {
      errorMessage.value = t('payment.errors.cancelRateLimited')
    } else {
      errorMessage.value = extractApiErrorMessage(err, t('payment.result.failed'))
    }
    appStore.showError(errorMessage.value)
  } finally {
    submitting.value = false
  }
}

onMounted(async () => {
  try {
    const res = await paymentAPI.getCheckoutInfo()
    checkout.value = res.data
    if (enabledMethods.value.length) {
      const order: readonly string[] = METHOD_ORDER
      const sorted = [...enabledMethods.value].sort((a, b) => {
        const ai = order.indexOf(a)
        const bi = order.indexOf(b)
        return (ai === -1 ? 999 : ai) - (bi === -1 ? 999 : bi)
      })
      selectedMethod.value = sorted[0]
    }
    if (checkout.value.balance_disabled) {
      activeTab.value = 'subscription'
    }
  } catch (err: unknown) { console.error('Failed to load checkout info:', err) }
  finally { loading.value = false }
})
</script>
