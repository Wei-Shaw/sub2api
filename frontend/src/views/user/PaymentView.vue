<template>
  <AppLayout>
    <div class="mx-auto max-w-4xl space-y-6">
      <div v-if="loading" class="flex items-center justify-center py-20">
        <div class="spinner h-8 w-8 text-accent"></div>
      </div>
      <!-- Checkout info failed to load: say so and offer a retry instead of claiming top-up is unavailable -->
      <div v-else-if="checkoutError" class="card empty-state">
        <p class="empty-state-description">{{ checkoutError }}</p>
        <button class="btn btn-secondary mt-4" @click="loadCheckout">{{ t('common.refresh') }}</button>
      </div>
      <template v-else>
        <!-- Tab Switcher (hide during payment and subscription confirm) -->
        <div v-if="tabs.length > 1 && paymentPhase === 'select' && !selectedPlan" class="tabs">
          <button v-for="tab in tabs" :key="tab.key"
            :class="['tab min-h-[40px] flex-1 sm:flex-none', activeTab === tab.key ? 'tab-active' : '']"
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
            :out-trade-no="paymentState.outTradeNo"
            :gateway-return-status="gatewayReturnStatus"
            @done="onPaymentDone"
            @success="onPaymentSuccess"
            @settled="onPaymentSettled"
          />
        </template>
        <!-- Tab content (select phase) -->
        <template v-else>
          <!-- Neither top-up nor subscriptions available (balance recharge disabled via API while subscriptions are off) -->
          <div v-if="tabs.length === 0" class="card empty-state">
            <p class="empty-state-description">{{ t('payment.billingUnavailable') }}</p>
          </div>
          <!-- Top-up Tab -->
          <template v-else-if="activeTab === 'recharge'">
            <!-- Recharge account: account reading + current balance (yellow = current reading) -->
            <div class="meter">
              <div class="meter-cell">
                <span class="meter-label">{{ t('payment.rechargeAccount') }}</span>
                <span class="truncate text-h3 font-bold text-fg">{{ user?.username || user?.email || '' }}</span>
              </div>
              <div class="meter-cell meter-cell-current">
                <span class="meter-label">{{ t('payment.currentBalance') }}</span>
                <span class="meter-value">${{ user?.balance?.toFixed(2) || '0.00' }}</span>
              </div>
            </div>
            <div v-if="enabledMethods.length === 0" class="card empty-state">
              <p class="empty-state-description">{{ t('payment.notAvailable') }}</p>
            </div>
            <template v-else>
            <section class="card">
              <div class="card-body space-y-5">
                <div
                  v-if="renderedBonusNotice"
                  class="markdown-body border border-accent bg-accent-weak px-4 py-3 text-body text-fg"
                  data-testid="recharge-bonus-notice"
                  v-html="renderedBonusNotice"
                ></div>
                <div>
                  <AmountInput
                    v-model="amount"
                    v-model:currency="inputCurrency"
                    :amounts="quickAmounts"
                    :min="inputMinAmount"
                    :max="inputMaxAmount"
                    :currency-options="currencyOptions"
                  />
                  <p v-if="exchangeRateError" class="input-hint text-warning">{{ exchangeRateError }}</p>
                  <p v-else-if="conversionHint" class="input-hint">{{ conversionHint }}</p>
                  <p v-if="amountError" class="input-hint text-warning">{{ amountError }}</p>
                </div>
                <div v-if="enabledMethods.length >= 1" class="border-t border-border pt-5">
                  <PaymentMethodSelector
                    :methods="methodOptions"
                    :selected="selectedMethod"
                    @select="selectedMethod = $event"
                  />
                </div>
              </div>
              <!-- Itemized charge: printed rows, total on a double rule -->
              <div v-if="validAmount > 0" class="border-t border-border">
                <table class="table">
                  <tbody>
                    <tr>
                      <td class="text-fg-muted">{{ t('payment.paymentAmount') }}</td>
                      <td class="text-right tabular-nums" :class="{ 'text-fg-subtle line-through': discountAmount > 0 }">{{ formatSelectedPaymentAmount(validAmount) }}</td>
                    </tr>
                    <tr v-if="discountAmount > 0" data-testid="recharge-discount-row">
                      <td class="text-fg-muted">{{ t('payment.rechargeBonus.discountLabelWithPercent', { percent: formatRechargeBonusNumber(bonusQuote.percent) }) }}</td>
                      <td class="text-right tabular-nums text-accent-strong">-{{ formatSelectedPaymentAmount(discountAmount) }}</td>
                    </tr>
                    <tr v-if="feeRate > 0">
                      <td class="text-fg-muted">{{ t('payment.fee') }} ({{ feeRate }}%)</td>
                      <td class="text-right tabular-nums">{{ formatSelectedPaymentAmount(feeAmount) }}</td>
                    </tr>
                    <tr v-if="feeRate > 0 || discountAmount > 0" class="row-total">
                      <td>{{ t('payment.actualPay') }}</td>
                      <td class="text-right text-h2 tabular-nums text-accent-strong">{{ formatSelectedPaymentAmount(totalAmount) }}</td>
                    </tr>
                    <tr v-if="bonusQuote.mode !== 'discount' && bonusQuote.bonus > 0" data-testid="recharge-bonus-row">
                      <td class="text-fg-muted">{{ t('payment.rechargeBonus.amountLabelWithPercent', { percent: formatRechargeBonusNumber(bonusQuote.percent) }) }}</td>
                      <td class="text-right tabular-nums text-accent-strong">+${{ bonusQuote.bonus.toFixed(2) }}</td>
                    </tr>
                    <tr :class="{ 'row-total': feeRate <= 0 && discountAmount <= 0 }">
                      <td :class="feeRate <= 0 && discountAmount <= 0 ? '' : 'text-fg-muted'">{{ t('payment.creditedBalance') }}</td>
                      <td class="text-right tabular-nums">${{ creditedAmount.toFixed(2) }}</td>
                    </tr>
                  </tbody>
                </table>
                <p v-if="exchangeRateText || balanceRechargeMultiplier !== 1" class="space-y-1 border-t border-border bg-surface-sunken px-3 py-2 text-meta text-fg-muted">
                  <span v-if="exchangeRateText" class="block">{{ exchangeRateText }}</span>
                  <span v-if="balanceRechargeMultiplier !== 1" class="block">
                    {{ t('payment.rechargeRatePreview', { currency: selectedCurrency, usd: balanceRechargeMultiplier.toFixed(2) }) }}
                  </span>
                </p>
              </div>
            </section>
            <button :class="['btn btn-lg min-h-[48px] w-full', paymentButtonClass]" :disabled="!canSubmit || submitting" @click="handleSubmitRecharge">
              <span v-if="submitting" class="flex items-center justify-center gap-2">
                <span class="spinner h-4 w-4"></span>
                {{ t('common.processing') }}
              </span>
              <span v-else class="tabular-nums">{{ t('payment.createOrder') }} {{ formatSelectedPaymentAmount(totalAmount) }}</span>
            </button>
            </template>
          </template>
          <!-- Subscribe Tab -->
          <template v-else-if="activeTab === 'subscription'">
            <!-- Subscription confirm (inline, replaces plan list) -->
            <template v-if="selectedPlan">
              <section class="card">
                <!-- Header: platform stamp + plan name -->
                <div class="card-header flex flex-wrap items-center gap-2">
                  <h3 class="card-title">{{ selectedPlan.name }}</h3>
                  <span class="badge">
                    {{ platformLabel(selectedPlan.group_platform || '') }}
                  </span>
                </div>
                <div class="card-body">
                  <!-- Price -->
                  <div class="flex flex-wrap items-baseline gap-2">
                    <span v-if="selectedPlan.original_price" class="text-body tabular-nums text-fg-subtle line-through">
                      {{ formatSelectedSubscriptionPaymentAmount(selectedPlan.original_price) }}
                    </span>
                    <span class="text-display font-bold tabular-nums text-fg">{{ formatSelectedSubscriptionPaymentAmount(selectedPlan.price) }}</span>
                    <span class="text-body text-fg-muted">/ {{ planValiditySuffix }}</span>
                  </div>
                  <!-- Description -->
                  <p v-if="selectedPlan.description" class="mt-2 text-body text-fg-muted">
                    {{ selectedPlan.description }}
                  </p>
                </div>
                <!-- Rate + Limits: ruled terms -->
                <dl class="divide-y divide-border border-t border-border text-body">
                  <div class="flex items-center justify-between gap-3 px-5 py-2">
                    <dt class="text-fg-muted">{{ t('payment.planCard.rate') }}</dt>
                    <dd class="font-bold tabular-nums text-fg">×{{ selectedPlan.rate_multiplier ?? 1 }}</dd>
                  </div>
                  <div v-if="planHasPeakRate(selectedPlan)" class="flex items-center justify-between gap-3 px-5 py-2">
                    <dt class="text-fg-muted">{{ t('payment.planCard.peakRate') }}</dt>
                    <dd class="text-right font-semibold text-warning">{{ planPeakRateLabel(selectedPlan) }}</dd>
                  </div>
                  <div v-if="selectedPlan.daily_limit_usd != null" class="flex items-center justify-between gap-3 px-5 py-2">
                    <dt class="text-fg-muted">{{ t('payment.planCard.dailyLimit') }}</dt>
                    <dd class="font-semibold tabular-nums text-fg">${{ selectedPlan.daily_limit_usd }}</dd>
                  </div>
                  <div v-if="selectedPlan.weekly_limit_usd != null" class="flex items-center justify-between gap-3 px-5 py-2">
                    <dt class="text-fg-muted">{{ t('payment.planCard.weeklyLimit') }}</dt>
                    <dd class="font-semibold tabular-nums text-fg">${{ selectedPlan.weekly_limit_usd }}</dd>
                  </div>
                  <div v-if="selectedPlan.monthly_limit_usd != null" class="flex items-center justify-between gap-3 px-5 py-2">
                    <dt class="text-fg-muted">{{ t('payment.planCard.monthlyLimit') }}</dt>
                    <dd class="font-semibold tabular-nums text-fg">${{ selectedPlan.monthly_limit_usd }}</dd>
                  </div>
                  <div v-if="selectedPlan.daily_limit_usd == null && selectedPlan.weekly_limit_usd == null && selectedPlan.monthly_limit_usd == null" class="flex items-center justify-between gap-3 px-5 py-2">
                    <dt class="text-fg-muted">{{ t('payment.planCard.quota') }}</dt>
                    <dd class="font-semibold text-fg">{{ t('payment.planCard.unlimited') }}</dd>
                  </div>
                </dl>
                <div v-if="enabledMethods.length >= 1" class="border-t border-border p-5">
                  <PaymentMethodSelector
                    :methods="subMethodOptions"
                    :selected="selectedMethod"
                    @select="selectedMethod = $event"
                  />
                </div>
                <table v-if="feeRate > 0 && selectedPlan.price > 0" class="table border-t border-border">
                  <tbody>
                    <tr>
                      <td class="text-fg-muted">{{ t('payment.amountLabel') }}</td>
                      <td class="text-right tabular-nums">{{ formatSelectedPaymentAmount(subPaymentAmount) }}</td>
                    </tr>
                    <tr>
                      <td class="text-fg-muted">{{ t('payment.fee') }} ({{ feeRate }}%)</td>
                      <td class="text-right tabular-nums">{{ formatSelectedPaymentAmount(subFeeAmount) }}</td>
                    </tr>
                    <tr class="row-total">
                      <td>{{ t('payment.actualPay') }}</td>
                      <td class="text-right text-h2 tabular-nums text-accent-strong">{{ formatSelectedPaymentAmount(subTotalAmount) }}</td>
                    </tr>
                  </tbody>
                </table>
              </section>
              <button :class="['btn btn-lg min-h-[48px] w-full', paymentButtonClass]" :disabled="!canSubmitSubscription || submitting" @click="confirmSubscribe">
                <span v-if="submitting" class="flex items-center justify-center gap-2">
                  <span class="spinner h-4 w-4"></span>
                  {{ t('common.processing') }}
                </span>
                <span v-else class="tabular-nums">{{ t('payment.createOrder') }} {{ formatSelectedPaymentAmount(subTotalAmount) }}</span>
              </button>
              <button class="btn btn-secondary min-h-[40px] w-full" @click="selectedPlan = null">{{ t('common.cancel') }}</button>
            </template>
            <!-- Plan list -->
            <template v-else>
              <div v-if="checkout.plans.length === 0" class="card empty-state">
                <Icon name="gift" size="xl" class="empty-state-icon" />
                <p class="empty-state-description">{{ t('payment.noPlans') }}</p>
              </div>
              <!-- Tier table: each plan is a printed column -->
              <div v-else :class="planGridClass">
                <SubscriptionPlanCard v-for="plan in checkout.plans" :key="plan.id" :plan="plan" :active-subscriptions="activeSubscriptions" @select="selectPlan" />
              </div>
              <!-- Active subscriptions (ruled register, below plan list) -->
              <section v-if="activeSubscriptions.length > 0">
                <h2 class="bill-section-title">{{ t('payment.activeSubscription') }}</h2>
                <div class="divide-y divide-border border-x border-b border-border bg-surface">
                  <div v-for="sub in activeSubscriptions" :key="sub.id"
                    class="flex items-center gap-3 px-3 py-2">
                    <div class="min-w-0 flex-1">
                      <div class="flex items-center gap-1.5">
                        <span class="truncate text-label font-semibold text-fg">{{ sub.group?.name || t('payment.groupFallback', { id: sub.group_id }) }}</span>
                        <span class="badge shrink-0">{{ platformLabel(sub.group?.platform || '') }}</span>
                      </div>
                      <div class="flex flex-wrap gap-x-3 text-meta tabular-nums text-fg-muted">
                        <span>{{ t('payment.planCard.rate') }}: ×{{ sub.group?.rate_multiplier ?? 1 }}</span>
                        <span v-if="subscriptionHasPeakRate(sub)">{{ t('payment.planCard.peakRate') }}: {{ subscriptionPeakRateLabel(sub) }}</span>
                        <span v-if="sub.group?.daily_limit_usd == null && sub.group?.weekly_limit_usd == null && sub.group?.monthly_limit_usd == null">{{ t('payment.planCard.quota') }}: {{ t('payment.planCard.unlimited') }}</span>
                        <span v-if="sub.expires_at">{{ t('userSubscriptions.daysRemaining', { days: getDaysRemaining(sub.expires_at) }) }}</span>
                        <span v-else>{{ t('userSubscriptions.noExpiration') }}</span>
                      </div>
                    </div>
                    <span class="badge badge-success shrink-0">{{ t('userSubscriptions.status.active') }}</span>
                  </div>
                </div>
              </section>
            </template>
          </template>
        </template>
        <div v-if="(checkout.help_text || checkout.help_image_url) && paymentPhase === 'select' && !selectedPlan" class="card card-body">
          <div class="flex flex-col items-center gap-3">
            <img v-if="checkout.help_image_url" :src="checkout.help_image_url" alt=""
              class="h-40 max-w-full cursor-pointer border border-border object-contain transition-opacity hover:opacity-80"
              @click="previewImage = checkout.help_image_url" />
            <div v-if="checkout.help_text" class="markdown-body w-full overflow-x-auto break-words" v-html="renderedHelpText"></div>
          </div>
        </div>
      </template>
    </div>
    <!-- Renewal Plan Selection Modal -->
    <BaseDialog :show="showRenewalModal" :title="t('payment.selectPlan')" close-on-click-outside @close="closeRenewalModal">
      <div class="pl-px pt-px">
        <SubscriptionPlanCard v-for="plan in renewalPlans" :key="plan.id" :plan="plan" :active-subscriptions="activeSubscriptions" @select="selectPlanFromModal" />
      </div>
    </BaseDialog>
    <!-- Image Preview Overlay -->
    <!-- design-system: raw overlay kept — image lightbox (click-anywhere-to-dismiss, no title/panel), not a dialog -->
    <Teleport to="body">
      <Transition name="modal">
        <div v-if="previewImage" class="fixed inset-0 z-[60] flex items-center justify-center bg-black/70" @click="previewImage = ''">
          <img :src="previewImage" alt="" class="max-h-[85vh] max-w-[90vw] rounded-sm object-contain shadow-overlay" />
        </div>
      </Transition>
    </Teleport>
  </AppLayout>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { marked } from 'marked'
import DOMPurify from 'dompurify'
import '@/styles/announcement-markdown.css'
import { useRoute, useRouter } from 'vue-router'
import { useAuthStore } from '@/stores/auth'
import { usePaymentStore } from '@/stores/payment'
import { useSubscriptionStore } from '@/stores/subscriptions'
import { useAppStore } from '@/stores'
import { FeatureFlags, resolveFeatureFlag } from '@/utils/featureFlags'
import { paymentAPI } from '@/api/payment'
import { extractApiErrorMessage, extractI18nErrorMessage } from '@/utils/apiError'
import { isMobileDevice } from '@/utils/device'
import { hasPeakRate, formatPeakRateWindow, serverTimezoneLabel, type PeakRateFields } from '@/utils/peak-rate'
import type { SubscriptionPlan, CheckoutInfoResponse, CreateOrderResult, OrderType } from '@/types/payment'
import { formatRechargeBonusNumber, normalizeRechargeBonusMode, normalizeRechargeBonusTiers, quoteRechargeBonus } from '@/utils/rechargeBonus'
import AppLayout from '@/components/layout/AppLayout.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import AmountInput from '@/components/payment/AmountInput.vue'
import PaymentMethodSelector from '@/components/payment/PaymentMethodSelector.vue'
import { METHOD_ORDER, getPaymentPopupFeatures } from '@/components/payment/providerConfig'
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
import { platformLabel } from '@/utils/platformColors'
import SubscriptionPlanCard from '@/components/payment/SubscriptionPlanCard.vue'
import PaymentStatusPanel from '@/components/payment/PaymentStatusPanel.vue'
import Icon from '@/components/icons/Icon.vue'
import { DEFAULT_PAYMENT_CURRENCY, formatPaymentAmount, normalizePaymentCurrency } from '@/components/payment/currency'
import { planValiditySuffix as validitySuffixOf } from '@/components/payment/validity'
import type { PaymentMethodOption } from '@/components/payment/PaymentMethodSelector.vue'
import { buildPaymentErrorToastMessage, describePaymentScenarioError } from './paymentUx'
import { listenForPopupResult } from './paymentPopupBridge'

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
const checkoutError = ref('')
const submitting = ref(false)
const errorMessage = ref('')
const errorHintMessage = ref('')
const activeTab = ref<'recharge' | 'subscription'>('recharge')
const amount = ref<number | null>(null)
const selectedMethod = ref('')
const selectedPlan = ref<SubscriptionPlan | null>(null)
const previewImage = ref('')

const paymentPhase = ref<'select' | 'paying'>('select')
// 网关回跳时带回来的 status，转交给支付面板提前收尾用。每开一张新单都要清掉，
// 否则上一单的「失败」会立刻把新单也判死。
const gatewayReturnStatus = ref('')

interface CreateOrderOptions {
  paymentType?: string
  isResume?: boolean
  amountCurrency?: string
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
    currency: '',
    paymentEnv: '',
    payAmount: 0,
    orderType: '',
    paymentMode: '',
    resumeToken: '',
    createdAt: 0,
  }
}

const paymentState = ref<PaymentRecoverySnapshot>(emptyPaymentState())

function persistRecoverySnapshot(snapshot: PaymentRecoverySnapshot) {
  if (typeof window === 'undefined' || !snapshot.orderId) return
  // 记下下单用户，换号后不会把上一个用户的待支付订单恢复出来
  writePaymentRecoverySnapshot(window.localStorage, { ...snapshot, userId: authStore.user?.id }, PAYMENT_RECOVERY_STORAGE_KEY)
}

function removeRecoverySnapshot() {
  if (typeof window === 'undefined') return
  clearPaymentRecoverySnapshot(window.localStorage, PAYMENT_RECOVERY_STORAGE_KEY)
}

function resetPayment() {
  paymentPhase.value = 'select'
  gatewayReturnStatus.value = ''
  paymentState.value = emptyPaymentState()
  removeRecoverySnapshot()
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
  authStore.refreshUser({ force: true })
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
  plans: [], balance_disabled: false, balance_recharge_multiplier: 1, subscription_usd_to_cny_rate: 0, recharge_fee_rate: 0, help_text: '', help_image_url: '',
})

const renderedHelpText = computed(() => DOMPurify.sanitize(
  marked.parse(checkout.value.help_text || '', { async: false, gfm: true, breaks: false }),
))

// 订阅功能开关（public settings 的 subscription_enabled，opt-out）。关闭后购买页只保留充值：
// 不再渲染「订阅」tab，只剩单个 tab 时顶部切换器也随之隐藏。
const subscriptionEnabled = computed(() => resolveFeatureFlag(appStore.cachedPublicSettings, FeatureFlags.subscription))

const tabs = computed(() => {
  const result: { key: 'recharge' | 'subscription'; label: string }[] = []
  if (!checkout.value.balance_disabled) result.push({ key: 'recharge', label: t('payment.tabTopUp') })
  if (subscriptionEnabled.value) result.push({ key: 'subscription', label: t('payment.tabSubscribe') })
  return result
})

// tab 列表随 checkout（balance_disabled）与订阅开关变化。当前 tab 不在列表里时收敛到第一个可用 tab，
// 两个方向都覆盖：关闭订阅 → 回到充值；仅订阅站点重新打开订阅 → 进入订阅。列表为空时模板展示不可用提示。
watch(tabs, (available) => {
  if (available.some((tab) => tab.key === activeTab.value)) return
  const leavingSubscription = activeTab.value === 'subscription'
  activeTab.value = available[0]?.key ?? 'recharge'
  if (leavingSubscription) selectedPlan.value = null
})

const visibleMethods = computed(() => getVisibleMethods(checkout.value.methods))
const enabledMethods = computed(() => Object.keys(visibleMethods.value))

// 用户可以按网关结算币种（VND）或 USD 填金额，两者通过越南外贸银行牌价换算。
// 这里只做预览，真正定价在后端用同一个汇率完成。
const inputCurrency = ref(DEFAULT_PAYMENT_CURRENCY)
const exchangeRate = ref(0)
const exchangeRateError = ref('')

const currencyOptions = computed(() => {
  const gateway = selectedCurrency.value
  if (gateway === 'USD') return [gateway]
  return exchangeRate.value > 0 ? [gateway, 'USD'] : [gateway]
})

/** 用户填的数额，单位是 inputCurrency。 */
const typedAmount = computed(() => amount.value ?? 0)

/** 折算成网关结算币种后要收的钱；后端用 RoundUp，这里镜像它。 */
const validAmount = computed(() => {
  if (inputCurrency.value === selectedCurrency.value) return typedAmount.value
  if (exchangeRate.value <= 0) return 0
  return ceilPaymentAmount(typedAmount.value * exchangeRate.value, selectedCurrency.value)
})

/** 折算成 USD 后要记的余额；后端向下取到分，这里同样向下。 */
const usdAmount = computed(() => {
  if (inputCurrency.value === 'USD') return typedAmount.value
  if (selectedCurrency.value === 'USD') return typedAmount.value
  if (exchangeRate.value <= 0) return 0
  return Math.floor((typedAmount.value / exchangeRate.value) * 100) / 100
})

const quickAmounts = computed(() =>
  inputCurrency.value === 'USD'
    ? [5, 10, 20, 50, 100, 200, 500]
    : [100000, 200000, 500000, 1000000, 2000000, 5000000]
)

/** 限额是按网关币种配置的，用户按 USD 填时要把边界折回去。 */
function limitToInputCurrency(limit: number): number {
  if (limit <= 0) return 0
  if (inputCurrency.value === selectedCurrency.value || exchangeRate.value <= 0) return limit
  return Math.ceil((limit / exchangeRate.value) * 100) / 100
}
// 限额取当前网关自己的，不取跨网关的并集：两个网关币种不同时，
// min/max 的 Math.min/max 会把 VND 和 USD 的数字混在一起比。
const inputMinAmount = computed(() => limitToInputCurrency(selectedLimit.value?.single_min ?? globalMinAmount.value))
const inputMaxAmount = computed(() => limitToInputCurrency(selectedLimit.value?.single_max ?? globalMaxAmount.value))

const exchangeRateText = computed(() => {
  if (exchangeRate.value <= 0 || selectedCurrency.value === 'USD') return ''
  return t('payment.exchangeRateNote', {
    rate: formatPaymentAmount(exchangeRate.value, selectedCurrency.value, localeCode.value),
  })
})

const conversionHint = computed(() => {
  if (typedAmount.value <= 0 || inputCurrency.value === selectedCurrency.value) return ''
  if (exchangeRate.value <= 0) return ''
  return t('payment.convertedAmount', {
    amount: formatPaymentAmount(validAmount.value, selectedCurrency.value, localeCode.value),
  })
})

async function loadExchangeRate() {
  const method = selectedMethod.value
  if (!method) return
  try {
    const { data: info } = await paymentAPI.getExchangeRate(method)
    const parsed = Number(info.rate)
    exchangeRate.value = Number.isFinite(parsed) && parsed > 0 ? parsed : 0
    exchangeRateError.value = ''
  } catch (err) {
    // 汇率拿不到时只允许按网关币种充值：不要拿一个猜的价格给用户看。
    exchangeRate.value = 0
    exchangeRateError.value = extractApiErrorMessage(err) || t('payment.exchangeRateUnavailable')
  }
  if (!currencyOptions.value.includes(inputCurrency.value)) {
    inputCurrency.value = selectedCurrency.value
  }
}

const balanceRechargeMultiplier = computed(() => {
  const multiplier = checkout.value.balance_recharge_multiplier
  return Number.isFinite(multiplier) && multiplier > 0 ? multiplier : 1
})
// 订阅 CNY 换算汇率（1 USD = X CNY）。0 = 未配置，订阅保持 price 直付（与后端 opt-in 条件严格镜像）。
const subscriptionUsdToCnyRate = computed(() => {
  const rate = checkout.value.subscription_usd_to_cny_rate
  return Number.isFinite(rate) && rate > 0 ? rate : 0
})
const baseCreditedAmount = computed(() => Math.round((usdAmount.value * balanceRechargeMultiplier.value) * 100) / 100)

// 充值优惠预览，镜像后端 quoteRechargeBonusConverted：阶梯按网关币种金额命中，
// 赠送/折扣落在 USD 到账基数上（等效倍率 = 到账基数 / 网关金额）。
const bonusQuote = computed(() => {
  const gateway = validAmount.value
  const base = baseCreditedAmount.value
  const tiers = normalizeRechargeBonusTiers(checkout.value.recharge_bonus_tiers)
  const mode = normalizeRechargeBonusMode(checkout.value.recharge_bonus_mode)
  // 到账基数为 0 时倍率会退化成 1（把 VND 当 USD），与后端一样直接不报优惠。
  if (gateway <= 0 || base <= 0) return { ...quoteRechargeBonus([], gateway, { mode }), base, credited: base }
  return quoteRechargeBonus(tiers, gateway, {
    multiplier: base / gateway,
    mode,
    currencyDigits: currencyFractionDigits(selectedCurrency.value),
  })
})
const payBaseAmount = computed(() => bonusQuote.value.payBase)
const discountAmount = computed(() => roundPaymentAmount(validAmount.value - payBaseAmount.value, selectedCurrency.value))
const creditedAmount = computed(() => bonusQuote.value.credited)
const renderedBonusNotice = computed(() => {
  const raw = (checkout.value.recharge_bonus_notice || '').trim()
  if (!raw) return ''
  return DOMPurify.sanitize(marked.parse(raw, { async: false, gfm: true, breaks: true }))
})

// Tier table: plans are printed columns sharing hairlines (cells use -ml-px/-mt-px).
// 2-col for up to 2 plans, 3-col for 3+.
const planGridClass = computed(() => {
  const n = checkout.value.plans.length
  if (n <= 2) return 'grid grid-cols-1 pl-px pt-px sm:grid-cols-2'
  return 'grid grid-cols-1 pl-px pt-px sm:grid-cols-2 lg:grid-cols-3'
})

// Check if an amount fits a method's [min, max]. 0 = no limit.
function amountFitsMethod(amt: number, methodType: string): boolean {
  if (amt <= 0) return true
  const ml = visibleMethods.value[methodType]
  if (!ml) return false
  if (ml.single_min > 0 && amt < ml.single_min) return false
  if (ml.single_max > 0 && amt > ml.single_max) return false
  return true
}

// Visible methods decide the amount range shown to users.
const globalMinAmount = computed(() => {
  const limits = Object.values(visibleMethods.value)
  if (limits.length === 0) return 0
  if (limits.some(limit => limit.single_min <= 0)) return 0
  return Math.min(...limits.map(limit => limit.single_min))
})
const globalMaxAmount = computed(() => {
  const limits = Object.values(visibleMethods.value)
  if (limits.length === 0) return 0
  if (limits.some(limit => limit.single_max <= 0)) return 0
  return Math.max(...limits.map(limit => limit.single_max))
})

// Selected method's limits (for validation and error messages)
const selectedLimit = computed(() => visibleMethods.value[selectedMethod.value])
const selectedCurrency = computed(() => normalizePaymentCurrency(selectedLimit.value?.currency))

watch(selectedCurrency, (currency, previous) => {
  inputCurrency.value = currency
  // 换网关就等于换计价币种，填过的数字不再是同一个意思：100000 VND 留在
  // 按 USD 结算的加密货币网关上会变成 100000 USD 打过去。清掉让用户重填。
  if (previous && currency !== previous) amount.value = null
})
watch(selectedMethod, () => {
  void loadExchangeRate()
})

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

// 套餐以 USD 定价。网关按别的币种结算时一定要换算：管理员配的订阅汇率优先，
// 没配就用实时牌价，两条路都镜像后端。拿不到汇率就返回 0 挡住下单，
// 绝不按面值直付——那会把 100 USD 的套餐卖成 100 VND。
function subscriptionPaymentAmountForCurrency(value: number, currency: string): number {
  if (currency === 'USD') return roundPaymentAmount(value, currency)
  const configured = subscriptionUsdToCnyRate.value
  if (configured > 0) return roundPaymentAmount(value * configured, currency)
  if (exchangeRate.value <= 0) return 0
  return ceilPaymentAmount(value * exchangeRate.value, currency)
}

function formatSelectedPaymentAmount(value: number): string {
  return formatPaymentAmount(value, selectedCurrency.value, localeCode.value)
}

function formatSelectedSubscriptionPaymentAmount(value: number): string {
  return formatSelectedPaymentAmount(subscriptionPaymentAmountForCurrency(value, selectedCurrency.value))
}

const methodOptions = computed<PaymentMethodOption[]>(() =>
  enabledMethods.value.map((type) => {
    const ml = visibleMethods.value[type]
    return {
      type,
      display_name: ml?.display_name,
      fee_rate: ml?.fee_rate ?? 0,
      available: ml?.available !== false && amountFitsMethod(validAmount.value, type),
    }
  })
)

const feeRate = computed(() => checkout.value?.recharge_fee_rate ?? 0)
// 手续费与实付按折后基数算（后端 limitAmount = quote.PayBase）；赠金模式下 payBase 就是 validAmount。
const feeAmount = computed(() =>
  feeRate.value > 0 && payBaseAmount.value > 0
    ? Math.ceil(((payBaseAmount.value * feeRate.value) / 100) * 100) / 100
    : 0
)
const totalAmount = computed(() =>
  feeRate.value > 0 && payBaseAmount.value > 0
    ? Math.round((payBaseAmount.value + feeAmount.value) * 100) / 100
    : payBaseAmount.value
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
    if (ml.single_min > 0 && validAmount.value < ml.single_min) return t('payment.amountTooLow', { min: formatSelectedPaymentAmount(ml.single_min) })
    if (ml.single_max > 0 && validAmount.value > ml.single_max) return t('payment.amountTooHigh', { max: formatSelectedPaymentAmount(ml.single_max) })
  }
  return ''
})

const canSubmit = computed(() =>
  validAmount.value > 0
    && amountFitsMethod(validAmount.value, selectedMethod.value)
    && selectedLimit.value?.available !== false
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
  const price = selectedPlan.value?.price ?? 0
  return enabledMethods.value.map((type) => {
    const ml = visibleMethods.value[type]
    const currency = normalizePaymentCurrency(ml?.currency)
    return {
      type,
      display_name: ml?.display_name,
      fee_rate: ml?.fee_rate ?? 0,
      available: ml?.available !== false && amountFitsMethod(subscriptionTotalAmountForCurrency(price, currency), type),
    }
  })
})

const canSubmitSubscription = computed(() =>
  selectedPlan.value !== null
    && amountFitsMethod(subTotalAmount.value, selectedMethod.value)
    && selectedLimit.value?.available !== false
)

// Auto-switch to first available method when current selection can't handle the amount
watch(() => [validAmount.value, selectedMethod.value] as const, ([amt, method]) => {
  if (amt <= 0 || amountFitsMethod(amt, method)) return
  const available = enabledMethods.value.find((m) => amountFitsMethod(amt, m))
  if (available) selectedMethod.value = available
})

const paymentButtonClass = 'btn-primary'

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
  await createOrder(typedAmount.value, 'balance', undefined, { amountCurrency: inputCurrency.value })
}

async function confirmSubscribe() {
  if (!selectedPlan.value || submitting.value) return
  await createOrder(selectedPlan.value.price, 'subscription', selectedPlan.value.id)
}

async function createOrder(orderAmount: number, orderType: OrderType, planId?: number, options: CreateOrderOptions = {}) {
  submitting.value = true
  errorMessage.value = ''
  errorHintMessage.value = ''
  const requestType = normalizeVisibleMethod(options.paymentType || selectedMethod.value) || options.paymentType || selectedMethod.value
  try {
    const payload = buildCreateOrderPayload({
      amount: orderAmount,
      amountCurrency: options.amountCurrency,
      paymentType: requestType,
      orderType,
      planId,
      origin: typeof window !== 'undefined' ? window.location.origin : '',
      isMobile: isMobileDevice(),
    })

    const result = await paymentStore.createOrder(payload) as CreateOrderResult & { resume_token?: string }
    const openWindow = (url: string) => {
      const win = window.open(url, 'paymentPopup', getPaymentPopupFeatures())
      if (!win || win.closed) {
        window.location.href = url
      }
    }
    const visibleMethod = normalizeVisibleMethod(requestType) || requestType
    const decision = decidePaymentLaunch(result, {
      visibleMethod,
      orderType,
      isMobile: isMobileDevice(),
    })

    if (decision.kind === 'unhandled') {
      applyScenarioError({ reason: 'UNHANDLED_PAYMENT_SCENARIO' }, visibleMethod)
      return
    }

    paymentState.value = decision.paymentState
    gatewayReturnStatus.value = ''
    paymentPhase.value = 'paying'
    persistRecoverySnapshot(decision.recovery)

    if (decision.kind === 'redirect_waiting' && decision.paymentState.payUrl) {
      // The SePay checkout is reached through a signed POST form served by our
      // own bridge page, so a popup that the browser blocks must still fall
      // back to a same-tab navigation rather than stranding the order.
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

function applyScenarioError(err: unknown, paymentMethod: string): boolean {
  const descriptor = describePaymentScenarioError(err, {
    paymentMethod,
    isMobile: isMobileDevice(),
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

// The checkout popup posts the order back here when the gateway returns, so
// the result lands in this window rather than in a popup the user then has to
// close by hand. The query only addresses the order; the result page reloads
// its real status from the backend.
let stopPopupResultListener: (() => void) | null = null

onMounted(() => {
  stopPopupResultListener = listenForPopupResult((query) => {
    // 支付面板本来就开在充值卡片里，而且一直在轮询这张单。用户从网关回来时
    // 把他推去一个独立的结果页，等于让他离开正在看的东西再看一遍同一件事。
    // 面板还在的话就把网关的说法交给它，人留在原地。
    if (paymentPhase.value === 'paying') {
      gatewayReturnStatus.value = String(query.status || '').trim().toLowerCase()
      return
    }
    // 面板已经不在了（比如中途刷新过），只能退回结果页。
    removeRecoverySnapshot()
    router.push({ path: '/payment/result', query }).catch(() => {})
  })
})

onUnmounted(() => {
  stopPopupResultListener?.()
  stopPopupResultListener = null
})

async function loadCheckout() {
  loading.value = true
  checkoutError.value = ''
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
    if (typeof window !== 'undefined') {
      const routeResumeToken = typeof route.query.resume_token === 'string'
        ? route.query.resume_token
        : undefined
      const restored = readPaymentRecoverySnapshot(
        window.localStorage.getItem(PAYMENT_RECOVERY_STORAGE_KEY),
        { resumeToken: routeResumeToken, userId: authStore.user?.id },
      )
      if (restored) {
        paymentState.value = restored
        // 从网关回跳过来时 URL 上带着结果，交给面板收尾——用户是被送回这一页的，
        // 不是自己刷新出来的。
        gatewayReturnStatus.value = typeof route.query.status === 'string'
          ? route.query.status.trim().toLowerCase()
          : ''
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
    // balance_disabled → the tabs watcher above moves activeTab to the subscription tab (when enabled).
    // Handle renewal navigation: ?tab=subscription&group=123 (ignored when subscriptions are disabled)
    if (route.query.tab === 'subscription' && subscriptionEnabled.value) {
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
  } catch (err: unknown) {
    checkoutError.value = extractI18nErrorMessage(err, t, 'payment.errors', t('common.error'))
    appStore.showError(checkoutError.value)
  }
  finally { loading.value = false }
}

onMounted(async () => {
  await loadCheckout()
  // Fetch active subscriptions (uses cache, non-blocking); skipped when the subscription feature is off
  if (subscriptionEnabled.value) {
    subscriptionStore.fetchActiveSubscriptions().catch(() => {})
  }
})
</script>
