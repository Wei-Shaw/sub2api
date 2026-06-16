<template>
  <AppLayout>
    <div class="mx-auto max-w-4xl space-y-6">
      <div v-if="loading" class="flex items-center justify-center py-20">
        <div class="h-8 w-8 animate-spin rounded-full border-4 border-primary-500 border-t-transparent"></div>
      </div>
      <template v-else>
        <!-- Tab Switcher (hide during payment and subscription confirm) -->
        <div v-if="co.tabs.value.length > 1 && order.paymentPhase.value === 'select' && !co.selectedPlan.value" class="flex space-x-1 rounded-xl bg-gray-100 p-1 dark:bg-dark-800">
          <button v-for="tab in co.tabs.value" :key="tab.key"
            class="flex-1 rounded-lg px-4 py-2.5 text-sm font-medium transition-all"
            :class="activeTab === tab.key ? 'bg-white text-gray-900 shadow dark:bg-dark-700 dark:text-white' : 'text-gray-500 hover:text-gray-700 dark:text-gray-400 dark:hover:text-gray-300'"
            @click="activeTab = tab.key">{{ tab.label }}</button>
        </div>
        <!-- Payment in progress (shared by recharge and subscription) -->
        <template v-if="order.paymentPhase.value === 'paying'">
          <PaymentStatusPanel
            :order-id="order.paymentState.value.orderId"
            :qr-code="order.paymentState.value.qrCode"
            :expires-at="order.paymentState.value.expiresAt"
            :payment-type="order.paymentState.value.paymentType"
            :pay-url="order.paymentState.value.payUrl"
            :order-type="order.paymentState.value.orderType"
            :currency="order.paymentState.value.currency || co.selectedCurrency.value"
            @done="onPaymentDone"
            @success="onPaymentSuccess"
            @settled="onPaymentSettled"
          />
        </template>
        <!-- Tab content (select phase) -->
        <template v-else>
          <template v-if="activeTab === 'recharge'">
            <RechargeTab
              :username="user?.username || ''"
              :balance="user?.balance"
              :amount="co.amount.value"
              :enabled-methods="co.enabledMethods.value"
              :method-options="co.methodOptions.value"
              :selected-method="co.selectedMethod.value"
              :min-amount="co.globalMinAmountNum.value"
              :max-amount="co.globalMaxAmountNum.value"
              :amount-error="co.amountError.value"
              :valid-amount="co.validAmount.value"
              :has-fee-rate="co.hasFeeRate.value"
              :fee-rate="co.feeRate.value"
              :fee-amount="co.feeAmount.value"
              :total-amount="co.totalAmount.value"
              :has-multiplier="co.hasMultiplier.value"
              :credited-amount="co.creditedAmount.value"
              :balance-recharge-multiplier="co.balanceRechargeMultiplier.value"
              :can-submit="co.canSubmit.value"
              :submitting="order.submitting.value"
              :button-class="co.paymentButtonClass.value"
              :format-amount="co.formatSelectedPaymentAmount"
              @update:amount="co.amount.value = $event"
              @update:selected-method="co.selectedMethod.value = $event"
              @submit="handleSubmitRecharge"
            />
          </template>
          <template v-else-if="activeTab === 'subscription'">
            <template v-if="co.selectedPlan.value">
              <SubscriptionConfirm
                :plan="co.selectedPlan.value"
                :enabled-methods="co.enabledMethods.value"
                :sub-method-options="co.subMethodOptions.value"
                :selected-method="co.selectedMethod.value"
                :has-fee-rate="co.hasFeeRate.value"
                :fee-rate="co.feeRate.value"
                :has-positive-plan-price="co.hasPositivePlanPrice.value"
                :sub-fee-amount="co.subFeeAmount.value"
                :sub-total-amount="co.subTotalAmount.value"
                :can-submit="co.canSubmitSubscription.value"
                :submitting="order.submitting.value"
                :button-class="co.paymentButtonClass.value"
                :format-amount="co.formatSelectedPaymentAmount"
                @update:selected-method="co.selectedMethod.value = $event"
                @confirm="confirmSubscribe"
                @cancel="co.selectedPlan.value = null"
              />
            </template>
            <template v-else>
              <SubscriptionList
                :plans="co.checkout.value.plans"
                :active-subscriptions="activeSubscriptions"
                :grid-class="co.planGridClass.value"
                @select="selectPlan"
              />
            </template>
          </template>
        </template>
        <div v-if="(co.checkout.value.help_text || co.checkout.value.help_image_url) && order.paymentPhase.value === 'select' && !co.selectedPlan.value" class="card p-4">
          <div class="flex flex-col items-center gap-3">
            <img v-if="co.checkout.value.help_image_url" :src="co.checkout.value.help_image_url" alt=""
              class="h-40 max-w-full cursor-pointer rounded-lg object-contain transition-opacity hover:opacity-80"
              @click="previewImage = co.checkout.value.help_image_url" />
            <p v-if="co.checkout.value.help_text" class="text-center text-sm text-gray-500 dark:text-gray-400">{{ co.checkout.value.help_text }}</p>
          </div>
        </div>
      </template>
    </div>
    <!-- Renewal Plan Selection Modal -->
    <RenewalModal
      :show="showRenewalModal"
      :plans="renewalPlans"
      :active-subscriptions="activeSubscriptions"
      @close="closeRenewalModal"
      @select="selectPlanFromModal"
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
import { ref, computed, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute, useRouter } from 'vue-router'
import { useAuthStore, useSubscriptionStore, useAppStore } from '../../stores/host'
import { paymentAPI } from '../../api/payment'
import { extractI18nErrorMessage } from '@sub2api/plugin-sdk'
import { money } from '../../utils/decimal'
import { normalizeVisibleMethod, PAYMENT_RECOVERY_STORAGE_KEY } from '../../components/payment/paymentFlow'
import { hasWechatResumeQuery, parseWechatResumeRoute, stripWechatResumeQuery } from './paymentWechatResume'
import { usePaymentCheckout } from '../../composables/usePaymentCheckout'
import { usePaymentOrder, type CreateOrderOptions } from '../../composables/usePaymentOrder'
import type { SubscriptionPlan } from '../../types/payment'
import AppLayout from '../../components/common/AppLayout.vue'
import PaymentStatusPanel from '../../components/payment/PaymentStatusPanel.vue'
import RechargeTab from '../../components/payment/RechargeTab.vue'
import SubscriptionConfirm from '../../components/payment/SubscriptionConfirm.vue'
import SubscriptionList from '../../components/payment/SubscriptionList.vue'
import RenewalModal from '../../components/payment/RenewalModal.vue'

const { t } = useI18n()
const route = useRoute()
const router = useRouter()
const authStore = useAuthStore()
const subscriptionStore = useSubscriptionStore()
const appStore = useAppStore()

const user = computed(() => authStore.user)
const activeSubscriptions = computed(() => subscriptionStore.activeSubscriptions)

const loading = ref(true)
const activeTab = ref<'recharge' | 'subscription'>('recharge')
const previewImage = ref('')

// -- Composables --
const co = usePaymentCheckout()
const order = usePaymentOrder({
  selectedMethod: () => co.selectedMethod.value,
  selectedPlan: () => co.selectedPlan.value,
  selectedCurrency: () => co.selectedCurrency.value,
})

// -- Renewal modal --
const showRenewalModal = ref(false)
const renewGroupId = ref<number | null>(null)
const renewalPlans = computed(() => {
  if (renewGroupId.value == null) return []
  return co.checkout.value.plans.filter(p => p.group_id === renewGroupId.value)
})

function selectPlan(plan: SubscriptionPlan) {
  co.selectedPlan.value = plan
}
function selectPlanFromModal(plan: SubscriptionPlan) {
  showRenewalModal.value = false
  renewGroupId.value = null
  co.selectedPlan.value = plan
}
function closeRenewalModal() {
  showRenewalModal.value = false
  renewGroupId.value = null
}

// -- Handlers --
async function handleSubmitRecharge() {
  if (!co.canSubmit.value || order.submitting.value) return
  await order.createOrder(co.validAmount.value.toString(), 'balance')
}
async function confirmSubscribe() {
  if (!co.selectedPlan.value || order.submitting.value) return
  await order.createOrder(co.selectedPlan.value.price, 'subscription', co.selectedPlan.value.id)
}

function onPaymentDone() {
  const wasSubscription = order.paymentState.value.orderType === 'subscription'
  order.resetPayment()
  co.selectedPlan.value = null
  if (wasSubscription) subscriptionStore.fetchActiveSubscriptions(true).catch(() => {})
}
function onPaymentSuccess() {
  order.removeRecoverySnapshot()
  authStore.refreshUser()
  if (order.paymentState.value.orderType === 'subscription') {
    subscriptionStore.fetchActiveSubscriptions(true).catch(() => {})
  }
}
function onPaymentSettled() {
  order.removeRecoverySnapshot()
}

// -- Wechat resume --
async function resumeWechatPaymentFromQuery() {
  const resume = parseWechatResumeRoute(route.query, co.checkout.value.plans, co.validAmount.value.toString())
  if (!resume) return
  co.selectedMethod.value = resume.paymentType
  const resumeAmountDec = money(resume.orderAmount)
  if (resume.orderType === 'balance' && resumeAmountDec.gt(0)) {
    co.amount.value = resumeAmountDec.toNumber()
  }
  if (resume.orderType === 'subscription' && resume.planId) {
    co.selectedPlan.value = co.checkout.value.plans.find(plan => plan.id === resume.planId) ?? null
  }
  await router.replace({ path: route.path, query: stripWechatResumeQuery(route.query) })
  if (resume.wechatResumeToken) {
    await order.createOrder('0', resume.orderType, resume.planId, { wechatResumeToken: resume.wechatResumeToken, paymentType: resume.paymentType, isResume: true } as CreateOrderOptions)
    return
  }
  if (resumeAmountDec.gt(0) && resume.openid) {
    await order.createOrder(resume.orderAmount, resume.orderType, resume.planId, { openid: resume.openid, paymentType: resume.paymentType, isResume: true } as CreateOrderOptions)
  }
}

// -- Init --
onMounted(async () => {
  try {
    const res = await paymentAPI.getCheckoutInfo()
    co.checkout.value = res.data
    co.initDefaultMethod()
    if (typeof window !== 'undefined') {
      if (hasWechatResumeQuery(route.query)) order.removeRecoverySnapshot()
      const routeResumeToken = typeof route.query.resume_token === 'string'
        ? route.query.resume_token
        : typeof route.query.wechat_resume_token === 'string'
          ? route.query.wechat_resume_token : undefined
      if (order.tryRestorePayment(routeResumeToken)) {
        const restored = order.getRestoredMethod()
        if (restored) co.selectedMethod.value = restored
      }
    }
    await resumeWechatPaymentFromQuery()
    if (co.checkout.value.balance_disabled) activeTab.value = 'subscription'
    if (route.query.tab === 'subscription') {
      activeTab.value = 'subscription'
      if (route.query.group) {
        const groupId = Number(route.query.group)
        const groupPlans = co.checkout.value.plans.filter(p => p.group_id === groupId)
        if (groupPlans.length === 1) co.selectedPlan.value = groupPlans[0]
        else if (groupPlans.length > 1) { renewGroupId.value = groupId; showRenewalModal.value = true }
      }
    }
  } catch (err: unknown) { appStore.showError(extractI18nErrorMessage(err, t, 'payment.errors', t('common.error'))) }
  finally { loading.value = false }
  subscriptionStore.fetchActiveSubscriptions().catch(() => {})
})
</script>