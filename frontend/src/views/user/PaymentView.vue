<template>
  <AppLayout>
    <div class="mx-auto max-w-3xl space-y-5">
      <div v-if="loading" class="rounded border border-line bg-surface p-5" aria-live="polite">
        <p class="flex items-center gap-2 text-2xs font-medium uppercase tracking-[0.08em] text-ink-tertiary">
          <span class="spinner h-3 w-3 shrink-0" aria-hidden="true" />
          {{ t('common.processing') }}
        </p>
      </div>

      <template v-else>
        <!--
          Tabs: an underline on a rule, not two pills floating on a gray track.
          `.tabs` / `.tab` / `.tab-active` in style.css own the geometry, and the
          active tab is the only thing on the strip carrying the accent — which
          is the accent's job here, since choosing a tab IS a selection.
        -->
        <div
          v-if="tabs.length > 1 && paymentPhase === 'select' && !selectedPlan"
          class="tabs"
          role="tablist"
        >
          <button
            v-for="tab in tabs"
            :key="tab.key"
            type="button"
            role="tab"
            :aria-selected="activeTab === tab.key"
            class="tab"
            :class="activeTab === tab.key && 'tab-active'"
            @click="activeTab = tab.key"
          >{{ tab.label }}</button>
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
            :transfer="paymentState.transfer"
            @done="onPaymentDone"
            @success="onPaymentSuccess"
            @settled="onPaymentSettled"
          />
        </template>

        <!-- Tab content (select phase) -->
        <template v-else>
          <!-- ═══ Top-up ═══ -->
          <template v-if="activeTab === 'recharge'">
            <!--
              The account header. The balance used to be `text-green-600` — a
              semantic status colour spent on a number that has no state, which
              also meant a zero balance read as reassuring green. It is a
              quantity, so it gets the quantity treatment: mono, tabular, right
              of its label, and no colour at all.
            -->
            <section class="rounded border border-line bg-surface px-4 py-3">
              <p class="text-2xs font-medium uppercase tracking-[0.04em] text-ink-tertiary">
                {{ t('payment.rechargeAccount') }}
              </p>
              <div class="mt-1 flex items-baseline justify-between gap-4">
                <p class="min-w-0 truncate text-sm font-medium text-ink">{{ user?.username || '' }}</p>
                <p class="flex shrink-0 items-baseline gap-1 text-xs text-ink-tertiary">
                  {{ t('payment.currentBalance') }}
                  <span class="text-2xs">{{ creditedCurrencySymbol }}</span>
                  <!--
                    `user?.balance` is `undefined` until the profile resolves.
                    `NumCell` renders an en dash for that and `0.00` only for a
                    real zero — on a balance, "not loaded yet" and "you have no
                    money" must not look identical.
                  -->
                  <NumCell :value="user?.balance ?? null" :precision="2" />
                </p>
              </div>
            </section>

            <div
              v-if="enabledMethods.length === 0"
              class="rounded border border-line bg-surface px-4 py-8 text-center text-sm text-ink-tertiary"
            >
              {{ t('payment.notAvailable') }}
            </div>

            <template v-else>
              <section class="rounded border border-line bg-surface p-4">
                <AmountInput
                  v-model="amount"
                  :amounts="[10, 20, 50, 100, 200, 500, 1000, 2000, 5000]"
                  :min="globalMinAmount"
                  :max="globalMaxAmount"
                  :error="amountError"
                />
              </section>

              <section v-if="enabledMethods.length >= 1" class="rounded border border-line bg-surface p-4">
                <PaymentMethodSelector
                  :methods="methodOptions"
                  :selected="selectedMethod"
                  @select="selectedMethod = $event"
                />
              </section>

              <!--
                The order summary. Hairline rows in a single panel — the old one
                was four `flex justify-between` lines with the total in
                `text-primary-600`, i.e. the accent used to mean "this number is
                big", which is exactly what the accent must not mean. Weight and
                size carry the emphasis instead.
              -->
              <dl
                v-if="validAmount > 0"
                class="divide-y divide-line-subtle rounded border border-line bg-surface px-4 text-xs"
              >
                <div class="flex items-baseline justify-between gap-4 py-2">
                  <dt class="shrink-0 text-ink-tertiary">{{ t('payment.paymentAmount') }}</dt>
                  <dd class="inline-flex items-baseline justify-end gap-0.5">
                    <span class="text-2xs text-ink-tertiary">{{ gatewayCurrencySymbol }}</span>
                    <NumCell :value="gatewayAmount" :precision="gatewayPrecision" />
                  </dd>
                </div>
                <div v-if="feeRate > 0" class="flex items-baseline justify-between gap-4 py-2">
                  <dt class="shrink-0 text-ink-tertiary">
                    {{ t('payment.fee') }}
                    <span class="font-mono tabular-nums">({{ feeRate }}%)</span>
                  </dt>
                  <dd class="inline-flex items-baseline justify-end gap-0.5">
                    <span class="text-2xs text-ink-tertiary">{{ gatewayCurrencySymbol }}</span>
                    <NumCell :value="feeAmount" :precision="gatewayPrecision" />
                  </dd>
                </div>
                <div v-if="feeRate > 0" class="flex items-baseline justify-between gap-4 py-2">
                  <dt class="shrink-0 font-medium text-ink">{{ t('payment.actualPay') }}</dt>
                  <dd class="inline-flex items-baseline justify-end gap-0.5 text-sm">
                    <span class="text-2xs text-ink-tertiary">{{ gatewayCurrencySymbol }}</span>
                    <NumCell :value="totalAmount" :precision="gatewayPrecision" />
                  </dd>
                </div>
                <!--
                  Credited balance is USD credit; everything above it is in the
                  gateway's settlement currency. The two symbols are the only
                  thing standing between the user and believing they topped up
                  100 dollars with a ¥100 payment — which is why the row also
                  shows on a 1× multiplier whenever the gateway settles in
                  something other than dollars: every other figure in the
                  summary is then a six-digit ₫ amount, and this is the only
                  place the credit itself appears.
                -->
                <div
                  v-if="showCreditedAmount"
                  class="flex items-baseline justify-between gap-4 py-2"
                >
                  <dt class="shrink-0 text-ink-tertiary">{{ t('payment.creditedBalance') }}</dt>
                  <dd class="inline-flex items-baseline justify-end gap-0.5">
                    <span class="text-2xs text-ink-tertiary">{{ creditedCurrencySymbol }}</span>
                    <NumCell :value="creditedAmount" :precision="2" />
                  </dd>
                </div>
                <div v-if="balanceRechargeMultiplier !== 1" class="py-2 text-2xs text-ink-tertiary">
                  {{ t('payment.rechargeRatePreview', { usd: balanceRechargeMultiplier.toFixed(2) }) }}
                </div>
              </dl>

              <!--
                `loading` keeps the label's box and overlays a spinner, rather
                than swapping the text for "processing…". A submit button that
                changes width at the moment of the click is bad anywhere; on the
                button that charges a card it is the worst version of the bug.
              -->
              <Button
                tone="accent"
                variant="solid"
                size="md"
                block
                data-testid="submit-recharge"
                :loading="submitting"
                :disabled="!canSubmit"
                @click="handleSubmitRecharge"
              >
                {{ t('payment.createOrder') }}
                <span class="ml-1 inline-flex items-baseline gap-0.5">
                  <span class="text-2xs opacity-80">{{ gatewayCurrencySymbol }}</span>
                  <NumCell :value="totalAmount" :precision="gatewayPrecision" />
                </span>
              </Button>
            </template>
          </template>

          <!-- ═══ Subscribe ═══ -->
          <template v-else-if="activeTab === 'subscription'">
            <!-- Subscription confirm (inline, replaces plan list) -->
            <template v-if="selectedPlan">
              <section class="rounded border border-line bg-surface">
                <div class="flex items-start justify-between gap-4 p-4">
                  <div class="min-w-0">
                    <!--
                      The platform is a CATEGORY, so it is a neutral badge. It
                      used to carry a per-platform hue, which gave this panel a
                      second accent and made the plan name compete with a label
                      that only says which upstream the group belongs to.
                    -->
                    <Badge caps>{{ platformLabel(selectedPlan.group_platform || '') }}</Badge>
                    <h1 class="mt-1.5 break-words text-lg font-semibold text-ink">
                      {{ selectedPlan.name }}
                    </h1>
                    <p v-if="selectedPlan.description" class="mt-1 text-xs text-ink-tertiary">
                      {{ selectedPlan.description }}
                    </p>
                  </div>

                  <div class="shrink-0 text-right">
                    <div class="flex items-baseline justify-end gap-0.5">
                      <span class="text-2xs text-ink-tertiary">{{ gatewayCurrencySymbol }}</span>
                      <span class="font-mono text-xl font-semibold tabular-nums slashed-zero text-ink">
                        <NumCell :value="subPaymentAmount" :precision="gatewayPrecision" />
                      </span>
                    </div>
                    <p class="mt-0.5 text-2xs text-ink-tertiary">/ {{ planValiditySuffix }}</p>
                    <div
                      v-if="subOriginalPriceAmount !== null"
                      class="mt-1 flex items-baseline justify-end gap-0.5 line-through"
                    >
                      <span class="text-2xs text-ink-disabled">{{ gatewayCurrencySymbol }}</span>
                      <span class="font-mono text-2xs tabular-nums slashed-zero text-ink-disabled">
                        <NumCell :value="subOriginalPriceAmount" :precision="gatewayPrecision" />
                      </span>
                    </div>
                  </div>
                </div>

                <!-- Spec rows. Hairlines, not a tinted well inside a panel. -->
                <dl class="divide-y divide-line-subtle border-t border-line-subtle text-xs">
                  <div class="flex items-baseline justify-between gap-4 px-4 py-1.5">
                    <dt class="shrink-0 text-ink-tertiary">{{ t('payment.planCard.rate') }}</dt>
                    <dd class="font-mono tabular-nums text-ink">×{{ selectedPlan.rate_multiplier ?? 1 }}</dd>
                  </div>
                  <div
                    v-if="planHasPeakRate(selectedPlan)"
                    class="flex items-baseline justify-between gap-4 px-4 py-1.5"
                  >
                    <dt class="shrink-0 text-ink-tertiary">{{ t('payment.planCard.peakRate') }}</dt>
                    <dd class="text-right font-mono tabular-nums text-ink-secondary">
                      {{ planPeakRateLabel(selectedPlan) }}
                    </dd>
                  </div>
                  <div
                    v-if="selectedPlan.daily_limit_usd != null"
                    class="flex items-baseline justify-between gap-4 px-4 py-1.5"
                  >
                    <dt class="shrink-0 text-ink-tertiary">{{ t('payment.planCard.dailyLimit') }}</dt>
                    <dd><NumCell :value="selectedPlan.daily_limit_usd" :precision="2" unit="USD" /></dd>
                  </div>
                  <div
                    v-if="selectedPlan.weekly_limit_usd != null"
                    class="flex items-baseline justify-between gap-4 px-4 py-1.5"
                  >
                    <dt class="shrink-0 text-ink-tertiary">{{ t('payment.planCard.weeklyLimit') }}</dt>
                    <dd><NumCell :value="selectedPlan.weekly_limit_usd" :precision="2" unit="USD" /></dd>
                  </div>
                  <div
                    v-if="selectedPlan.monthly_limit_usd != null"
                    class="flex items-baseline justify-between gap-4 px-4 py-1.5"
                  >
                    <dt class="shrink-0 text-ink-tertiary">{{ t('payment.planCard.monthlyLimit') }}</dt>
                    <dd><NumCell :value="selectedPlan.monthly_limit_usd" :precision="2" unit="USD" /></dd>
                  </div>
                  <div v-if="selectedPlanHasNoLimit" class="flex items-baseline justify-between gap-4 px-4 py-1.5">
                    <dt class="shrink-0 text-ink-tertiary">{{ t('payment.planCard.quota') }}</dt>
                    <dd class="text-ink">{{ t('payment.planCard.unlimited') }}</dd>
                  </div>
                </dl>
              </section>

              <section v-if="enabledMethods.length >= 1" class="rounded border border-line bg-surface p-4">
                <PaymentMethodSelector
                  :methods="subMethodOptions"
                  :selected="selectedMethod"
                  @select="selectedMethod = $event"
                />
              </section>

              <dl
                v-if="feeRate > 0 && selectedPlan.price > 0"
                class="divide-y divide-line-subtle rounded border border-line bg-surface px-4 text-xs"
              >
                <div class="flex items-baseline justify-between gap-4 py-2">
                  <dt class="shrink-0 text-ink-tertiary">{{ t('payment.amountLabel') }}</dt>
                  <dd class="inline-flex items-baseline justify-end gap-0.5">
                    <span class="text-2xs text-ink-tertiary">{{ gatewayCurrencySymbol }}</span>
                    <NumCell :value="subPaymentAmount" :precision="gatewayPrecision" />
                  </dd>
                </div>
                <div class="flex items-baseline justify-between gap-4 py-2">
                  <dt class="shrink-0 text-ink-tertiary">
                    {{ t('payment.fee') }}
                    <span class="font-mono tabular-nums">({{ feeRate }}%)</span>
                  </dt>
                  <dd class="inline-flex items-baseline justify-end gap-0.5">
                    <span class="text-2xs text-ink-tertiary">{{ gatewayCurrencySymbol }}</span>
                    <NumCell :value="subFeeAmount" :precision="gatewayPrecision" />
                  </dd>
                </div>
                <div class="flex items-baseline justify-between gap-4 py-2">
                  <dt class="shrink-0 font-medium text-ink">{{ t('payment.actualPay') }}</dt>
                  <dd class="inline-flex items-baseline justify-end gap-0.5 text-sm">
                    <span class="text-2xs text-ink-tertiary">{{ gatewayCurrencySymbol }}</span>
                    <NumCell :value="subTotalAmount" :precision="gatewayPrecision" />
                  </dd>
                </div>
              </dl>

              <div class="space-y-2">
                <Button
                  tone="accent"
                  variant="solid"
                  size="md"
                  block
                  data-testid="submit-subscription"
                  :loading="submitting"
                  :disabled="!canSubmitSubscription"
                  @click="confirmSubscribe"
                >
                  {{ t('payment.createOrder') }}
                  <span class="ml-1 inline-flex items-baseline gap-0.5">
                    <span class="text-2xs opacity-80">{{ gatewayCurrencySymbol }}</span>
                    <NumCell :value="subTotalAmount" :precision="gatewayPrecision" />
                  </span>
                </Button>
                <Button variant="outline" size="md" block @click="selectedPlan = null">
                  {{ t('common.cancel') }}
                </Button>
              </div>
            </template>

            <!-- Plan list -->
            <template v-else>
              <!--
                No 48px gift glyph over the empty state. An illustration that
                appears only when there is nothing to show is decoration where
                the user most needs a sentence.
              -->
              <div
                v-if="checkout.plans.length === 0"
                class="rounded border border-line bg-surface px-4 py-8 text-center text-sm text-ink-tertiary"
              >
                {{ t('payment.noPlans') }}
              </div>
              <div v-else :class="planGridClass">
                <SubscriptionPlanCard
                  v-for="plan in checkout.plans"
                  :key="plan.id"
                  :plan="plan"
                  :active-subscriptions="activeSubscriptions"
                  @select="selectPlan"
                />
              </div>

              <!-- Active subscriptions (compact, below plan list) -->
              <section v-if="activeSubscriptions.length > 0">
                <h2 class="mb-2 text-2xs font-medium uppercase tracking-[0.04em] text-ink-tertiary">
                  {{ t('payment.activeSubscription') }}
                </h2>
                <!--
                  Each row used to open with a 1px×24px accent bar in the
                  platform's colour, plus a `rounded-full` micro-badge in the
                  same hue and a `badge-success` pill. Three colour channels for
                  two facts. Now: the platform as a neutral badge, the state as a
                  dot WITH its word, and the row itself untinted.
                -->
                <ul class="divide-y divide-line-subtle rounded border border-line bg-surface">
                  <li
                    v-for="sub in activeSubscriptions"
                    :key="sub.id"
                    class="flex items-center gap-3 px-4 py-2"
                  >
                    <div class="min-w-0 flex-1">
                      <div class="flex items-center gap-1.5">
                        <span class="truncate text-xs font-medium text-ink">
                          {{ sub.group?.name || t('payment.groupFallback', { id: sub.group_id }) }}
                        </span>
                        <Badge caps class="shrink-0">{{ platformLabel(sub.group?.platform || '') }}</Badge>
                      </div>
                      <div class="mt-0.5 flex flex-wrap gap-x-3 text-2xs text-ink-tertiary">
                        <span>
                          {{ t('payment.planCard.rate') }}:
                          <span class="font-mono tabular-nums">×{{ sub.group?.rate_multiplier ?? 1 }}</span>
                        </span>
                        <span v-if="subscriptionHasPeakRate(sub)">
                          {{ t('payment.planCard.peakRate') }}: {{ subscriptionPeakRateLabel(sub) }}
                        </span>
                        <span v-if="subscriptionHasNoLimit(sub)">
                          {{ t('payment.planCard.quota') }}: {{ t('payment.planCard.unlimited') }}
                        </span>
                        <span v-if="sub.expires_at">
                          {{ t('userSubscriptions.daysRemaining', { days: getDaysRemaining(sub.expires_at) }) }}
                        </span>
                        <span v-else>{{ t('userSubscriptions.noExpiration') }}</span>
                      </div>
                    </div>
                    <StatusDot
                      class="shrink-0"
                      tone="success"
                      :label="t('userSubscriptions.status.active')"
                    />
                  </li>
                </ul>
              </section>
            </template>
          </template>
        </template>

        <section
          v-if="(checkout.help_text || checkout.help_image_url) && paymentPhase === 'select' && !selectedPlan"
          class="rounded border border-line bg-surface p-4"
        >
          <div class="flex flex-col items-center gap-3">
            <button
              v-if="checkout.help_image_url"
              type="button"
              class="cursor-zoom-in rounded-sm transition-opacity duration-fast ease-out hover:opacity-80"
              :aria-label="t('payment.title')"
              @click="previewImage = checkout.help_image_url"
            >
              <img :src="checkout.help_image_url" alt="" class="h-40 max-w-full object-contain" />
            </button>
            <p v-if="checkout.help_text" class="text-center text-xs text-ink-secondary">
              {{ checkout.help_text }}
            </p>
          </div>
        </section>
      </template>
    </div>

    <!-- Renewal Plan Selection Modal -->
    <Teleport to="body">
      <Transition name="modal">
        <div
          v-if="showRenewalModal"
          class="fixed inset-0 z-50 flex items-start justify-center overflow-y-auto bg-overlay/60 p-4 py-12"
          @click.self="closeRenewalModal"
        >
          <div
            class="relative w-full max-w-lg rounded border border-line bg-surface-raised p-5 shadow-modal"
            role="dialog"
            aria-modal="true"
            :aria-label="t('payment.selectPlan')"
          >
            <button
              type="button"
              class="absolute right-3 top-3 rounded-sm p-1 text-ink-tertiary transition-colors duration-fast ease-out hover:bg-surface-hover hover:text-ink"
              :aria-label="t('common.close')"
              :title="t('common.close')"
              @click="closeRenewalModal"
            >
              <Icon name="x" size="sm" />
            </button>
            <h2 class="mb-4 text-sm font-semibold text-ink">{{ t('payment.selectPlan') }}</h2>
            <div class="space-y-3">
              <SubscriptionPlanCard
                v-for="plan in renewalPlans"
                :key="plan.id"
                :plan="plan"
                :active-subscriptions="activeSubscriptions"
                @select="selectPlanFromModal"
              />
            </div>
          </div>
        </div>
      </Transition>
    </Teleport>

    <!-- Image Preview Overlay -->
    <Teleport to="body">
      <Transition name="modal">
        <div
          v-if="previewImage"
          class="fixed inset-0 z-[60] flex items-center justify-center bg-overlay/80 p-4"
          @click="previewImage = ''"
        >
          <img :src="previewImage" alt="" class="max-h-[85vh] max-w-[90vw] rounded-sm object-contain" />
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
import { hasPeakRate, formatPeakRateWindow, serverTimezoneLabel, type PeakRateFields } from '@/utils/peak-rate'
import type { SubscriptionPlan, CheckoutInfoResponse, CreateOrderResult, OrderType } from '@/types/payment'
import type { UserSubscription } from '@/types'
import AppLayout from '@/components/layout/AppLayout.vue'
// Direct paths, never the `components/common` barrel: it drags `createI18n`
// into the module graph and breaks the partial `vue-i18n` factory mock this
// view's spec relies on.
import Badge from '@/components/common/Badge.vue'
import Button from '@/components/common/Button.vue'
import NumCell from '@/components/common/NumCell.vue'
import StatusDot from '@/components/common/StatusDot.vue'
import AmountInput from '@/components/payment/AmountInput.vue'
import PaymentMethodSelector from '@/components/payment/PaymentMethodSelector.vue'
/*
 * `isBuiltInAlipayMethod` / `isBuiltInWxpayMethod` are gone from this view along
 * with `paymentButtonClass`. The submit button no longer takes the selected
 * provider's colour: on this page the provider is already named and marked in
 * the method selector directly above, and repainting the ONE primary action per
 * provider meant the page's most important control changed meaning-by-colour
 * four different ways. `.btn-stripe` and friends survive where they belong — on
 * a button that talks to exactly one provider (see StripePaymentView).
 */
import { METHOD_ORDER } from '@/components/payment/providerConfig'
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
import {
  DEFAULT_PAYMENT_CURRENCY,
  SEPAY_CURRENCY,
  currencySymbol,
  formatPaymentAmount,
  normalizePaymentCurrency,
  paymentCurrencyFractionDigits,
} from '@/components/payment/currency'
import { planValiditySuffix as validitySuffixOf } from '@/components/payment/validity'
import type { PaymentMethodOption } from '@/components/payment/PaymentMethodSelector.vue'
import { buildPaymentErrorToastMessage, describePaymentScenarioError } from './paymentUx'

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

/** Extracted only so the template stops carrying a three-clause `v-if`. */
function subscriptionHasNoLimit(sub: UserSubscription): boolean {
  return sub.group?.daily_limit_usd == null
    && sub.group?.weekly_limit_usd == null
    && sub.group?.monthly_limit_usd == null
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
  paymentType?: string
  isResume?: boolean
  mobileQrFallbackAttempted?: boolean
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
    intentId: '',
    currency: '',
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
  plans: [], balance_disabled: false, balance_recharge_multiplier: 1, subscription_usd_to_vnd_rate: 0, recharge_fee_rate: 0, help_text: '', help_image_url: '',
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
  return Number.isFinite(multiplier) && multiplier > 0 ? multiplier : 1
})
// USD→VND rate for dong channels. 0 = unset, in which case the amount is
// charged as-is — the same opt-in condition the backend applies.
const usdToVndRate = computed(() => {
  const rate = checkout.value.subscription_usd_to_vnd_rate
  return Number.isFinite(rate) && rate > 0 ? rate : 0
})
const creditedAmount = computed(() => Math.round((validAmount.value * balanceRechargeMultiplier.value) * 100) / 100)

// Adaptive grid: center single card, 2-col for 2 plans, 3-col for 3+
const planGridClass = computed(() => {
  const n = checkout.value.plans.length
  if (n <= 2) return 'grid grid-cols-1 gap-4 sm:grid-cols-2'
  return 'grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3'
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

// Visible methods decide the amount range shown to users. The amount box is
// USD, so each method's bound is converted out of its settlement currency
// first — a raw dong figure would otherwise land in the box as dollars.
const globalMinAmount = computed(() => {
  const limits = Object.values(visibleMethods.value)
  if (limits.length === 0) return 0
  if (limits.some(limit => limit.single_min <= 0)) return 0
  return Math.min(...limits.map(limit => ceilPaymentAmount(
    usdAmountFromGatewayAmount(limit.single_min, normalizePaymentCurrency(limit.currency)),
    DEFAULT_PAYMENT_CURRENCY,
  )))
})
const globalMaxAmount = computed(() => {
  const limits = Object.values(visibleMethods.value)
  if (limits.length === 0) return 0
  if (limits.some(limit => limit.single_max <= 0)) return 0
  return Math.max(...limits.map(limit => floorPaymentAmount(
    usdAmountFromGatewayAmount(limit.single_max, normalizePaymentCurrency(limit.currency)),
    DEFAULT_PAYMENT_CURRENCY,
  )))
})

// Selected method's limits (for validation and error messages)
const selectedLimit = computed(() => visibleMethods.value[selectedMethod.value])
const selectedCurrency = computed(() => normalizePaymentCurrency(selectedLimit.value?.currency))

/**
 * The symbol and the minor-unit precision for whatever the selected gateway
 * settles in. Both are needed because every displayed amount is now a symbol
 * `<span>` plus a `NumCell`, and `NumCell` has no idea what money is — passing
 * a flat `precision="2"` would invent two decimal places for JPY and KRW.
 */
const gatewayCurrencySymbol = computed(() => currencySymbol(selectedCurrency.value))
const gatewayPrecision = computed(() => paymentCurrencyFractionDigits(selectedCurrency.value))

/** Balance credit is always USD, whatever the gateway charged in. */
const creditedCurrencySymbol = currencySymbol(DEFAULT_PAYMENT_CURRENCY)

/**
 * A 1× multiplier used to hide the credited row, which was fine while the
 * summary was denominated in dollars either way. Once a dong channel converts,
 * the row is the only line still stating what the payment buys.
 */
const showCreditedAmount = computed(() =>
  balanceRechargeMultiplier.value !== 1 || selectedCurrency.value !== DEFAULT_PAYMENT_CURRENCY
)

const localeCode = computed(() => {
  const raw = i18n.locale as unknown
  if (typeof raw === 'string') return raw
  if (raw && typeof raw === 'object' && 'value' in raw) {
    return String((raw as { value?: string }).value || '')
  }
  return undefined
})

function roundPaymentAmount(value: number, currency: string): number {
  if (!Number.isFinite(value)) return 0
  const factor = 10 ** paymentCurrencyFractionDigits(currency)
  return Math.round(value * factor) / factor
}

function ceilPaymentAmount(value: number, currency: string): number {
  if (!Number.isFinite(value)) return 0
  const factor = 10 ** paymentCurrencyFractionDigits(currency)
  return Math.ceil(value * factor) / factor
}

function floorPaymentAmount(value: number, currency: string): number {
  if (!Number.isFinite(value)) return 0
  const factor = 10 ** paymentCurrencyFractionDigits(currency)
  return Math.floor(value * factor) / factor
}

/**
 * A USD figure — plan price or recharge amount alike — as the gateway will
 * collect it. Mirrors the backend's `calculateGatewayBaseAmount`.
 */
function gatewayPaymentAmountForCurrency(value: number, currency: string): number {
  const rate = usdToVndRate.value
  if (rate <= 0 || currency !== SEPAY_CURRENCY) return roundPaymentAmount(value, currency)
  return roundPaymentAmount(value * rate, currency)
}

/**
 * The inverse. Per-method `single_min` / `single_max` are stated in the
 * gateway's settlement currency, but the amount box takes USD, so the bounds
 * handed to it have to come back the other way — otherwise a dong channel with
 * a ₫20,000 floor would tell the user to top up at least 20,000 dollars.
 *
 * Left unrounded on purpose: a bound has to be rounded *inward* (min up, max
 * down) or the hinted figure converts back to just outside the bound it came
 * from, and the amount box then rejects the very number it suggested.
 */
function usdAmountFromGatewayAmount(value: number, currency: string): number {
  const rate = usdToVndRate.value
  if (rate <= 0 || currency !== SEPAY_CURRENCY) return value
  return value / rate
}

/**
 * Still needed for the `amountTooLow` / `amountTooHigh` strings: those go
 * through `t()` interpolation, so the bound has to arrive as finished text
 * rather than as a `NumCell`.
 */
function formatSelectedPaymentAmount(value: number): string {
  return formatPaymentAmount(value, selectedCurrency.value, localeCode.value)
}

const methodOptions = computed<PaymentMethodOption[]>(() =>
  enabledMethods.value.map((type) => {
    const ml = visibleMethods.value[type]
    return {
      type,
      fee_rate: ml?.fee_rate ?? 0,
      available: ml?.available !== false && amountFitsMethod(rechargeTotalForMethod(type), type),
    }
  })
)

const feeRate = computed(() => checkout.value?.recharge_fee_rate ?? 0)

/**
 * The recharge amount as the selected gateway will collect it. The box takes
 * USD; a dong channel charges that times the configured rate, so every figure
 * in the summary below has to be the converted one — printing the USD number
 * under a `₫` symbol is how a $10 top-up came to ask for ten dong.
 */
const gatewayAmount = computed(() => gatewayPaymentAmountForCurrency(validAmount.value, selectedCurrency.value))
const feeAmount = computed(() =>
  feeRate.value > 0 && gatewayAmount.value > 0
    ? ceilPaymentAmount((gatewayAmount.value * feeRate.value) / 100, selectedCurrency.value)
    : 0
)
const totalAmount = computed(() =>
  feeRate.value > 0 && gatewayAmount.value > 0
    ? roundPaymentAmount(gatewayAmount.value + feeAmount.value, selectedCurrency.value)
    : gatewayAmount.value
)

/** The recharge total in the settlement currency of one specific method. */
function rechargeTotalForMethod(methodType: string): number {
  const ml = visibleMethods.value[methodType]
  return gatewayTotalAmountForCurrency(validAmount.value, normalizePaymentCurrency(ml?.currency))
}

const amountError = computed(() => {
  if (validAmount.value <= 0) return ''
  // No method can handle this amount
  if (!enabledMethods.value.some((m) => amountFitsMethod(rechargeTotalForMethod(m), m))) {
    return t('payment.amountNoMethod')
  }
  // Selected method can't handle this amount (but others can). Both sides of
  // the comparison are in the gateway's currency, fee included — the same
  // figure the backend checks the limits against.
  const ml = selectedLimit.value
  if (ml) {
    if (ml.single_min > 0 && totalAmount.value < ml.single_min) return t('payment.amountTooLow', { min: formatSelectedPaymentAmount(ml.single_min) })
    if (ml.single_max > 0 && totalAmount.value > ml.single_max) return t('payment.amountTooHigh', { max: formatSelectedPaymentAmount(ml.single_max) })
  }
  return ''
})

const canSubmit = computed(() =>
  validAmount.value > 0
    && amountFitsMethod(totalAmount.value, selectedMethod.value)
    && selectedLimit.value?.available !== false
)

const subPaymentAmount = computed(() => {
  const price = selectedPlan.value?.price ?? 0
  return gatewayPaymentAmountForCurrency(price, selectedCurrency.value)
})

/** `null`, not `0`, when the plan carries no list price — there is no strike-through to draw. */
const subOriginalPriceAmount = computed<number | null>(() => {
  const original = selectedPlan.value?.original_price ?? 0
  if (!original || original <= 0) return null
  return gatewayPaymentAmountForCurrency(original, selectedCurrency.value)
})

const selectedPlanHasNoLimit = computed(() =>
  selectedPlan.value?.daily_limit_usd == null
  && selectedPlan.value?.weekly_limit_usd == null
  && selectedPlan.value?.monthly_limit_usd == null
)

const subFeeAmount = computed(() => {
  if (feeRate.value <= 0 || subPaymentAmount.value <= 0) return 0
  return ceilPaymentAmount((subPaymentAmount.value * feeRate.value) / 100, selectedCurrency.value)
})

const subTotalAmount = computed(() => {
  if (feeRate.value <= 0 || subPaymentAmount.value <= 0) return subPaymentAmount.value
  return roundPaymentAmount(subPaymentAmount.value + subFeeAmount.value, selectedCurrency.value)
})

function gatewayTotalAmountForCurrency(value: number, currency: string): number {
  const paymentAmount = gatewayPaymentAmountForCurrency(value, currency)
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
      fee_rate: ml?.fee_rate ?? 0,
      available: ml?.available !== false && amountFitsMethod(gatewayTotalAmountForCurrency(price, currency), type),
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
  await createOrder(validAmount.value, 'balance')
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
    })

    const result = await paymentStore.createOrder(payload) as CreateOrderResult & { resume_token?: string }
    const openWindow = (url: string) => {
      const win = window.open(url, '_blank', 'noopener,noreferrer')
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
    paymentPhase.value = 'paying'
    persistRecoverySnapshot(decision.recovery)

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
    return false
    /* eslint-disable no-unreachable */
    || reason === '__never__'
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
    })
    const result = await paymentStore.createOrder(payload) as CreateOrderResult & { resume_token?: string }
    const decision = decidePaymentLaunch(result, {
      visibleMethod,
      orderType: context.orderType,
      isMobile: false,
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
    if (typeof window !== 'undefined') {
      const routeResumeToken = typeof route.query.resume_token === 'string'
        ? route.query.resume_token
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
