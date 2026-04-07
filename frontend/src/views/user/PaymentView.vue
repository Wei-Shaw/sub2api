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
              :amounts="[10, 50, 100, 200, 500, 1000]"
              :min="minAmount"
              :max="maxAmount"
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
          <div v-if="plansLoading" class="flex items-center justify-center py-20">
            <div class="h-8 w-8 animate-spin rounded-full border-4 border-primary-500 border-t-transparent"></div>
          </div>
          <div v-else-if="plans.length === 0" class="py-16 text-center text-gray-500 dark:text-gray-400">{{ t('payment.noPlans') }}</div>
          <div v-else class="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3">
            <SubscriptionPlanCard v-for="plan in plans" :key="plan.id" :plan="plan" @select="openSubscribeDialog" />
          </div>
        </template>
        <div v-if="config?.help_text" class="card p-4">
          <p class="text-sm text-gray-500 dark:text-gray-400">{{ config.help_text }}</p>
        </div>
      </template>
    </div>
    <!-- Subscription Confirm Dialog -->
    <BaseDialog :show="!!selectedPlan" :title="t('payment.confirmSubscription')" @close="selectedPlan = null">
      <div v-if="selectedPlan" class="space-y-4">
        <div class="rounded-xl bg-gray-50 p-4 dark:bg-dark-800">
          <p class="text-sm font-medium text-gray-900 dark:text-white">{{ selectedPlan.name }}</p>
          <p class="mt-1 text-2xl font-bold text-primary-600 dark:text-primary-400">¥{{ selectedPlan.price }}</p>
          <p v-if="selectedPlan.description" class="mt-1 text-xs text-gray-500 dark:text-dark-400">{{ selectedPlan.description }}</p>
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
import type { SubscriptionPlan, MethodLimit } from '@/types/payment'
import AppLayout from '@/components/layout/AppLayout.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import AmountInput from '@/components/payment/AmountInput.vue'
import PaymentMethodSelector from '@/components/payment/PaymentMethodSelector.vue'
import SubscriptionPlanCard from '@/components/payment/SubscriptionPlanCard.vue'
import type { PaymentMethodOption } from '@/components/payment/PaymentMethodSelector.vue'

const { t } = useI18n()
const router = useRouter()
const authStore = useAuthStore()
const paymentStore = usePaymentStore()
const appStore = useAppStore()

const user = computed(() => authStore.user)
const config = computed(() => paymentStore.config)
const plans = computed(() => paymentStore.plans)

const loading = ref(true)
const plansLoading = ref(false)
const submitting = ref(false)
const errorMessage = ref('')
const activeTab = ref<'recharge' | 'subscription'>('recharge')
const amount = ref<number | null>(null)
const selectedMethod = ref('')
const selectedPlan = ref<SubscriptionPlan | null>(null)
const methodLimits = ref<Record<string, MethodLimit>>({})

const tabs = computed(() => {
  const result: { key: 'recharge' | 'subscription'; label: string }[] = []
  if (!config.value?.balance_disabled) result.push({ key: 'recharge', label: t('payment.tabTopUp') })
  result.push({ key: 'subscription', label: t('payment.tabSubscribe') })
  return result
})

// Available methods derived from limits API (actual provider types)
const enabledMethods = computed(() => Object.keys(methodLimits.value))
// 0 = no limit; provider-level overrides global
const minAmount = computed(() => {
  const limit = methodLimits.value[selectedMethod.value]
  if (limit?.single_min && limit.single_min > 0) return limit.single_min
  return config.value?.min_amount && config.value.min_amount > 0 ? config.value.min_amount : 0
})
const maxAmount = computed(() => {
  const limit = methodLimits.value[selectedMethod.value]
  if (limit?.single_max && limit.single_max > 0) return limit.single_max
  return config.value?.max_amount && config.value.max_amount > 0 ? config.value.max_amount : 0
})

const methodOptions = computed<PaymentMethodOption[]>(() =>
  enabledMethods.value.map((type) => {
    const limit = methodLimits.value[type]
    return { type, fee_rate: limit?.fee_rate ?? 0, available: limit?.available !== false }
  })
)

const validAmount = computed(() => amount.value ?? 0)
const feeRate = computed(() => methodLimits.value[selectedMethod.value]?.fee_rate ?? 0)
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
  if (minAmount.value > 0 && validAmount.value < minAmount.value) return t('payment.amountTooLow', { min: minAmount.value })
  if (maxAmount.value > 0 && validAmount.value > maxAmount.value) return t('payment.amountTooHigh', { max: maxAmount.value })
  return ''
})

const canSubmit = computed(() => {
  const limitInfo = methodLimits.value[selectedMethod.value]
  if (validAmount.value <= 0) return false
  if (minAmount.value > 0 && validAmount.value < minAmount.value) return false
  if (maxAmount.value > 0 && validAmount.value > maxAmount.value) return false
  return limitInfo?.available !== false
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
      router.push({ path: '/payment/stripe', query: { order_id: String(result.order_id), client_secret: result.client_secret } })
    } else if (result.qr_code) {
      router.push({ path: '/payment/qrcode', query: { order_id: String(result.order_id), qr: result.qr_code || '', pay_url: result.pay_url || '', expires_at: result.expires_at || '' } })
    } else if (result.pay_url) {
      window.open(result.pay_url, '_blank')
      router.push({ path: '/payment/qrcode', query: { order_id: String(result.order_id), pay_url: result.pay_url, expires_at: result.expires_at || '' } })
    } else {
      errorMessage.value = t('payment.result.failed')
      appStore.showError(errorMessage.value)
    }
  } catch (err: any) {
    if (err.reason === 'TOO_MANY_PENDING') {
      errorMessage.value = t('payment.errors.tooManyPending', { max: err.metadata?.max || '' })
    } else if (err.reason === 'CANCEL_RATE_LIMITED') {
      errorMessage.value = t('payment.errors.cancelRateLimited')
    } else {
      errorMessage.value = extractApiErrorMessage(err, t('payment.result.failed'))
    }
    appStore.showError(errorMessage.value)
  } finally {
    submitting.value = false
  }
}

async function loadPlans() {
  plansLoading.value = true
  try { await paymentStore.fetchPlans() } catch (err) { console.error('Failed to load plans:', err) }
  finally { plansLoading.value = false }
}

watch(() => activeTab.value, (tab) => {
  if (tab === 'subscription' && plans.value.length === 0) loadPlans()
})

onMounted(async () => {
  try {
    await paymentStore.fetchConfig(true)
    try {
      const limitsRes = await paymentAPI.getLimits()
      methodLimits.value = limitsRes.data
    } catch (e) { /* limits endpoint may not exist */ }
    if (enabledMethods.value.length) selectedMethod.value = enabledMethods.value[0]
    if (config.value?.balance_disabled) {
      activeTab.value = 'subscription'
      await loadPlans()
    }
  } catch (err) { console.error('Failed to load config:', err) }
  finally { loading.value = false }
})
</script>

<style scoped>
.fade-enter-active, .fade-leave-active { transition: all 0.3s ease; }
.fade-enter-from, .fade-leave-to { opacity: 0; transform: translateY(-8px); }
</style>
