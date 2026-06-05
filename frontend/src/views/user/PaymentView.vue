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
            class="relative flex-1 rounded-lg px-4 py-2.5 text-sm font-medium transition-all"
            :class="activeTab === tab.key ? 'bg-white text-gray-900 shadow dark:bg-dark-700 dark:text-white' : 'text-gray-500 hover:text-gray-700 dark:text-gray-400 dark:hover:text-gray-300'"
            @click="onTabClicked(tab.key)">
            {{ tab.label }}
            <span
              v-if="tab.key === 'recharge' && showRechargeTabDot"
              class="absolute right-1.5 top-1.5 inline-block h-3 w-3 rounded-full bg-red-500 ring-2 ring-red-500/30 motion-safe:animate-pulse"
              :aria-label="t('payment.promo.redDotAria')"
            ></span>
          </button>
        </div>
        <!-- Payment in progress (shared by recharge and subscription) -->
        <template v-if="paymentPhase === 'paying'">
          <PaymentStatusPanel
            :order-id="paymentState.orderId"
            :qr-code="paymentState.qrCode"
            :expires-at="paymentState.expiresAt"
            :payment-type="paymentState.paymentType"
            :pay-url="paymentState.payUrl"
            :order-type="paymentState.orderType"
            :currency="paymentState.currency || selectedCurrency"
            @done="onPaymentDone"
            @success="onPaymentSuccess"
            @settled="onPaymentSettled"
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
            <div v-if="enabledMethods.length === 0" class="card py-16 text-center">
              <p class="text-gray-500 dark:text-gray-400">{{ t('payment.notAvailable') }}</p>
            </div>
            <template v-else>
            <!-- 充值赠送活动 banner（活动开启 + 在窗口内时由后端下发） -->
            <div
              v-if="rechargePromo"
              class="card border border-amber-200 bg-amber-50 p-4 dark:border-amber-700/40 dark:bg-amber-900/20"
            >
              <p class="text-sm font-medium text-amber-800 dark:text-amber-200">
                {{ promoValidUntilLabel
                    ? t('payment.promo.banner', { validUntil: promoValidUntilLabel })
                    : t('payment.promo.bannerNoExpiry') }}
              </p>
              <div
                v-if="promoTiers.length"
                class="mt-1 text-xs text-amber-700 dark:text-amber-300"
              >
                <span v-for="(tier, idx) in promoTiers" :key="tier.min_amount">
                  {{ t('payment.promo.tier', {
                    minAmount: tier.min_amount,
                    rate: Math.round(tier.bonus_rate * 100),
                  }) }}<span v-if="idx < promoTiers.length - 1">{{ t('payment.promo.tiersJoiner') }}</span>
                </span>

                <p>{{ t('payment.promo.customHint') }}</p>
              </div>
            </div>
            <div class="card p-6">
              <AmountInput
                v-model="amount"
                :amounts="[10, 20, 50, 100, 200, 500, 1000, 2000, 5000]"
                :min="globalMinAmount"
                :max="globalMaxAmount"
                :bonus-tiers="promoTiers"
                :show-red-dots="showPromoRedDots"
                @bonus-preset-clicked="onPromoPresetClicked"
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
            <div v-if="validAmount > 0" class="card p-6">
              <div class="space-y-2 text-sm">
                <div class="flex justify-between">
                  <span class="text-gray-500 dark:text-gray-400">{{ t('payment.paymentAmount') }}</span>
                  <span class="text-gray-900 dark:text-white">{{ formatSelectedPaymentAmount(validAmount) }}</span>
                </div>
                <div v-if="feeRate > 0" class="flex justify-between">
                  <span class="text-gray-500 dark:text-gray-400">{{ t('payment.fee') }} ({{ feeRate }}%)</span>
                  <span class="text-gray-900 dark:text-white">{{ formatSelectedPaymentAmount(feeAmount) }}</span>
                </div>
                <div v-if="feeRate > 0" class="flex justify-between border-t border-gray-200 pt-2 dark:border-dark-600">
                  <span class="font-medium text-gray-700 dark:text-gray-300">{{ t('payment.actualPay') }}</span>
                  <span class="text-lg font-bold text-primary-600 dark:text-primary-400">{{ formatSelectedPaymentAmount(totalAmount) }}</span>
                </div>
                <div v-if="balanceRechargeMultiplier !== 1" class="flex justify-between" :class="{ 'border-t border-gray-200 pt-2 dark:border-dark-600': feeRate <= 0 }">
                  <span class="text-gray-500 dark:text-gray-400">{{ t('payment.creditedBalance') }}</span>
                  <span class="text-gray-900 dark:text-white">${{ creditedAmount.toFixed(2) }}</span>
                </div>
                <!-- 赠送：仅当当前金额命中赠送档位时显示 -->
                <div
                  v-if="currentBonus > 0"
                  class="flex justify-between"
                  :class="{ 'border-t border-gray-200 pt-2 dark:border-dark-600': feeRate <= 0 && balanceRechargeMultiplier === 1 }"
                >
                  <span class="text-gray-500 dark:text-gray-400">{{ t('payment.promo.bonusLine') }}</span>
                  <span class="font-medium text-amber-600 dark:text-amber-400">+${{ currentBonus.toFixed(2) }}</span>
                </div>
                <!-- 合计入账：creditedBalance + bonus；仅有赠送时才有意义 -->
                <div
                  v-if="currentBonus > 0"
                  class="flex justify-between border-t border-gray-200 pt-2 dark:border-dark-600"
                >
                  <span class="font-medium text-gray-700 dark:text-gray-300">{{ t('payment.promo.totalCredited') }}</span>
                  <span class="text-lg font-bold text-amber-600 dark:text-amber-400">${{ totalCredited.toFixed(2) }}</span>
                </div>
                <p v-if="balanceRechargeMultiplier !== 1" class="border-t border-gray-200 pt-2 text-xs text-gray-500 dark:border-dark-600 dark:text-gray-400">
                  {{ t('payment.rechargeRatePreview', { usd: balanceRechargeMultiplier.toFixed(2) }) }}
                </p>
              </div>
            </div>
            <button :class="['btn w-full py-3 text-base font-medium', paymentButtonClass]" :disabled="!canSubmit || submitting" data-test="payment-submit-recharge" @click="handleSubmitRecharge">
              <span v-if="submitting" class="flex items-center justify-center gap-2">
                <span class="h-4 w-4 animate-spin rounded-full border-2 border-white border-t-transparent"></span>
                {{ t('common.processing') }}
              </span>
              <span v-else>{{ t('payment.createOrder') }} {{ formatSelectedPaymentAmount(totalAmount) }}</span>
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
                    {{ formatSelectedPaymentAmount(selectedPlan.original_price) }}
                  </span>
                  <span :class="['text-3xl font-bold', planTextClass]">{{ formatSelectedPaymentAmount(selectedPlan.price) }}</span>
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
              <div v-if="enabledMethods.length >= 1" class="card p-6">
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
                    <span class="text-gray-900 dark:text-white">{{ formatSelectedPaymentAmount(selectedPlan.price) }}</span>
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
                <span v-else>{{ t('payment.createOrder') }} {{ formatSelectedPaymentAmount(feeRate > 0 ? subTotalAmount : selectedPlan.price) }}</span>
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
    <!--
      Recharge-promo expired confirm modal.

      Triggered exclusively from `handleSubmitRecharge` when the
      promo's `valid_until` has ticked over since the page rendered;
      the visual + interactive grammar is deliberately matched to the
      Renewal Plan modal above (same overlay treatment, same Teleport
      / Transition wrapping, same close affordance) so users who've
      already learned that pattern in this view recognise this as a
      modal-class confirm rather than a different surface.

      Buttons live in a 2-column flex row instead of stacked: this is
      a yes/no decision, stacking would imply a hierarchy ("the top
      one is canonical") that the cancel-vs-continue choice doesn't
      have. The destructive-leaning side (cancel) is on the left,
      mirroring the OS conventions on macOS dialogs.
    -->
    <Teleport to="body">
      <Transition name="modal">
        <div
          v-if="showPromoExpiredModal"
          class="fixed inset-0 z-50 flex items-center justify-center bg-black/60 p-4 backdrop-blur-sm"
          data-test="promo-expired-modal"
          @click.self="cancelRechargeAfterPromoExpired"
        >
          <div class="relative w-full max-w-md rounded-2xl border border-gray-200 bg-white p-6 shadow-2xl dark:border-dark-700 dark:bg-dark-900">
            <button
              class="absolute right-4 top-4 rounded-lg p-1 text-gray-400 transition-colors hover:bg-gray-100 hover:text-gray-600 dark:hover:bg-dark-700 dark:hover:text-gray-200"
              @click="cancelRechargeAfterPromoExpired"
            >
              <svg class="h-5 w-5" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2"><path stroke-linecap="round" stroke-linejoin="round" d="M6 18L18 6M6 6l12 12" /></svg>
            </button>
            <h3 class="mb-2 text-lg font-semibold text-gray-900 dark:text-white">
              {{ t('payment.promo.expiredTitle') }}
            </h3>
            <p class="mb-5 text-sm leading-relaxed text-gray-600 dark:text-dark-300">
              {{ t('payment.promo.expiredBody') }}
            </p>
            <div class="flex gap-2">
              <button
                class="btn btn-secondary flex-1"
                data-test="promo-expired-cancel"
                @click="cancelRechargeAfterPromoExpired"
              >
                {{ t('payment.promo.expiredCancel') }}
              </button>
              <button
                class="btn btn-primary flex-1"
                data-test="promo-expired-confirm"
                @click="confirmRechargeAfterPromoExpired"
              >
                {{ t('payment.promo.expiredContinue') }}
              </button>
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
import { paymentAPI } from '@/api/payment'
import { extractApiErrorMessage, extractI18nErrorMessage } from '@/utils/apiError'
import { isMobileDevice } from '@/utils/device'
import type { SubscriptionPlan, CheckoutInfoResponse, CreateOrderResult, OrderType, RechargePromo } from '@/types/payment'
import AppLayout from '@/components/layout/AppLayout.vue'
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
import { platformAccentBarClass, platformBadgeLightClass, platformBadgeClass, platformTextClass, platformLabel } from '@/utils/platformColors'
import SubscriptionPlanCard from '@/components/payment/SubscriptionPlanCard.vue'
import PaymentStatusPanel from '@/components/payment/PaymentStatusPanel.vue'
import Icon from '@/components/icons/Icon.vue'
import { formatPaymentAmount, normalizePaymentCurrency } from '@/components/payment/currency'
import type { PaymentMethodOption } from '@/components/payment/PaymentMethodSelector.vue'
import { buildPaymentErrorToastMessage, describePaymentScenarioError } from './paymentUx'
import { hasWechatResumeQuery, parseWechatResumeRoute, stripWechatResumeQuery } from './paymentWechatResume'
import { useRechargePromoDot } from '@/composables/useRechargePromoDot'

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

const loading = ref(true)
const submitting = ref(false)
const errorMessage = ref('')
const errorHintMessage = ref('')
const activeTab = ref<'recharge' | 'subscription'>('recharge')
const amount = ref<number | null>(null)
const selectedMethod = ref('')
const selectedPlan = ref<SubscriptionPlan | null>(null)
const previewImage = ref('')

const paymentPhase = ref<'select' | 'paying'>('select')

interface CreateOrderOptions {
  openid?: string
  wechatResumeToken?: string
  paymentType?: string
  isResume?: boolean
  mobileQrFallbackAttempted?: boolean
  /**
   * 当前金额 banner / breakdown 上向用户展示的赠送预览金额。
   * 仅在 balance 充值且 > 0 时传；后端用其触发 RECHARGE_PROMO_EXPIRED
   * 二次校验。订阅 / 重放 (resume) / mobile-QR fallback 等路径不传，
   * 语义即"无赠送期待"，后端不会拦截。
   */
  expectedBonus?: number
  /**
   * 用户在二次确认 modal 上点过"继续充值"重发本次请求时为 true。
   */
  promoExpiredAcknowledged?: boolean
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
    payAmount: 0,
    orderType: '',
    paymentMode: '',
    resumeToken: '',
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

function buildWechatOAuthAuthorizeUrl(
  authorizeUrl: string,
  context: { paymentType: string; orderType: OrderType; planId?: number; orderAmount: number },
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

function onPaymentSuccess() {
  removeRecoverySnapshot()
  authStore.refreshUser()
  if (paymentState.value.orderType === 'subscription') {
    subscriptionStore.fetchActiveSubscriptions(true).catch(() => {})
  }
}

function onPaymentSettled() {
  removeRecoverySnapshot()
}

// All checkout data from single API call
const checkout = ref<CheckoutInfoResponse>({
  methods: {}, global_min: 0, global_max: 0,
  plans: [], balance_disabled: false, balance_recharge_multiplier: 1, recharge_fee_rate: 0, help_text: '', help_image_url: '', stripe_publishable_key: '',
})

const tabs = computed(() => {
  const result: { key: 'recharge' | 'subscription'; label: string }[] = []
  if (!checkout.value.balance_disabled) result.push({ key: 'recharge', label: t('payment.tabTopUp') })
  result.push({ key: 'subscription', label: t('payment.tabSubscribe') })
  return result
})

const visibleMethods = computed(() => getVisibleMethods(checkout.value.methods))
const enabledMethods = computed(() => Object.keys(visibleMethods.value))
const validAmount = computed(() => amount.value ?? 0)
const balanceRechargeMultiplier = computed(() => {
  const multiplier = checkout.value.balance_recharge_multiplier
  return multiplier > 0 ? multiplier : 1
})
const creditedAmount = computed(() => Math.round((validAmount.value * balanceRechargeMultiplier.value) * 100) / 100)

/**
 * 充值赠送活动；后端只在活动开启 + 当前时间在窗口内时下发，否则字段缺失。
 * 与 balance_recharge_multiplier 是两个完全独立的概念（一个加余额、一个作用于消费），
 * 这里只读后端结果、不再二次判断时间窗。
 */
const rechargePromo = computed<RechargePromo | null>(() => checkout.value?.recharge_promo ?? null)

/** AmountInput 等子组件需要的 tier 数组；活动不存在时空数组以便组件无脑渲染。 */
const promoTiers = computed(() => rechargePromo.value?.tiers ?? [])

/**
 * 计算 `payAmount` 应得的赠送金额（与后端 ResolveRechargeBonus 对齐）：
 * 升序 tiers 中最高匹配档命中（档位匹配仍以 payAmount 为准），
 * bonus = ceil(payAmount × multiplier × bonusRate × 100) / 100。
 *
 * 注意：赠送基数是 credited_balance（payAmount × multiplier），不是裸 payAmount——
 * 这与后端公式严格一致，避免前后端预览出现 0.01 级偏差。
 */
function bonusForAmount(payAmount: number): number {
  const promo = rechargePromo.value
  if (!promo || !promo.enabled || promo.tiers.length === 0) return 0
  if (!Number.isFinite(payAmount) || payAmount <= 0) return 0
  let rate = 0
  for (const tier of promo.tiers) {
    if (payAmount >= tier.min_amount) {
      rate = tier.bonus_rate
    } else {
      break
    }
  }
  if (rate <= 0) return 0
  return Math.ceil(payAmount * balanceRechargeMultiplier.value * rate * 100) / 100
}

/** 当前金额的赠送，用于 breakdown 行渲染。 */
const currentBonus = computed(() => bonusForAmount(validAmount.value))

/** 合计入账 = credited_balance + bonus（两个相加，绝不相乘）。 */
const totalCredited = computed(() => Math.round((creditedAmount.value + currentBonus.value) * 100) / 100)

const promoDot = useRechargePromoDot({
  userId: computed(() => user.value?.id ?? null),
  promo: rechargePromo,
})

/** Tab 红点：仅活动开启 + 用户未 dismiss 时显示。 */
const showRechargeTabDot = computed(() => promoDot.shouldShow.value)

/** AmountInput 的预置金额红点开关；与 tab 红点共用 dismiss 状态。 */
const showPromoRedDots = computed(() => promoDot.shouldShow.value)

/**
 * Render the recharge promo's `valid_until` ISO timestamp down to the
 * second. The bare `Date#toLocaleString()` we used previously is locale-
 * dependent — some runtimes drop seconds from the default short format,
 * which made "ends at 23:59:59" indistinguishable from "ends at 23:59"
 * for users on those locales. Hand the explicit options bag in so all
 * locales get H/M/S, matching the homepage banner and the admin
 * RechargePromos table.
 *
 * Empty / unparseable inputs fall through to '' (banner switches to the
 * "no expiry" copy) or to the original raw string (defensive, never
 * empty-renders mid-page).
 */
const promoValidUntilLabel = computed(() => {
  const raw = rechargePromo.value?.valid_until
  if (!raw) return ''
  try {
    const d = new Date(raw)
    if (isNaN(d.getTime())) return raw
    return d.toLocaleString(undefined, {
      year: 'numeric',
      month: '2-digit',
      day: '2-digit',
      hour: '2-digit',
      minute: '2-digit',
      second: '2-digit',
      hour12: false,
    })
  } catch {
    return raw
  }
})

function onRechargeTabClicked() {
  // 切到充值 Tab 即视为"看见活动"，按需求一次性 dismiss
  if (promoDot.shouldShow.value) {
    promoDot.dismiss()
  }
}

/** Tab 切换入口：除了普通赋值，还顺便处理充值红点 dismiss。 */
function onTabClicked(key: 'recharge' | 'subscription') {
  activeTab.value = key
  if (key === 'recharge') {
    onRechargeTabClicked()
  }
}

function onPromoPresetClicked(_amount: number) {
  // 点击命中赠送档位的金额即 dismiss 红点（即使不进入提交流程）
  if (promoDot.shouldShow.value) {
    promoDot.dismiss()
  }
}

/**
 * 充值赠送活动是否在"此时此刻"已经过期。
 *
 * 设计取舍：
 *   • 不做成 reactive computed + 定时器轮询。该判定只在用户点击
 *     "创建订单"那一刻才有意义，平时 banner / breakdown 维持
 *     onMounted 时的快照即可（用户已经在看着 banner 操作，让 banner
 *     在他眼皮底下文案突变反而比弹窗确认更出戏）。一次性 Date.now()
 *     比较 → 零额外生命周期、零清理负担。
 *   • 后端只在 enabled + 在窗口内时才下发 recharge_promo，所以拿到
 *     promo 一定意味着"页面打开时活动是开着的"，能形成 expired 这
 *     一过渡只能因为用户停留过久。这正是题目要拦的场景。
 *   • valid_until 缺省（无截止活动）→ 永不过期；解析失败 → 保守判
 *     定为未过期（避免后端推坏数据时无脑封锁支付通路）。
 */
function isPromoExpiredNow(): boolean {
  const promo = rechargePromo.value
  if (!promo) return false
  if (!promo.valid_until) return false
  const ts = Date.parse(promo.valid_until)
  if (!Number.isFinite(ts)) return false
  return Date.now() >= ts
}

/**
 * 充值活动到期二次确认弹窗的状态。
 *
 * `pendingExpiredAmount` 把"用户当时点击提交时的金额"快照下来，与
 * `validAmount` 解耦——避免弹窗显示后用户再去改 AmountInput 造成
 * 实际下单金额与告警金额漂移（小概率但很难调试）。
 */
const showPromoExpiredModal = ref(false)
const pendingExpiredAmount = ref(0)

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
const localeCode = computed(() => {
  const raw = i18n.locale as unknown
  if (typeof raw === 'string') return raw
  if (raw && typeof raw === 'object' && 'value' in raw) {
    return String((raw as { value?: string }).value || '')
  }
  return undefined
})

function formatSelectedPaymentAmount(value: number): string {
  return formatPaymentAmount(value, selectedCurrency.value, localeCode.value)
}

const methodOptions = computed<PaymentMethodOption[]>(() =>
  enabledMethods.value.map((type) => {
    const ml = visibleMethods.value[type]
    return {
      type,
      fee_rate: ml?.fee_rate ?? 0,
      available: ml?.available !== false && amountFitsMethod(validAmount.value, type),
    }
  })
)

const feeRate = computed(() => checkout.value?.recharge_fee_rate ?? 0)
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

// Subscription-specific: method options based on plan price
const subMethodOptions = computed<PaymentMethodOption[]>(() => {
  const planPrice = selectedPlan.value?.price ?? 0
  return enabledMethods.value.map((type) => {
    const ml = visibleMethods.value[type]
    return {
      type,
      fee_rate: ml?.fee_rate ?? 0,
      available: ml?.available !== false && amountFitsMethod(planPrice, type),
    }
  })
})

const subFeeAmount = computed(() => {
  const price = selectedPlan.value?.price ?? 0
  if (feeRate.value <= 0 || price <= 0) return 0
  return Math.ceil(((price * feeRate.value) / 100) * 100) / 100
})

const subTotalAmount = computed(() => {
  const price = selectedPlan.value?.price ?? 0
  if (feeRate.value <= 0 || price <= 0) return price
  return Math.round((price + subFeeAmount.value) * 100) / 100
})

const canSubmitSubscription = computed(() =>
  selectedPlan.value !== null
    && amountFitsMethod(selectedPlan.value.price, selectedMethod.value)
    && selectedLimit.value?.available !== false
)

// Auto-switch to first available method when current selection can't handle the amount
watch(() => [validAmount.value, selectedMethod.value] as const, ([amt, method]) => {
  if (amt <= 0 || amountFitsMethod(amt, method)) return
  const available = enabledMethods.value.find((m) => amountFitsMethod(amt, m))
  if (available) selectedMethod.value = available
})

// Payment button class: follows selected payment method color
const paymentButtonClass = computed(() => {
  const m = selectedMethod.value
  if (!m) return 'btn-primary'
  if (m.includes('alipay')) return 'btn-alipay'
  if (m.includes('wxpay')) return 'btn-wxpay'
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
  const u = selectedPlan.value.validity_unit || 'day'
  if (u === 'month') return t('payment.perMonth')
  if (u === 'year') return t('payment.perYear')
  return `${selectedPlan.value.validity_days}${t('payment.days')}`
})

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
  // 充值赠送活动到期防呆：用户停留时间过长时，页面上的 banner /
  // breakdown 加赠行依然来自 onMounted 时拿到的 checkout-info 快照
  // （rechargePromo 不会自己刷），但 valid_until 已经悄悄过期了。
  // 后端在 createOrder 时会按服务器当前时间重新判窗，赠送会被静默
  // 拒绝 —— 那一刻用户已经付完钱、看着账单和 UI 上 +$X.XX 对不上号
  // 才反应过来，体验非常糟糕。
  //
  // 这里在客户端判窗 + 二次确认：只要 valid_until 已过就先弹一个
  // 模态告诉用户"加赠没了，是否仍继续"，确认后才走老路。判断只发
  // 生在点击的瞬间（一次性 Date.now() 比较），不需要轮询定时器；
  // 那种情况发生频率远低于支付主流程，加 setInterval 反而引入清理
  // 复杂度。
  if (isPromoExpiredNow()) {
    pendingExpiredAmount.value = validAmount.value
    showPromoExpiredModal.value = true
    return
  }
  // 把当前金额的赠送预览金额（与后端 ResolveRechargeBonus 同算法
  // mirror）随请求带给 server。这是触发 server 端 RECHARGE_PROMO_EXPIRED
  // 二次校验的唯一信号 — 只有 client 明确表明"我正在向用户展示 $X
  // 赠送"时，server 才会在赠送实际不发的时候返回 409；其它路径
  // (订阅 / recovery 重放) 不传该字段，server 一律放行。
  await createOrder(validAmount.value, 'balance', undefined, {
    expectedBonus: bonusForAmount(validAmount.value),
  })
}

/**
 * 用户在到期确认弹窗里点"继续充值"——把闸口外面那笔金额放进真正
 * 的下单流程。为什么单独锁一个 `pendingExpiredAmount`、而不是直接
 * 复用 `validAmount.value`？因为模态是异步交互、用户可以再去改输
 * 入框，避免"显示弹窗时是 200，确认前用户改成 500"造成的金额漂移。
 */
async function confirmRechargeAfterPromoExpired() {
  const amt = pendingExpiredAmount.value
  showPromoExpiredModal.value = false
  pendingExpiredAmount.value = 0
  if (amt > 0) {
    // 用户已二次确认；带 ack=true 重发，server 跳过 promo 拦截。
    // expectedBonus 仍传当前的本地预览：让 server 能精确比对（虽然
    // 在 ack=true 路径下 server 不再读这个字段做拦截判定，仍透传
    // 是为了语义自包含 — 这次提交在客户端的"期待"是什么，链路上始终
    // 一致）。fulfillment 阶段按服务器时间重判窗，不会因此误发赠送。
    await createOrder(amt, 'balance', undefined, {
      expectedBonus: bonusForAmount(amt),
      promoExpiredAcknowledged: true,
    })
  }
}

/** 用户取消——不创建订单，让其回到金额选择界面继续犹豫或退出。 */
function cancelRechargeAfterPromoExpired() {
  showPromoExpiredModal.value = false
  pendingExpiredAmount.value = 0
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
      paymentType: requestType,
      orderType,
      planId,
      origin: typeof window !== 'undefined' ? window.location.origin : '',
      isMobile: isMobileDevice(),
      isWechatBrowser: typeof window !== 'undefined' && /MicroMessenger/i.test(window.navigator.userAgent),
      forceQRCode: !!(checkout.value.alipay_force_qrcode && normalizeVisibleMethod(requestType) === 'alipay'),
      expectedBonus: options.expectedBonus,
      promoExpiredAcknowledged: options.promoExpiredAcknowledged,
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
      })
      return
    }

    if (decision.kind === 'unhandled') {
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
    // 充值赠送活动到期 — 服务端兜底拦截。
    //
    // 客户端时钟可能慢于真实时间，本地 isPromoExpiredNow() 漏判时
    // server 会返回 409 RECHARGE_PROMO_EXPIRED；我们必须把这条错误
    // 转化为同一个二次确认 modal（与本地快速路径形成对称），而不是
    // 走通用 toast。条件刻意收紧到"balance 充值 + 还没 ack"——订阅
    // 永远不应进这个分支（前端不会传 expectedBonus），但加这层
    // 防护让错误处理读起来更显式；ack 已传却又拿到 409 属于服务端
    // 状态自相矛盾，按通用错误处理走，方便调试时不被静默吞掉。
    if (
      apiErr.reason === 'RECHARGE_PROMO_EXPIRED' &&
      orderType === 'balance' &&
      !options.promoExpiredAcknowledged
    ) {
      pendingExpiredAmount.value = orderAmount
      showPromoExpiredModal.value = true
      return
    }
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
    const payload = buildCreateOrderPayload({
      amount: context.orderAmount,
      paymentType: visibleMethod,
      orderType: context.orderType,
      planId: context.planId,
      origin: typeof window !== 'undefined' ? window.location.origin : '',
      isMobile: false,
      isWechatBrowser: false,
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

  await router.replace({ path: route.path, query: stripWechatResumeQuery(route.query) })

  if (resume.wechatResumeToken) {
    await createOrder(0, resume.orderType, resume.planId, {
      wechatResumeToken: resume.wechatResumeToken,
      paymentType: resume.paymentType,
      isResume: true,
    })
    return
  }

  if (resume.orderAmount > 0 && resume.openid) {
    await createOrder(resume.orderAmount, resume.orderType, resume.planId, {
      openid: resume.openid,
      paymentType: resume.paymentType,
      isResume: true,
    })
  }
}

onMounted(async () => {
  try {
    const res = await paymentAPI.getCheckoutInfo()
    checkout.value = res.data
    // 把活动配置同步到 payment store，让侧边栏 /purchase 红点直接复用，
    // 避免两侧各自再发一次 checkout-info。
    paymentStore.setRechargePromo(res.data?.recharge_promo ?? null)
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
      if (hasWechatResumeQuery(route.query)) {
        removeRecoverySnapshot()
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
    // Plaza → PaymentView funnel: when the visitor lands here via
    // `/purchase?plan_id=<id>` (typically after clicking "Buy now" on the
    // plaza, possibly via a login round-trip), preselect that plan and clear
    // `plan_id` from the URL so reload starts clean.
    //
    // Wechat-resume / state-driven (`tab=subscription&group=…`) flows take
    // precedence: only act when neither already chose a plan and no Wechat
    // resume token / restored payment state is in play.
    if (typeof route.query.plan_id !== 'undefined') {
      const wechatResuming = hasWechatResumeQuery(route.query) || paymentPhase.value === 'paying'
      if (!selectedPlan.value && !wechatResuming) {
        const rawId = route.query.plan_id
        if (typeof rawId === 'string') {
          const parsed = Number(rawId)
          if (Number.isInteger(parsed) && parsed > 0) {
            const match = checkout.value.plans.find((p) => p.id === parsed)
            if (match) {
              activeTab.value = 'subscription'
              selectedPlan.value = match
            }
          }
        }
      }
      // Always strip the param so reloads stay idempotent — even when the id
      // didn't match, leaving `?plan_id=9999` around adds nothing useful.
      const { plan_id: _omit, ...cleanQuery } = route.query
      void _omit
      router.replace({ path: route.path, query: cleanQuery }).catch(() => {})
    }
  } catch (err: unknown) { appStore.showError(extractI18nErrorMessage(err, t, 'payment.errors', t('common.error'))) }
  finally { loading.value = false }
  // Fetch active subscriptions (uses cache, non-blocking)
  subscriptionStore.fetchActiveSubscriptions().catch(() => {})
})
</script>
