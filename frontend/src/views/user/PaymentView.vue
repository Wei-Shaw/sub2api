<template>
  <AppLayout>
    <div class="mx-auto max-w-4xl space-y-6">
      <div v-if="loading" class="flex items-center justify-center py-20">
        <div class="h-8 w-8 animate-spin rounded-full border-4 border-primary-500 border-t-transparent"></div>
      </div>
      <template v-else>
        <!-- Tab Switcher (hide during payment and subscription confirm) -->
        <div v-if="tabs.length > 1 && paymentPhase === 'select' && !selectedPlan" class="flex space-x-1 rounded-xl bg-gray-100 p-1 dark:bg-dark-800">
          <button v-for="tab in tabs" :key="tab.key"
            class="flex-1 rounded-lg px-4 py-2.5 text-sm font-medium transition-all"
            :class="activeTab === tab.key ? 'bg-white text-gray-900 shadow dark:bg-dark-700 dark:text-white' : 'text-gray-500 hover:text-gray-700 dark:text-gray-400 dark:hover:text-gray-300'"
            @click="activeTab = tab.key">{{ tab.label }}</button>
        </div>
        <!-- Payment in progress (shared by recharge and subscription) -->
        <template v-if="paymentPhase === 'paying'">
          <PaymentStatusPanel
            :order-id="paymentState.orderId"
            :amount="paymentState.amount"
            :pay-amount="paymentState.payAmount"
            :qr-code="paymentState.qrCode"
            :expires-at="paymentState.expiresAt"
            :payment-type="paymentState.paymentType"
            :pay-url="paymentState.payUrl"
            :order-type="paymentState.orderType"
            :currency="paymentState.currency || selectedCurrency"
            :payment-amount="paymentState.paymentAmount"
            :ledger-amount="paymentState.ledgerAmount"
            :payment-currency="paymentState.paymentCurrency || paymentState.currency || selectedCurrency"
            :ledger-currency="paymentState.ledgerCurrency"
            :out-trade-no="paymentState.outTradeNo"
            :mobile-alipay-deep-link="paymentState.alipayMobilePrecreateDeepLink"
            @done="onPaymentDone"
            @success="onPaymentSuccess"
            @settled="onPaymentSettled"
          />
        </template>
        <template v-else-if="paymentPhase === 'paddle'">
          <PaddleCheckoutInline
            :checkout-id="paymentState.checkoutId"
            :client-token="checkout.paddle_client_token ?? ''"
            :environment="checkout.paddle_environment"
            :order="paddleCheckoutOrder"
            :plan="paddleCheckoutPlan"
            :currency-meta="checkout.currency_meta"
            @back="resetPayment"
            @completed="onPaddleCompleted"
          />
        </template>
        <!-- Tab content (select phase) -->
        <template v-else>
          <!-- Top-up Tab -->
          <template v-if="activeTab === 'recharge'">
            <!-- Recharge Account Card -->
            <div class="card p-5">
              <p class="text-xs font-medium text-gray-400 dark:text-gray-500">{{ t('payment.rechargeAccount') }}</p>
              <p class="mt-1 text-base font-semibold text-gray-900 dark:text-white">{{ user?.username || '' }}</p>
              <p class="mt-0.5 text-sm font-medium text-green-600 dark:text-green-400">{{ t('payment.currentBalance') }}: {{ user?.balance?.toFixed(2) || '0.00' }}</p>
            </div>
            <div v-if="currencySelectableMethods.length === 0" class="card py-16 text-center">
              <p class="text-gray-500 dark:text-gray-400">{{ t('payment.notAvailable') }}</p>
            </div>
            <template v-else>
            <div class="card p-6">
              <div v-if="availablePaymentCurrencies.length > 1" class="mb-4">
                <label class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">
                  {{ t('payment.paymentCurrency') }}
                </label>
                <select v-model="selectedPaymentCurrency" class="input w-full">
                  <option v-for="currency in availablePaymentCurrencies" :key="currency" :value="currency">
                    {{ currencyMeta(currency, checkout.currency_meta).symbol }} {{ currency }}
                  </option>
                </select>
              </div>
              <!-- Balance Package Selector -->
              <div v-if="availableBalancePackages.length > 0" class="mb-4">
                <label class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">
                  {{ t('payment.selectPackage') || 'Select Package' }}
                </label>
                <div class="grid grid-cols-2 gap-2 sm:grid-cols-3">
                  <button
                    v-for="pkg in availableBalancePackages"
                    :key="pkg.code"
                    :class="[
                      'relative rounded-lg border p-3 text-left transition-all',
                      selectedBalancePackage?.code === pkg.code
                        ? 'border-primary-500 bg-primary-50 ring-2 ring-primary-500 dark:bg-primary-900/20'
                        : 'border-gray-200 hover:border-primary-300 dark:border-dark-500 dark:hover:border-primary-600',
                    ]"
                    @click="selectBalancePackage(pkg)"
                  >
                    <span v-if="pkg.badge" class="absolute -top-2 right-2 rounded bg-amber-500 px-1.5 py-0.5 text-[10px] font-bold text-white">{{ pkg.badge }}</span>
                    <p class="text-sm font-semibold text-gray-900 dark:text-white">{{ pkg.label }}</p>
                    <p class="mt-0.5 text-xs text-gray-500 dark:text-gray-400">{{ formatTokens(pkg.actual_credits) }} tokens</p>
                  </button>
                </div>
              </div>
              <AmountInput
                v-model="amount"
                :amounts="localQuickAmounts"
                :min="globalMinAmount"
                :max="globalMaxAmount"
                :currency-symbol="paymentCurrencyMeta.symbol"
                :minor-units="paymentMinorUnits"
              />
              <p v-if="amountError" class="mt-2 text-xs text-amber-600 dark:text-amber-300">{{ amountError }}</p>
            </div>
            <div v-if="showPaymentMethodSelector" class="card p-6">
              <PaymentMethodSelector
                :methods="methodOptions"
                :selected="selectedMethod"
                @select="selectedMethod = $event"
              />
            </div>
            <div v-if="validAmount > 0" class="card p-6">
              <div class="space-y-2 text-sm">
                <div class="flex justify-between">
                  <span class="text-gray-500 dark:text-gray-400">{{ t('payment.paymentAmount') }}</span>
                  <span class="text-gray-900 dark:text-white">{{ formatPaymentMoney(validAmount) }}</span>
                </div>
                <div v-if="feeRate > 0" class="flex justify-between">
                  <span class="text-gray-500 dark:text-gray-400">{{ t('payment.fee') }} ({{ feeRate }}%)</span>
                  <span class="text-gray-900 dark:text-white">{{ formatPaymentMoney(feeAmount) }}</span>
                </div>
                <div v-if="feeRate > 0" class="flex justify-between border-t border-gray-200 pt-2 dark:border-dark-600">
                  <span class="font-medium text-gray-700 dark:text-gray-300">{{ t('payment.actualPay') }}</span>
                  <span class="text-lg font-bold text-primary-600 dark:text-primary-400">{{ formatPaymentMoney(totalAmount) }}</span>
                </div>
                <div class="flex justify-between" :class="{ 'border-t border-gray-200 pt-2 dark:border-dark-600': feeRate <= 0 }">
                  <span class="text-gray-500 dark:text-gray-400">{{ t('payment.creditedBalance') }}</span>
                  <span class="text-gray-900 dark:text-white">{{ formatLedgerMoney(creditedAmount) }}</span>
                </div>
                <p class="border-t border-gray-200 pt-2 text-xs text-gray-500 dark:border-dark-600 dark:text-gray-400">
                  {{ t('payment.rechargeRatePreview', { local: formatPaymentMoney(1), usd: formatLedgerMoney(ledgerAmountFromPayment(1, paymentCurrency, ledgerCurrency, checkout.manual_fx_rates, checkout.currency_meta)) }) }}
                </p>
                <p v-if="fxStatusMessage" class="text-xs" :class="fxStatusClass">
                  {{ fxStatusMessage }}
                </p>
              </div>
            </div>
            <button :class="['btn w-full py-3 text-base font-medium', paymentButtonClass]" :disabled="!canSubmit || submitting" @click="handleSubmitRecharge">
              <span v-if="submitting" class="flex items-center justify-center gap-2">
                <span class="h-4 w-4 animate-spin rounded-full border-2 border-white border-t-transparent"></span>
                {{ t('common.processing') }}
              </span>
              <span v-else>{{ t('payment.createOrder') }} {{ formatPaymentMoney(totalAmount) }}</span>
            </button>
            </template>
          </template>
          <!-- Subscribe Tab -->
          <template v-else-if="activeTab === 'subscription'">
            <!-- Subscription confirm (inline, replaces plan list) -->
            <template v-if="selectedPlan">
              <div class="card p-5">
                <!-- Header: platform badge + plan name -->
                <div class="mb-3 flex flex-wrap items-center gap-2">
                  <span :class="['rounded-md border px-2 py-0.5 text-xs font-medium', planBadgeClass]">
                    {{ platformLabel(selectedPlan.group_platform || '') }}
                  </span>
                  <h3 class="text-lg font-bold text-gray-900 dark:text-white">{{ selectedPlan.name }}</h3>
                </div>
                <!-- Price -->
                <div class="flex items-baseline gap-2">
                  <span v-if="selectedPlan.original_price" class="text-sm text-gray-400 line-through dark:text-gray-500">
                    {{ formatSelectedSubscriptionPaymentAmount(selectedPlan.original_price) }}
                  </span>
                  <span :class="['text-3xl font-bold', planTextClass]">{{ formatSelectedSubscriptionPaymentAmount(selectedPlan.price) }}</span>
                  <span class="text-sm text-gray-500 dark:text-gray-400">/ {{ planValiditySuffix }}</span>
                </div>
                <!-- Description -->
                <p v-if="selectedPlan.description" class="mt-2 text-sm leading-relaxed text-gray-500 dark:text-gray-400">
                  {{ selectedPlan.description }}
                </p>
                <!-- Rate + Limits grid -->
                <div class="mt-3 grid grid-cols-2 gap-3">
                  <div>
                    <span class="text-xs text-gray-400 dark:text-gray-500">{{ t('payment.planCard.rate') }}</span>
                    <div class="flex items-baseline">
                      <span :class="['text-lg font-bold', planTextClass]">×{{ selectedPlan.rate_multiplier ?? 1 }}</span>
                    </div>
                  </div>
                  <div v-if="planHasPeakRate(selectedPlan)">
                    <span class="text-xs text-gray-400 dark:text-gray-500">{{ t('payment.planCard.peakRate') }}</span>
                    <div class="text-sm font-semibold text-amber-700 dark:text-amber-300">
                      {{ planPeakRateLabel(selectedPlan) }}
                    </div>
                  </div>
                  <div v-if="selectedPlan.daily_limit_usd != null">
                    <span class="text-xs text-gray-400 dark:text-gray-500">{{ t('payment.planCard.dailyLimit') }}</span>
                    <div class="text-lg font-semibold text-gray-800 dark:text-gray-200">${{ selectedPlan.daily_limit_usd }}</div>
                  </div>
                  <div v-if="selectedPlan.weekly_limit_usd != null">
                    <span class="text-xs text-gray-400 dark:text-gray-500">{{ t('payment.planCard.weeklyLimit') }}</span>
                    <div class="text-lg font-semibold text-gray-800 dark:text-gray-200">${{ selectedPlan.weekly_limit_usd }}</div>
                  </div>
                  <div v-if="selectedPlan.monthly_limit_usd != null">
                    <span class="text-xs text-gray-400 dark:text-gray-500">{{ t('payment.planCard.monthlyLimit') }}</span>
                    <div class="text-lg font-semibold text-gray-800 dark:text-gray-200">${{ selectedPlan.monthly_limit_usd }}</div>
                  </div>
                  <div v-if="selectedPlan.daily_limit_usd == null && selectedPlan.weekly_limit_usd == null && selectedPlan.monthly_limit_usd == null">
                    <span class="text-xs text-gray-400 dark:text-gray-500">{{ t('payment.planCard.quota') }}</span>
                    <div class="text-lg font-semibold text-gray-800 dark:text-gray-200">{{ t('payment.planCard.unlimited') }}</div>
                  </div>
                </div>
              </div>
              <div v-if="availablePaymentCurrencies.length > 1" class="card p-6">
                <label class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">
                  {{ t('payment.paymentCurrency') }}
                </label>
                <select v-model="selectedPaymentCurrency" class="input w-full">
                  <option v-for="currency in availablePaymentCurrencies" :key="currency" :value="currency">
                    {{ currencyMeta(currency, checkout.currency_meta).symbol }} {{ currency }}
                  </option>
                </select>
              </div>
              <div v-if="showPaymentMethodSelector" class="card p-6">
                <PaymentMethodSelector
                  :methods="subMethodOptions"
                  :selected="selectedMethod"
                  @select="selectedMethod = $event"
                />
              </div>
              <div v-if="feeRate > 0 && selectedPlan.price > 0" class="card p-6">
                <div class="space-y-2 text-sm">
                  <div class="flex justify-between">
                    <span class="text-gray-500 dark:text-gray-400">{{ t('payment.amountLabel') }}</span>
                    <span class="text-gray-900 dark:text-white">{{ formatSelectedPaymentAmount(subPaymentAmount) }}</span>
                  </div>
                  <div class="flex justify-between">
                    <span class="text-gray-500 dark:text-gray-400">{{ t('payment.fee') }} ({{ feeRate }}%)</span>
                    <span class="text-gray-900 dark:text-white">{{ formatSelectedPaymentAmount(subFeeAmount) }}</span>
                  </div>
                  <div class="flex justify-between border-t border-gray-200 pt-2 dark:border-dark-600">
                    <span class="font-medium text-gray-700 dark:text-gray-300">{{ t('payment.actualPay') }}</span>
                    <span class="text-lg font-bold text-primary-600 dark:text-primary-400">{{ formatSelectedPaymentAmount(subTotalAmount) }}</span>
                  </div>
                </div>
              </div>
              <button :class="['btn w-full py-3 text-base font-medium', paymentButtonClass]" :disabled="!canSubmitSubscription || submitting" @click="confirmSubscribe">
                <span v-if="submitting" class="flex items-center justify-center gap-2">
                  <span class="h-4 w-4 animate-spin rounded-full border-2 border-white border-t-transparent"></span>
                  {{ t('common.processing') }}
                </span>
                <span v-else>{{ t('payment.createOrder') }} {{ formatSelectedPaymentAmount(subTotalAmount) }}</span>
              </button>
              <button class="btn btn-secondary w-full" @click="selectedPlan = null">{{ t('common.cancel') }}</button>
            </template>
            <!-- Plan list -->
            <template v-else>
              <div v-if="checkout.plans.length === 0" class="card py-16 text-center">
                <Icon name="gift" size="xl" class="mx-auto mb-3 text-gray-300 dark:text-dark-600" />
                <p class="text-gray-500 dark:text-gray-400">{{ t('payment.noPlans') }}</p>
              </div>
              <div v-else :class="planGridClass">
                <SubscriptionPlanCard v-for="plan in checkout.plans" :key="plan.id" :plan="plan" :active-subscriptions="activeSubscriptions" @select="selectPlan" />
              </div>
              <!-- Active subscriptions (compact, below plan list) -->
              <div v-if="activeSubscriptions.length > 0">
                <p class="mb-2 text-xs font-medium text-gray-400 dark:text-gray-500">{{ t('payment.activeSubscription') }}</p>
                <div class="space-y-2">
                  <div v-for="sub in activeSubscriptions" :key="sub.id"
                    class="flex items-center gap-3 rounded-xl border border-gray-100 bg-white px-3 py-2 dark:border-dark-700 dark:bg-dark-800">
                    <div :class="['h-6 w-1 shrink-0 rounded-full', platformAccentBarClass(sub.group?.platform || '')]" />
                    <div class="min-w-0 flex-1">
                      <div class="flex items-center gap-1.5">
                        <span class="truncate text-xs font-semibold text-gray-900 dark:text-white">{{ sub.group?.name || t('payment.groupFallback', { id: sub.group_id }) }}</span>
                        <span :class="['shrink-0 rounded-full px-1.5 py-0.5 text-[9px] font-medium', platformBadgeLightClass(sub.group?.platform || '')]">{{ platformLabel(sub.group?.platform || '') }}</span>
                      </div>
                      <div class="flex flex-wrap gap-x-3 text-[11px] text-gray-400 dark:text-gray-500">
                        <span>{{ t('payment.planCard.rate') }}: ×{{ sub.group?.rate_multiplier ?? 1 }}</span>
                        <span v-if="subscriptionHasPeakRate(sub)">{{ t('payment.planCard.peakRate') }}: {{ subscriptionPeakRateLabel(sub) }}</span>
                        <span v-if="sub.group?.daily_limit_usd == null && sub.group?.weekly_limit_usd == null && sub.group?.monthly_limit_usd == null">{{ t('payment.planCard.quota') }}: {{ t('payment.planCard.unlimited') }}</span>
                        <span v-if="sub.expires_at">{{ t('userSubscriptions.daysRemaining', { days: getDaysRemaining(sub.expires_at) }) }}</span>
                        <span v-else>{{ t('userSubscriptions.noExpiration') }}</span>
                      </div>
                    </div>
                    <span class="badge badge-success shrink-0 text-[10px]">{{ t('userSubscriptions.status.active') }}</span>
                  </div>
                </div>
              </div>
            </template>
          </template>
        </template>
        <div v-if="(checkout.help_text || checkout.help_image_url) && paymentPhase === 'select' && !selectedPlan" class="card p-4">
          <div class="flex flex-col items-center gap-3">
            <img v-if="checkout.help_image_url" :src="checkout.help_image_url" alt=""
              class="h-40 max-w-full cursor-pointer rounded-lg object-contain transition-opacity hover:opacity-80"
              @click="previewImage = checkout.help_image_url" />
            <p v-if="checkout.help_text" class="text-center text-sm text-gray-500 dark:text-gray-400">{{ checkout.help_text }}</p>
          </div>
        </div>
      </template>
    </div>
    <!-- Renewal Plan Selection Modal -->
    <Teleport to="body">
      <Transition name="modal">
        <div v-if="showRenewalModal" class="fixed inset-0 z-50 flex items-center justify-center bg-black/60 backdrop-blur-sm p-4" @click.self="closeRenewalModal">
          <div class="relative w-full max-w-lg rounded-2xl border border-gray-200 bg-white p-6 shadow-2xl dark:border-dark-700 dark:bg-dark-900">
            <!-- Close button -->
            <button class="absolute right-4 top-4 rounded-lg p-1 text-gray-400 transition-colors hover:bg-gray-100 hover:text-gray-600 dark:hover:bg-dark-700 dark:hover:text-gray-200" @click="closeRenewalModal">
              <svg class="h-5 w-5" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2"><path stroke-linecap="round" stroke-linejoin="round" d="M6 18L18 6M6 6l12 12" /></svg>
            </button>
            <h3 class="mb-4 text-lg font-semibold text-gray-900 dark:text-white">{{ t('payment.selectPlan') }}</h3>
            <div class="space-y-4">
              <SubscriptionPlanCard v-for="plan in renewalPlans" :key="plan.id" :plan="plan" :active-subscriptions="activeSubscriptions" @select="selectPlanFromModal" />
            </div>
          </div>
        </div>
      </Transition>
    </Teleport>
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
import { useRoute, useRouter } from 'vue-router'
import { useAuthStore } from '@/stores/auth'
import { usePaymentStore } from '@/stores/payment'
import { useSubscriptionStore } from '@/stores/subscriptions'
import { useAppStore } from '@/stores'
import { paymentAPI, type PublicOrderVerifyResult } from '@/api/payment'
import { extractApiErrorMessage, extractI18nErrorMessage } from '@/utils/apiError'
import { isMobileDevice } from '@/utils/device'
import { hasPeakRate, formatPeakRateWindow, serverTimezoneLabel, type PeakRateFields } from '@/utils/peak-rate'
import type { SubscriptionPlan, CheckoutInfoResponse, CreateOrderResult, OrderType, PaymentOrder, BalancePackage } from '@/types/payment'
import AppLayout from '@/components/layout/AppLayout.vue'
import AmountInput from '@/components/payment/AmountInput.vue'
import PaymentMethodSelector from '@/components/payment/PaymentMethodSelector.vue'
import { METHOD_ORDER, getPaymentPopupFeatures, isBuiltInAlipayMethod, isBuiltInWxpayMethod } from '@/components/payment/providerConfig'
import { ceilMoney, currencyMeta, currencyMinorUnits, formatMoney, ledgerAmountFromPayment, paymentAmountFromLedger, roundMoney } from '@/utils/money'
import {
  PAYMENT_RECOVERY_STORAGE_KEY,
  buildCreateOrderPayload,
  clearPaymentRecoverySnapshot,
  decidePaymentLaunch,
  getVisibleMethods,
  normalizeVisibleMethod,
  readPaymentRecoverySnapshot,
  type PaymentRecoverySnapshot,
  writePaymentRecoverySnapshot,
} from '@/components/payment/paymentFlow'
import { platformAccentBarClass, platformBadgeLightClass, platformBadgeClass, platformTextClass, platformLabel } from '@/utils/platformColors'
import SubscriptionPlanCard from '@/components/payment/SubscriptionPlanCard.vue'
import PaymentStatusPanel from '@/components/payment/PaymentStatusPanel.vue'
import PaddleCheckoutInline from '@/components/payment/PaddleCheckoutInline.vue'
import Icon from '@/components/icons/Icon.vue'
import { DEFAULT_PAYMENT_CURRENCY, formatPaymentAmount, normalizePaymentCurrency } from '@/components/payment/currency'
import { planValiditySuffix as validitySuffixOf } from '@/components/payment/validity'
import type { PaymentMethodOption } from '@/components/payment/PaymentMethodSelector.vue'
import { buildPaymentErrorToastMessage, describePaymentScenarioError } from './paymentUx'
import { hasWechatResumeQuery, parseWechatResumeRoute, stripWechatResumeQuery } from './paymentWechatResume'

const i18n = useI18n()
const { t } = i18n
const route = useRoute()
const router = useRouter()
const authStore = useAuthStore()
const paymentStore = usePaymentStore()
const subscriptionStore = useSubscriptionStore()
const appStore = useAppStore()

const user = computed(() => authStore.user)
const activeSubscriptions = computed(() => subscriptionStore.activeSubscriptions)

function getDaysRemaining(expiresAt: string): number {
  const diff = new Date(expiresAt).getTime() - Date.now()
  return Math.max(0, Math.ceil(diff / (1000 * 60 * 60 * 24)))
}

function subscriptionHasPeakRate(sub: { group?: PeakRateFields | null }): boolean {
  return hasPeakRate(sub.group)
}

function subscriptionPeakRateLabel(sub: { group?: PeakRateFields | null }): string {
  return formatPeakRateWindow(sub.group, serverTimezoneLabel(appStore.cachedPublicSettings?.server_utc_offset))
}

const loading = ref(true)
const submitting = ref(false)
const errorMessage = ref('')
const errorHintMessage = ref('')
const activeTab = ref<'recharge' | 'subscription'>('recharge')
const amount = ref<number | null>(null)
const selectedMethod = ref('')
const selectedPlan = ref<SubscriptionPlan | null>(null)
const selectedBalancePackage = ref<BalancePackage | null>(null)
const previewImage = ref('')

const paymentPhase = ref<'select' | 'paying' | 'paddle'>('select')

interface CreateOrderOptions {
  openid?: string
  wechatResumeToken?: string
  paymentType?: string
  paymentCurrency?: string
  isResume?: boolean
  mobileQrFallbackAttempted?: boolean
  quoteId?: string
  balancePackageId?: string
}

interface WeixinJSBridgeLike {
  invoke(
    action: string,
    payload: Record<string, unknown>,
    callback: (result: Record<string, unknown>) => void,
  ): void
}

function emptyPaymentState(): PaymentRecoverySnapshot {
  return {
    orderId: 0,
    amount: 0,
    qrCode: '',
    expiresAt: '',
    paymentType: '',
    payUrl: '',
    outTradeNo: '',
    clientSecret: '',
    intentId: '',
    currency: '',
    countryCode: '',
    paymentEnv: '',
    checkoutId: '',
    payAmount: 0,
    paymentAmount: 0,
    ledgerAmount: 0,
    paymentCurrency: '',
    ledgerCurrency: '',
    orderType: '',
    paymentMode: '',
    resumeToken: '',
    alipayMobilePrecreateDeepLink: false,
    createdAt: 0,
  }
}

function getWeixinJSBridge(): WeixinJSBridgeLike | undefined {
  return (window as Window & { WeixinJSBridge?: WeixinJSBridgeLike }).WeixinJSBridge
}

function waitForWeixinJSBridge(timeoutMs = 4000): Promise<WeixinJSBridgeLike | null> {
  const existing = getWeixinJSBridge()
  if (existing) return Promise.resolve(existing)

  return new Promise((resolve) => {
    let settled = false
    const finish = (bridge: WeixinJSBridgeLike | null) => {
      if (settled) return
      settled = true
      document.removeEventListener('WeixinJSBridgeReady', handleReady)
      document.removeEventListener('onWeixinJSBridgeReady', handleReady)
      window.clearTimeout(timer)
      resolve(bridge)
    }
    const handleReady = () => finish(getWeixinJSBridge() ?? null)
    const timer = window.setTimeout(() => finish(getWeixinJSBridge() ?? null), timeoutMs)
    document.addEventListener('WeixinJSBridgeReady', handleReady, false)
    document.addEventListener('onWeixinJSBridgeReady', handleReady, false)
  })
}

async function invokeWechatJsapiPayment(payload: Record<string, unknown>): Promise<Record<string, unknown>> {
  const bridge = await waitForWeixinJSBridge()
  if (!bridge) {
    throw new Error('WECHAT_JSAPI_UNAVAILABLE')
  }
  return new Promise((resolve) => {
    bridge.invoke('getBrandWCPayRequest', payload, (result) => resolve(result || {}))
  })
}

const paymentState = ref<PaymentRecoverySnapshot>(emptyPaymentState())
const paddleCheckoutOrder = ref<PaymentOrder | null>(null)
const paddleCheckoutPlan = computed(() => {
  const planId = paddleCheckoutOrder.value?.plan_id
  if (!planId) return selectedPlan.value
  return checkout.value.plans.find(plan => plan.id === planId) ?? selectedPlan.value
})

function persistRecoverySnapshot(snapshot: PaymentRecoverySnapshot) {
  if (typeof window === 'undefined' || !snapshot.orderId) return
  writePaymentRecoverySnapshot(window.localStorage, snapshot, PAYMENT_RECOVERY_STORAGE_KEY)
}

function removeRecoverySnapshot() {
  if (typeof window === 'undefined') return
  clearPaymentRecoverySnapshot(window.localStorage, PAYMENT_RECOVERY_STORAGE_KEY)
}

function resetPayment() {
  paymentPhase.value = 'select'
  paymentState.value = emptyPaymentState()
  paddleCheckoutOrder.value = null
  removeRecoverySnapshot()
}

async function onPaddleCompleted() {
  const resultState = { ...paymentState.value }
  resetPayment()
  await redirectToPaymentResult(resultState)
}

async function redirectToPaymentResult(state: PaymentRecoverySnapshot): Promise<void> {
  const query: Record<string, string | undefined> = {}
  if (state.orderId > 0) {
    query.order_id = String(state.orderId)
  }
  if (state.outTradeNo) {
    query.out_trade_no = state.outTradeNo
  }
  if (state.resumeToken) {
    query.resume_token = state.resumeToken
  }
  await router.push({
    path: '/payment/result',
    query,
  })
}

function buildWechatOAuthAuthorizeUrl(
  authorizeUrl: string,
  context: { paymentType: string; orderType: OrderType; planId?: number; orderAmount: number; amountMode?: 'ledger' | 'payment'; paymentCurrency?: string; quoteId?: string },
): string {
  const normalizedUrl = authorizeUrl.trim()
  if (!normalizedUrl || typeof window === 'undefined') {
    return normalizedUrl
  }

  try {
    const targetUrl = new URL(normalizedUrl, window.location.origin)
    const redirectPath = targetUrl.searchParams.get('redirect') || '/purchase'
    const redirectUrl = new URL(redirectPath, window.location.origin)
    const paymentType = normalizeVisibleMethod(context.paymentType) || context.paymentType.trim() || 'wxpay'

    redirectUrl.searchParams.set('payment_type', paymentType)
    redirectUrl.searchParams.set('order_type', context.orderType)
    if (context.amountMode) {
      redirectUrl.searchParams.set('amount_mode', context.amountMode)
    } else {
      redirectUrl.searchParams.delete('amount_mode')
    }
    if (context.paymentCurrency) {
      redirectUrl.searchParams.set('payment_currency', context.paymentCurrency)
    } else {
      redirectUrl.searchParams.delete('payment_currency')
    }
    if (context.quoteId) {
      redirectUrl.searchParams.set('quote_id', context.quoteId)
    } else {
      redirectUrl.searchParams.delete('quote_id')
    }

    if (context.planId) {
      redirectUrl.searchParams.set('plan_id', String(context.planId))
    } else {
      redirectUrl.searchParams.delete('plan_id')
    }

    if (context.orderAmount > 0) {
      redirectUrl.searchParams.set('amount', String(context.orderAmount))
    } else {
      redirectUrl.searchParams.delete('amount')
    }

    targetUrl.searchParams.set('redirect', `${redirectUrl.pathname}${redirectUrl.search}`)
    return targetUrl.toString()
  } catch {
    return normalizedUrl
  }
}

function onPaymentDone() {
  const wasSubscription = paymentState.value.orderType === 'subscription'
  resetPayment()
  selectedPlan.value = null
  if (wasSubscription) {
    subscriptionStore.fetchActiveSubscriptions(true).catch(() => {})
  }
}

async function onPaymentSuccess() {
  const completedPayment = { ...paymentState.value }
  removeRecoverySnapshot()
  authStore.refreshUser()
  if (paymentState.value.orderType === 'subscription') {
    subscriptionStore.fetchActiveSubscriptions(true).catch(() => {})
  }
  await redirectToPaymentResult(completedPayment)
}

function onPaymentSettled() {
  removeRecoverySnapshot()
}

// All checkout data from single API call
const checkout = ref<CheckoutInfoResponse>({
  methods: {}, global_min: 0, global_max: 0,
  plans: [], balance_packages: [], balance_disabled: false, balance_recharge_multiplier: 1, subscription_usd_to_cny_rate: 0, recharge_fee_rate: 0, help_text: '', help_image_url: '', stripe_publishable_key: '', paddle_client_token: '', paddle_environment: '',
  ledger_currency: 'USD', allowed_payment_currencies: ['USD'], manual_fx_rates: {}, currency_meta: {},
  fx_status: { source: 'manual', stale_after_seconds: 86400, stale: true, missing_currencies: [] },
})
const selectedPaymentCurrency = ref('')

const tabs = computed(() => {
  const result: { key: 'recharge' | 'subscription'; label: string }[] = []
  if (!checkout.value.balance_disabled) result.push({ key: 'recharge', label: t('payment.tabTopUp') })
  result.push({ key: 'subscription', label: t('payment.tabSubscribe') })
  return result
})

const availableBalancePackages = computed(() => {
  return (checkout.value.balance_packages || []).filter(p => p.for_sale).sort((a, b) => a.sort_order - b.sort_order)
})

const visibleMethods = computed(() => getVisibleMethods(checkout.value.methods))
const visibleMethodTypes = computed(() => Object.keys(visibleMethods.value))
const ledgerCurrency = computed(() => checkout.value.ledger_currency || 'USD')
const availablePaymentCurrencies = computed(() => {
  const currencies = checkout.value.allowed_payment_currencies?.map(c => c.trim().toUpperCase()).filter(Boolean) ?? []
  return currencies.length ? currencies : [ledgerCurrency.value]
})
const paymentCurrency = computed(() => {
  const selected = selectedPaymentCurrency.value.trim().toUpperCase()
  return availablePaymentCurrencies.value.includes(selected) ? selected : availablePaymentCurrencies.value[0]
})
const paymentCurrencyMeta = computed(() => currencyMeta(paymentCurrency.value, checkout.value.currency_meta))
const paymentMinorUnits = computed(() => currencyMinorUnits(paymentCurrency.value, checkout.value.currency_meta))
const enabledMethods = computed(() => Object.keys(visibleMethods.value).filter(methodSupportsCurrentSelection))
const validAmount = computed(() => amount.value ?? 0)
const ledgerPreviewAmount = computed(() => ledgerAmountFromPayment(
  validAmount.value,
  paymentCurrency.value,
  ledgerCurrency.value,
  checkout.value.manual_fx_rates,
  checkout.value.currency_meta,
))
const balanceRechargeMultiplier = computed(() => {
  const multiplier = checkout.value.balance_recharge_multiplier
  return Number.isFinite(multiplier) && multiplier > 0 ? multiplier : 1
})
// 订阅 CNY 换算汇率（1 USD = X CNY）。0 = 未配置，订阅保持 price 直付（与后端 opt-in 条件严格镜像）。
const subscriptionUsdToCnyRate = computed(() => {
  const rate = checkout.value.subscription_usd_to_cny_rate
  return Number.isFinite(rate) && rate > 0 ? rate : 0
})
const creditedAmount = computed(() => roundMoney(ledgerPreviewAmount.value * balanceRechargeMultiplier.value, ledgerCurrency.value, checkout.value.currency_meta))
const localQuickAmounts = computed(() => {
  const currency = paymentCurrency.value
  if (currency === 'VND') return [50000, 100000, 200000, 500000, 1000000, 2000000, 5000000]
  if (currency === 'KRW') return [5000, 10000, 20000, 50000, 100000, 200000, 500000]
  if (currency === 'CNY') return [50, 100, 200, 500, 1000, 2000, 5000]
  return [10, 20, 50, 100, 200, 500, 1000]
})
const localMinAmount = computed(() => checkout.value.global_min > 0
  ? paymentAmountFromLedger(checkout.value.global_min, paymentCurrency.value, ledgerCurrency.value, checkout.value.manual_fx_rates, checkout.value.currency_meta)
  : 0)
const localMaxAmount = computed(() => checkout.value.global_max > 0
  ? paymentAmountFromLedger(checkout.value.global_max, paymentCurrency.value, ledgerCurrency.value, checkout.value.manual_fx_rates, checkout.value.currency_meta)
  : 0)
function formatPaymentMoney(value: number): string {
  return formatMoney(value, paymentCurrency.value, checkout.value.currency_meta)
}
function formatLedgerMoney(value: number): string {
  return formatMoney(value, ledgerCurrency.value, checkout.value.currency_meta)
}
function formatTokens(value: number): string {
  if (value >= 1_000_000_000) return `${(value / 1_000_000_000).toFixed(1)}B`
  if (value >= 1_000_000) return `${(value / 1_000_000).toFixed(0)}M`
  if (value >= 1_000) return `${(value / 1_000).toFixed(0)}K`
  return String(value)
}
const fxStatusClass = computed(() => checkout.value.fx_status?.stale
  ? 'text-amber-600 dark:text-amber-300'
  : 'text-gray-500 dark:text-gray-400')
const fxStatusMessage = computed(() => {
  const status = checkout.value.fx_status
  if (!status) return ''
  const source = status.source || 'manual'
  const missing = status.missing_currencies?.join(', ')
  if (missing) return t('payment.fxRateMissing', { currencies: missing })
  if (status.stale) return t('payment.fxRateStale', { source })
  if (!status.updated_at) return ''
  return t('payment.fxRateUpdated', { source, time: new Date(status.updated_at).toLocaleString() })
})
function methodSupportsSelectedCurrency(method: string): boolean {
  const currency = paymentCurrency.value
  const methodCurrencies = visibleMethods.value[method]?.allowed_payment_currencies
    ?.map(c => c.trim().toUpperCase())
    .filter(Boolean) ?? []
  if (methodCurrencies.length > 0) return methodCurrencies.includes(currency)
  if (method === 'sepay') return currency === 'VND'
  if (['stripe', 'alipay', 'wxpay', 'easypay', 'card', 'link', 'alipay_direct', 'wxpay_direct'].includes(method)) return currency === 'CNY'
  if (method === 'paddle') return true
  return currency === ledgerCurrency.value
}

function sortMethodsByDisplayOrder(methods: string[]): string[] {
  const order: readonly string[] = METHOD_ORDER
  return [...methods].sort((a, b) => {
    const ai = order.indexOf(a as typeof METHOD_ORDER[number])
    const bi = order.indexOf(b as typeof METHOD_ORDER[number])
    return (ai === -1 ? 999 : ai) - (bi === -1 ? 999 : bi)
  })
}

function methodSupportsCurrentSelection(method: string): boolean {
  const ml = visibleMethods.value[method]
  return !!ml && ml.available !== false && methodSupportsSelectedCurrency(method)
}

const currencySelectableMethods = computed(() =>
  sortMethodsByDisplayOrder(visibleMethodTypes.value.filter(methodSupportsCurrentSelection))
)
const showPaymentMethodSelector = computed(() => currencySelectableMethods.value.length > 1)

function selectDefaultMethodForCurrency() {
  const currentMethod = selectedMethod.value
  if (currentMethod && currencySelectableMethods.value.includes(currentMethod)) return
  selectedMethod.value = currencySelectableMethods.value[0] || ''
}

// Adaptive grid: center single card, 2-col for 2 plans, 3-col for 3+
const planGridClass = computed(() => {
  const n = checkout.value.plans.length
  if (n <= 2) return 'grid grid-cols-1 gap-5 sm:grid-cols-2'
  return 'grid grid-cols-1 gap-5 sm:grid-cols-2 lg:grid-cols-3'
})

// Check if an amount fits a method's [min, max]. 0 = no limit.
function amountFitsMethod(amt: number, methodType: string): boolean {
  if (amt <= 0) return true
  const ml = visibleMethods.value[methodType]
  if (!ml) return false
  const ledgerAmount = paymentCurrency.value === ledgerCurrency.value
    ? amt
    : ledgerAmountFromPayment(amt, paymentCurrency.value, ledgerCurrency.value, checkout.value.manual_fx_rates, checkout.value.currency_meta)
  if (ledgerAmount <= 0) return false
  if (ml.single_min > 0 && ledgerAmount < ml.single_min) return false
  if (ml.single_max > 0 && ledgerAmount > ml.single_max) return false
  return true
}

// Visible methods decide the amount range shown to users.
const globalMinAmount = computed(() => localMinAmount.value)
const globalMaxAmount = computed(() => localMaxAmount.value)

// Selected method's limits (for validation and error messages)
const selectedLimit = computed(() => visibleMethods.value[selectedMethod.value])
const selectedCurrency = computed(() => normalizePaymentCurrency(selectedLimit.value?.currency))
const localeCode = computed(() => {
  const raw = i18n.locale as unknown
  if (typeof raw === 'string') return raw
  if (raw && typeof raw === 'object' && 'value' in raw) {
    return String((raw as { value?: string }).value || '')
  }
  return undefined
})

function currencyFractionDigits(currency: string): number {
  try {
    return new Intl.NumberFormat(undefined, {
      style: 'currency',
      currency,
    }).resolvedOptions().maximumFractionDigits ?? 2
  } catch {
    return 2
  }
}

function roundPaymentAmount(value: number, currency: string): number {
  if (!Number.isFinite(value)) return 0
  const factor = 10 ** currencyFractionDigits(currency)
  return Math.round(value * factor) / factor
}

function ceilPaymentAmount(value: number, currency: string): number {
  if (!Number.isFinite(value)) return 0
  const factor = 10 ** currencyFractionDigits(currency)
  return Math.ceil(value * factor) / factor
}

function subscriptionPaymentAmountForCurrency(value: number, currency: string): number {
  const rate = subscriptionUsdToCnyRate.value
  if (rate <= 0 || currency !== DEFAULT_PAYMENT_CURRENCY) return roundPaymentAmount(value, currency)
  return roundPaymentAmount(value * rate, currency)
}

function formatSelectedPaymentAmount(value: number): string {
  return formatPaymentAmount(value, selectedCurrency.value, localeCode.value)
}

function formatSelectedSubscriptionPaymentAmount(value: number): string {
  return formatSelectedPaymentAmount(subscriptionPaymentAmountForCurrency(value, selectedCurrency.value))
}

const methodOptions = computed<PaymentMethodOption[]>(() =>
  currencySelectableMethods.value.map((type) => {
    const ml = visibleMethods.value[type]
    return {
      type,
      display_name: ml?.display_name,
      fee_rate: ml?.fee_rate ?? 0,
      available: amountFitsMethod(validAmount.value, type),
    }
  })
)

const feeRate = computed(() => checkout.value?.recharge_fee_rate ?? 0)
const feeAmount = computed(() =>
  feeRate.value > 0 && validAmount.value > 0
    ? ceilMoney((validAmount.value * feeRate.value) / 100, paymentCurrency.value, checkout.value.currency_meta)
    : 0
)
const totalAmount = computed(() =>
  feeRate.value > 0 && validAmount.value > 0
    ? roundMoney(validAmount.value + feeAmount.value, paymentCurrency.value, checkout.value.currency_meta)
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
    if (ml.single_min > 0 && ledgerPreviewAmount.value < ml.single_min) return t('payment.amountTooLow', { min: formatPaymentMoney(localMinAmount.value) })
    if (ml.single_max > 0 && ledgerPreviewAmount.value > ml.single_max) return t('payment.amountTooHigh', { max: formatPaymentMoney(localMaxAmount.value) })
  }
  return ''
})

const canSubmit = computed(() =>
  validAmount.value > 0
    && methodSupportsCurrentSelection(selectedMethod.value)
    && amountFitsMethod(validAmount.value, selectedMethod.value)
)

const subPaymentAmount = computed(() => {
  const price = selectedPlan.value?.price ?? 0
  return subscriptionPaymentAmountForCurrency(price, selectedCurrency.value)
})

const subFeeAmount = computed(() => {
  if (feeRate.value <= 0 || subPaymentAmount.value <= 0) return 0
  return ceilPaymentAmount((subPaymentAmount.value * feeRate.value) / 100, selectedCurrency.value)
})

const subTotalAmount = computed(() => {
  if (feeRate.value <= 0 || subPaymentAmount.value <= 0) return subPaymentAmount.value
  return roundPaymentAmount(subPaymentAmount.value + subFeeAmount.value, selectedCurrency.value)
})

function subscriptionTotalAmountForCurrency(value: number, currency: string): number {
  const paymentAmount = subscriptionPaymentAmountForCurrency(value, currency)
  if (feeRate.value <= 0 || paymentAmount <= 0) return paymentAmount
  const fee = ceilPaymentAmount((paymentAmount * feeRate.value) / 100, currency)
  return roundPaymentAmount(paymentAmount + fee, currency)
}

// Subscription-specific: method options based on gateway pay amount
const subMethodOptions = computed<PaymentMethodOption[]>(() => {
  const planPrice = selectedPlan.value?.price ?? 0
  return currencySelectableMethods.value.map((type) => {
    const ml = visibleMethods.value[type]
    const withinLimits = planPrice <= 0
      || (((ml?.single_min ?? 0) <= 0 || planPrice >= (ml?.single_min ?? 0))
        && ((ml?.single_max ?? 0) <= 0 || planPrice <= (ml?.single_max ?? 0)))
    const payAmountForMethod = subscriptionTotalAmountForCurrency(planPrice, normalizePaymentCurrency(ml?.currency))
    return {
      type,
      display_name: ml?.display_name,
      fee_rate: ml?.fee_rate ?? 0,
      available: ml?.available !== false && withinLimits && amountFitsMethod(payAmountForMethod, type),
    }
  })
})

const canSubmitSubscription = computed(() => {
  if (selectedPlan.value === null) return false
  const ml = selectedLimit.value
  if (!ml || !methodSupportsCurrentSelection(selectedMethod.value)) return false
  const price = selectedPlan.value.price
  if (ml.single_min > 0 && price < ml.single_min) return false
  if (ml.single_max > 0 && price > ml.single_max) return false
  return true
})
// Auto-switch to first available method when current selection can't handle the amount
watch(() => [validAmount.value, selectedMethod.value] as const, ([amt, method]) => {
  if (amt <= 0 || amountFitsMethod(amt, method)) return
  const available = enabledMethods.value.find((m) => amountFitsMethod(amt, m))
  if (available) selectedMethod.value = available
})

watch(paymentCurrency, () => {
  amount.value = null
  selectDefaultMethodForCurrency()
})

watch(currencySelectableMethods, selectDefaultMethodForCurrency, { immediate: true })

// Payment button class: follows selected payment method color
const paymentButtonClass = computed(() => {
  const m = selectedMethod.value
  if (!m) return 'btn-primary'
  if (isBuiltInAlipayMethod(m)) return 'btn-alipay'
  if (isBuiltInWxpayMethod(m)) return 'btn-wxpay'
  if (m === 'stripe') return 'btn-stripe'
  if (m === 'airwallex') return 'btn-airwallex'
  return 'btn-primary'
})

// Subscription confirm: platform accent colors (clean card, no gradient)
const planBadgeClass = computed(() => platformBadgeClass(selectedPlan.value?.group_platform || ''))
const planTextClass = computed(() => platformTextClass(selectedPlan.value?.group_platform || ''))

// Renewal modal state
const showRenewalModal = ref(false)
const renewGroupId = ref<number | null>(null)
const renewalPlans = computed(() => {
  if (renewGroupId.value == null) return []
  return checkout.value.plans.filter(p => p.group_id === renewGroupId.value)
})

const planValiditySuffix = computed(() => {
  if (!selectedPlan.value) return ''
  return validitySuffixOf(selectedPlan.value, t)
})

function planHasPeakRate(plan: SubscriptionPlan): boolean {
  return hasPeakRate(plan)
}

function planPeakRateLabel(plan: SubscriptionPlan): string {
  return formatPeakRateWindow(plan, serverTimezoneLabel(appStore.cachedPublicSettings?.server_utc_offset))
}

function selectPlan(plan: SubscriptionPlan) {
  selectedPlan.value = plan
  errorMessage.value = ''
}

function selectPlanFromModal(plan: SubscriptionPlan) {
  showRenewalModal.value = false
  renewGroupId.value = null
  selectedPlan.value = plan
  errorMessage.value = ''
}

function closeRenewalModal() {
  showRenewalModal.value = false
  renewGroupId.value = null
}

async function handleSubmitRecharge() {
  if (!canSubmit.value || submitting.value) return
  const pkg = selectedBalancePackage.value
  await createOrder(validAmount.value, 'balance', undefined, { balancePackageId: pkg?.code })
}

function selectBalancePackage(pkg: BalancePackage) {
  if (selectedBalancePackage.value?.code === pkg.code) {
    selectedBalancePackage.value = null
    amount.value = null
  } else {
    selectedBalancePackage.value = pkg
    amount.value = pkg.amount_ledger
  }
}

async function confirmSubscribe() {
  if (!selectedPlan.value || submitting.value) return
  await createOrder(selectedPlan.value.price, 'subscription', selectedPlan.value.id)
}

async function createPaymentQuoteForOrder(orderAmount: number, orderType: OrderType, paymentType: string, planId?: number, balancePackageId?: string): Promise<string> {
  if (orderAmount <= 0) {
    return ''
  }
  const amountMode = orderType === 'subscription' ? 'ledger' : 'payment'
  const res = await paymentAPI.createQuote({
    amount: orderAmount,
    amount_mode: amountMode,
    payment_currency: paymentCurrency.value,
    payment_type: paymentType,
    order_type: orderType,
    plan_id: planId,
    balance_package_id: balancePackageId,
  })
  return res.data.quote_id
}

async function createOrder(orderAmount: number, orderType: OrderType, planId?: number, options: CreateOrderOptions = {}) {
  submitting.value = true
  errorMessage.value = ''
  errorHintMessage.value = ''
  const requestType = normalizeVisibleMethod(options.paymentType || selectedMethod.value) || options.paymentType || selectedMethod.value
  const requestPaymentCurrency = (options.paymentCurrency || paymentCurrency.value).trim().toUpperCase()
  try {
    const quoteId = options.wechatResumeToken
      ? ''
      : options.quoteId || await createPaymentQuoteForOrder(orderAmount, orderType, requestType, planId, options.balancePackageId)
    const payload = buildCreateOrderPayload({
      amount: orderAmount,
      paymentType: requestType,
      orderType,
      planId,
      balancePackageId: options.balancePackageId,
      origin: typeof window !== 'undefined' ? window.location.origin : '',
      isMobile: isMobileDevice(),
      isWechatBrowser: typeof window !== 'undefined' && /MicroMessenger/i.test(window.navigator.userAgent),
      forceQRCode: !!(checkout.value.alipay_force_qrcode && normalizeVisibleMethod(requestType) === 'alipay'),
      mobilePrecreateDeepLink: checkout.value.alipay_mobile_precreate_deep_link === true,
      amountMode: orderType === 'subscription' ? 'ledger' : 'payment',
      paymentCurrency: requestPaymentCurrency,
      quoteId: quoteId || undefined,
    })
    if (options.openid) {
      payload.openid = options.openid
    }
    if (options.wechatResumeToken) {
      payload.wechat_resume_token = options.wechatResumeToken
    }

    const result = await paymentStore.createOrder(payload) as CreateOrderResult & { resume_token?: string }
    const openWindow = (url: string) => {
      const win = window.open(url, 'paymentPopup', getPaymentPopupFeatures())
      if (!win || win.closed) {
        window.location.href = url
      }
    }
    const visibleMethod = normalizeVisibleMethod(requestType) || requestType
    // When user clicks the dedicated Stripe button, leave method blank so the
    // landing page renders Stripe's full Payment Element (card/link/alipay/wxpay).
    const stripeMethod = visibleMethod === 'stripe'
      ? ''
      : visibleMethod === 'wxpay' ? 'wechat_pay' : 'alipay'
    const stripeRouteUrl = result.client_secret && visibleMethod !== 'airwallex'
      ? router.resolve({
        path: '/payment/stripe',
        query: {
          order_id: String(result.order_id),
          client_secret: result.client_secret,
          method: stripeMethod || undefined,
          resume_token: result.resume_token || undefined,
        },
      }).href
      : ''
    const airwallexRouteUrl = result.client_secret && result.intent_id
      ? router.resolve({
        path: '/payment/airwallex',
        query: {
          order_id: String(result.order_id),
          out_trade_no: result.out_trade_no || undefined,
          resume_token: result.resume_token || undefined,
        },
      }).href
      : ''
    const decision = decidePaymentLaunch(result, {
      visibleMethod,
      orderType,
      isMobile: isMobileDevice(),
      isWechatBrowser: typeof window !== 'undefined' && /MicroMessenger/i.test(window.navigator.userAgent),
      forceQRCode: !!(checkout.value.alipay_force_qrcode && visibleMethod === 'alipay'),
      mobilePrecreateDeepLink: checkout.value.alipay_mobile_precreate_deep_link === true,
      stripePopupUrl: stripeRouteUrl,
      stripeRouteUrl,
      airwallexRouteUrl,
    })

    if (decision.kind === 'wechat_oauth' && decision.oauth?.authorize_url) {
      window.location.href = buildWechatOAuthAuthorizeUrl(decision.oauth.authorize_url, {
        paymentType: visibleMethod,
        orderType,
        planId,
        orderAmount,
        amountMode: orderType === 'subscription' ? 'ledger' : 'payment',
        paymentCurrency: requestPaymentCurrency,
        quoteId: quoteId || undefined,
      })
      return
    }

    if (decision.kind === 'unhandled') {
      if (decision.paymentState.checkoutId) {
        paymentState.value = decision.paymentState
        paddleCheckoutOrder.value = buildPaddleOrderFromCreateResult(result, orderType, planId)
        paymentPhase.value = 'paddle'
        persistRecoverySnapshot(decision.recovery)
        return
      }
      applyScenarioError({ reason: 'UNHANDLED_PAYMENT_SCENARIO' }, visibleMethod)
      return
    }

    paymentState.value = decision.paymentState
    paymentPhase.value = 'paying'
    persistRecoverySnapshot(decision.recovery)

    if (decision.kind === 'stripe_popup') {
      openWindow(decision.paymentState.payUrl)
      return
    }
    if (decision.kind === 'stripe_route') {
      window.location.href = decision.paymentState.payUrl
      return
    }
    if (decision.kind === 'airwallex_route') {
      window.location.href = decision.paymentState.payUrl
      return
    }
    if (decision.kind === 'wechat_jsapi' && decision.jsapi) {
      try {
        const jsapiResult = await invokeWechatJsapiPayment(decision.jsapi as Record<string, unknown>)
        const errMsg = String(jsapiResult.err_msg || '').toLowerCase()
        if (errMsg.includes('cancel')) {
          appStore.showInfo(t('payment.qr.cancelled'))
          resetPayment()
        } else if (errMsg && !errMsg.includes('ok')) {
          resetPayment()
          const fallbackApplied = await attemptMobileQrFallback(
            { reason: 'WECHAT_JSAPI_FAILED', message: errMsg },
            {
              orderAmount,
              orderType,
              planId,
              paymentType: visibleMethod,
              attempted: options.mobileQrFallbackAttempted === true,
            },
          )
          if (!fallbackApplied) {
            applyScenarioError({ reason: 'WECHAT_JSAPI_FAILED', message: errMsg }, visibleMethod)
          }
        } else {
          const resultState = { ...decision.paymentState }
          resetPayment()
          await redirectToPaymentResult(resultState)
        }
      } catch (err: unknown) {
        resetPayment()
        const fallbackApplied = await attemptMobileQrFallback(err, {
          orderAmount,
          orderType,
          planId,
          paymentType: visibleMethod,
          attempted: options.mobileQrFallbackAttempted === true,
        })
        if (!fallbackApplied) {
          throw err
        }
      }
      return
    }
    if (decision.kind === 'redirect_waiting' && decision.paymentState.payUrl) {
      if (isMobileDevice()) {
        window.location.href = decision.paymentState.payUrl
        return
      }
      openWindow(decision.paymentState.payUrl)
    }
  } catch (err: unknown) {
    const apiErr = err as Record<string, unknown>
    if (apiErr.reason === 'TOO_MANY_PENDING') {
      const metadata = apiErr.metadata as Record<string, unknown> | undefined
      errorMessage.value = t('payment.errors.tooManyPending', { max: metadata?.max || '' })
      errorHintMessage.value = ''
    } else if (apiErr.reason === 'CANCEL_RATE_LIMITED') {
      errorMessage.value = t('payment.errors.cancelRateLimited')
      errorHintMessage.value = ''
    } else if (await attemptMobileQrFallback(err, {
      orderAmount,
      orderType,
      planId,
      paymentType: requestType,
      attempted: options.mobileQrFallbackAttempted === true,
    })) {
      return
    } else {
      const handled = applyScenarioError(
        err,
        normalizeVisibleMethod(options.paymentType || selectedMethod.value) || selectedMethod.value,
      )
      if (!handled) {
        errorMessage.value = extractI18nErrorMessage(err, t, 'payment.errors', extractApiErrorMessage(err, t('payment.result.failed')))
        errorHintMessage.value = ''
      }
      if (handled) {
        return
      }
    }
    appStore.showError(buildPaymentErrorToastMessage(errorMessage.value, errorHintMessage.value))
  } finally {
    submitting.value = false
  }
}

interface MobileQrFallbackContext {
  orderAmount: number
  orderType: OrderType
  planId?: number
  paymentType: string
  attempted: boolean
}

function shouldFallbackToDesktopQr(err: unknown, paymentMethod: string, attempted: boolean): boolean {
  if (attempted || !isMobileDevice()) {
    return false
  }

  const normalizedMethod = normalizeVisibleMethod(paymentMethod) || paymentMethod
  const reason = typeof err === 'object' && err && 'reason' in err && typeof err.reason === 'string'
    ? err.reason
    : ''
  const message = err instanceof Error
    ? err.message
    : (typeof err === 'object' && err && 'message' in err && typeof err.message === 'string'
      ? err.message
      : '')
  const normalizedMessage = message.toLowerCase()

  if (normalizedMethod === 'wxpay') {
    return reason === 'WECHAT_H5_NOT_AUTHORIZED'
      || reason === 'WECHAT_PAYMENT_MP_NOT_CONFIGURED'
      || reason === 'WECHAT_JSAPI_FAILED'
      || reason === 'PAYMENT_GATEWAY_ERROR'
      || reason === 'UNHANDLED_PAYMENT_SCENARIO'
      || normalizedMessage.includes('weixinjsbridge is unavailable')
      || normalizedMessage.includes('wechat_jsapi_unavailable')
  }

  if (normalizedMethod === 'alipay') {
    return reason === 'PAYMENT_GATEWAY_ERROR' || reason === 'UNHANDLED_PAYMENT_SCENARIO'
  }

  return false
}

async function attemptMobileQrFallback(err: unknown, context: MobileQrFallbackContext): Promise<boolean> {
  if (!shouldFallbackToDesktopQr(err, context.paymentType, context.attempted)) {
    return false
  }

  try {
    const visibleMethod = normalizeVisibleMethod(context.paymentType) || context.paymentType
    const quoteId = await createPaymentQuoteForOrder(context.orderAmount, context.orderType, visibleMethod, context.planId)
    const payload = buildCreateOrderPayload({
      amount: context.orderAmount,
      paymentType: visibleMethod,
      orderType: context.orderType,
      planId: context.planId,
      origin: typeof window !== 'undefined' ? window.location.origin : '',
      isMobile: false,
      isWechatBrowser: false,
      amountMode: context.orderType === 'subscription' ? 'ledger' : 'payment',
      paymentCurrency: paymentCurrency.value,
      quoteId: quoteId || undefined,
    })
    const result = await paymentStore.createOrder(payload) as CreateOrderResult & { resume_token?: string }
    const stripeMethod = visibleMethod === 'wxpay' ? 'wechat_pay' : 'alipay'
    const stripeRouteUrl = result.client_secret
      ? router.resolve({
        path: '/payment/stripe',
        query: {
          order_id: String(result.order_id),
          client_secret: result.client_secret,
          method: stripeMethod,
          resume_token: result.resume_token || undefined,
        },
      }).href
      : ''
    const decision = decidePaymentLaunch(result, {
      visibleMethod,
      orderType: context.orderType,
      isMobile: false,
      isWechatBrowser: false,
      stripePopupUrl: stripeRouteUrl,
      stripeRouteUrl,
    })

    if (decision.kind !== 'qr_waiting' || !decision.paymentState.qrCode) {
      return false
    }

    errorMessage.value = ''
    errorHintMessage.value = ''
    paymentState.value = decision.paymentState
    paymentPhase.value = 'paying'
    persistRecoverySnapshot(decision.recovery)
    appStore.showWarning(t('payment.errors.mobilePaymentFallbackToQr'))
    return true
  } catch {
    return false
  }
}

function buildPaddleOrderFromCreateResult(
  result: CreateOrderResult & { resume_token?: string },
  orderType: OrderType,
  planId?: number,
): PaymentOrder {
  const paymentCurrencyCode = result.payment_currency || paymentCurrency.value || ledgerCurrency.value
  const ledgerCurrencyCode = result.ledger_currency || ledgerCurrency.value
  return {
    id: Number(result.order_id || 0),
    user_id: 0,
    amount: Number(result.amount || 0),
    pay_amount: Number(result.pay_amount || result.amount || 0),
    payment_amount: Number(result.payment_amount || result.pay_amount || result.amount || 0),
    ledger_amount: Number(result.ledger_amount || result.amount || 0),
    payment_currency: paymentCurrencyCode,
    ledger_currency: ledgerCurrencyCode,
    fee_rate: Number(result.fee_rate || 0),
    payment_type: result.payment_type || 'paddle',
    out_trade_no: result.out_trade_no || '',
    status: 'PENDING',
    order_type: orderType,
    created_at: new Date().toISOString(),
    expires_at: result.expires_at || '',
    refund_amount: 0,
    plan_id: planId,
  }
}

function buildPaddleOrderFromPublicResult(result: PublicOrderVerifyResult): PaymentOrder {
  const paymentCurrencyCode = result.payment_currency || result.currency || paymentCurrency.value || ledgerCurrency.value
  const ledgerCurrencyCode = result.ledger_currency || ledgerCurrency.value
  const amount = Number(result.amount || result.pay_amount || result.payment_amount || result.ledger_amount || 0)
  return {
    id: Number(result.id || 0),
    user_id: Number(result.user_id || 0),
    amount,
    pay_amount: Number(result.pay_amount || amount),
    payment_amount: Number(result.payment_amount || result.pay_amount || amount),
    ledger_amount: Number(result.ledger_amount || result.amount || amount),
    payment_currency: paymentCurrencyCode,
    ledger_currency: ledgerCurrencyCode,
    fee_rate: Number(result.fee_rate || 0),
    payment_type: result.payment_type || 'paddle',
    out_trade_no: result.out_trade_no || '',
    status: (result.status || 'PENDING') as PaymentOrder['status'],
    order_type: (result.order_type || 'balance') as OrderType,
    created_at: result.created_at || new Date().toISOString(),
    expires_at: result.expires_at || '',
    refund_amount: Number(result.refund_amount || 0),
    plan_id: result.plan_id,
  }
}

async function resolvePaddleCheckoutOrder(resumeToken: string, outTradeNo: string): Promise<PaymentOrder | null> {
  try {
    if (resumeToken) {
      const result = await paymentAPI.resolveOrderPublicByResumeToken(resumeToken)
      return buildPaddleOrderFromPublicResult(result.data)
    }
    if (outTradeNo) {
      const result = await paymentAPI.verifyOrderPublic(outTradeNo)
      return buildPaddleOrderFromPublicResult(result.data)
    }
  } catch (_err: unknown) {
    return null
  }
  return null
}

function queryValue(value: unknown): string {
  if (typeof value === 'string') return value
  if (Array.isArray(value) && typeof value[0] === 'string') return value[0]
  return ''
}

async function resumeFirstPartyPaddleCheckoutFromQuery(): Promise<boolean> {
  const checkoutId = queryValue(route.query.checkout_id)
  if (!checkoutId) {
    return false
  }
  const orderId = Number(queryValue(route.query.order_id)) || 0
  const expiresAt = queryValue(route.query.expires_at)
  const outTradeNo = queryValue(route.query.out_trade_no)
  const resumeToken = queryValue(route.query.resume_token)
  const snapshot: PaymentRecoverySnapshot = {
    ...emptyPaymentState(),
    orderId,
    amount: 0,
    expiresAt,
    paymentType: 'paddle',
    checkoutId,
    outTradeNo,
    resumeToken,
    createdAt: Date.now(),
  }
  selectedMethod.value = 'paddle'
  paymentState.value = snapshot
  paddleCheckoutOrder.value = await resolvePaddleCheckoutOrder(resumeToken, outTradeNo)
  paymentPhase.value = 'paddle'
  persistRecoverySnapshot(snapshot)
  await router.replace({ path: route.path, query: { ...route.query, source: 'first_party_checkout' } })
  return true
}

function applyScenarioError(err: unknown, paymentMethod: string): boolean {
  const descriptor = describePaymentScenarioError(err, {
    paymentMethod,
    isMobile: isMobileDevice(),
    isWechatBrowser: typeof window !== 'undefined' && /MicroMessenger/i.test(window.navigator.userAgent),
  })
  if (!descriptor) {
    errorMessage.value = ''
    errorHintMessage.value = ''
    return false
  }
  errorMessage.value = t(descriptor.messageKey)
  errorHintMessage.value = descriptor.hintKey ? t(descriptor.hintKey) : ''
  appStore.showError(buildPaymentErrorToastMessage(errorMessage.value, errorHintMessage.value))
  return true
}

async function resumeWechatPaymentFromQuery() {
  const resume = parseWechatResumeRoute(route.query, checkout.value.plans, validAmount.value)
  if (!resume) {
    return
  }

  selectedMethod.value = resume.paymentType
  if (resume.orderType === 'balance' && resume.orderAmount > 0) {
    amount.value = resume.orderAmount
  }
  if (resume.orderType === 'subscription' && resume.planId) {
    selectedPlan.value = checkout.value.plans.find(plan => plan.id === resume.planId) ?? null
  }
  if (resume.paymentCurrency && availablePaymentCurrencies.value.includes(resume.paymentCurrency)) {
    selectedPaymentCurrency.value = resume.paymentCurrency
  }

  await router.replace({ path: route.path, query: stripWechatResumeQuery(route.query) })

  if (resume.wechatResumeToken) {
    await createOrder(0, resume.orderType, resume.planId, {
      wechatResumeToken: resume.wechatResumeToken,
      paymentType: resume.paymentType,
      paymentCurrency: resume.paymentCurrency,
      isResume: true,
      quoteId: resume.quoteId,
    })
    return
  }

  if (resume.orderAmount > 0 && resume.openid) {
    await createOrder(resume.orderAmount, resume.orderType, resume.planId, {
      openid: resume.openid,
      paymentType: resume.paymentType,
      paymentCurrency: resume.paymentCurrency,
      isResume: true,
      quoteId: resume.quoteId,
    })
  }
}

onMounted(async () => {
  try {
    const res = await paymentAPI.getCheckoutInfo()
    checkout.value = res.data
    selectedPaymentCurrency.value = availablePaymentCurrencies.value[0]
    selectDefaultMethodForCurrency()
    if (typeof window !== 'undefined') {
      if (hasWechatResumeQuery(route.query)) {
        removeRecoverySnapshot()
      }
      if (await resumeFirstPartyPaddleCheckoutFromQuery()) {
        loading.value = false
        return
      }
      const routeResumeToken = typeof route.query.resume_token === 'string'
        ? route.query.resume_token
        : typeof route.query.wechat_resume_token === 'string'
          ? route.query.wechat_resume_token
          : undefined
      const restored = readPaymentRecoverySnapshot(
        window.localStorage.getItem(PAYMENT_RECOVERY_STORAGE_KEY),
        { resumeToken: routeResumeToken },
      )
      if (restored) {
        paymentState.value = restored
        paymentPhase.value = 'paying'
        const restoredMethod = normalizeVisibleMethod(restored.paymentType)
          || (visibleMethods.value[restored.paymentType] ? restored.paymentType : '')
        if (restoredMethod) {
          selectedMethod.value = restoredMethod
        }
      } else {
        removeRecoverySnapshot()
      }
    }
    await resumeWechatPaymentFromQuery()
    if (checkout.value.balance_disabled) {
      activeTab.value = 'subscription'
    }
    // Handle renewal navigation: ?tab=subscription&group=123
    if (route.query.tab === 'subscription') {
      activeTab.value = 'subscription'
      if (route.query.group) {
        const groupId = Number(route.query.group)
        const groupPlans = checkout.value.plans.filter(p => p.group_id === groupId)
        if (groupPlans.length === 1) {
          selectedPlan.value = groupPlans[0]
        } else if (groupPlans.length > 1) {
          renewGroupId.value = groupId
          showRenewalModal.value = true
        }
      }
    }
  } catch (err: unknown) { appStore.showError(extractI18nErrorMessage(err, t, 'payment.errors', t('common.error'))) }
  finally { loading.value = false }
  // Fetch active subscriptions (uses cache, non-blocking)
  subscriptionStore.fetchActiveSubscriptions().catch(() => {})
})
</script>
