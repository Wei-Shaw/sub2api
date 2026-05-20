<template>
  <AppLayout>
    <div class="mx-auto max-w-5xl space-y-6">
      <div v-if="loading" class="flex items-center justify-center py-20">
        <div class="h-8 w-8 animate-spin rounded-full border-4 border-primary-500 border-t-transparent"></div>
      </div>
      <template v-else>
        <template v-if="paymentPhase === 'paying'">
          <PaymentStatusPanel
            :order-id="paymentState.orderId" :qr-code="paymentState.qrCode"
            :expires-at="paymentState.expiresAt" :payment-type="paymentState.paymentType"
            :pay-url="paymentState.payUrl" :order-type="paymentState.orderType"
            @done="onPaymentDone" @success="onPaymentSuccess" @settled="onPaymentSettled"
          />
        </template>
        <template v-else>
          <template v-if="selectedPlan">
            <div class="mb-2"><button class="text-sm text-gray-500 hover:text-gray-700 dark:text-gray-400" @click="selectedPlan = null">&larr; 返回</button></div>
            <div class="grid gap-6 lg:grid-cols-5">
              <div class="lg:col-span-3 rounded-2xl border border-gray-200 bg-white p-6 dark:border-gray-700 dark:bg-gray-800">
                <div class="mb-4 flex items-center gap-3"><span class="rounded-lg bg-gray-900 px-2.5 py-1 text-xs font-medium text-white dark:bg-white dark:text-gray-900">{{ selectedPlan.name }}</span><span class="text-sm text-gray-400">{{ platformLabel(selectedPlan.group_platform || '') }}</span></div>
                <div class="flex items-baseline gap-2"><span v-if="selectedPlan.original_price" class="text-sm text-gray-400 line-through">&yen;{{ selectedPlan.original_price }}</span><span class="text-4xl font-bold text-gray-900 dark:text-white">&yen;{{ planDisplayAmount(selectedPlan) }}</span><span class="text-sm text-gray-400">{{ planPriceSuffix(selectedPlan) }}</span></div>
                <p v-if="selectedPlan.description" class="mt-3 text-sm leading-6 text-gray-500 dark:text-gray-400">{{ selectedPlan.description }}</p>
                <ul v-if="planFeatures(selectedPlan).length" class="mt-5 space-y-3 text-sm text-gray-600 dark:text-gray-300">
                  <li v-for="feature in planFeatures(selectedPlan)" :key="feature" class="flex gap-2">
                    <span class="mt-1.5 h-1.5 w-1.5 shrink-0 rounded-full bg-gray-900 dark:bg-white"></span>
                    <span>{{ feature }}</span>
                  </li>
                </ul>
              </div>
              <div class="lg:col-span-2 rounded-2xl border border-gray-200 bg-white p-6 dark:border-gray-700 dark:bg-gray-800">
                <h3 class="text-sm font-semibold text-gray-900 dark:text-white">购买方式</h3>
                <p class="mt-2 text-sm text-gray-500 dark:text-gray-400">直接使用账户余额购买套餐</p>
                <div class="mt-4 rounded-xl bg-gray-50 px-4 py-3 dark:bg-gray-700/50">
                  <p class="text-xs text-gray-400">当前可用余额</p>
                  <p class="mt-1 text-lg font-semibold text-gray-900 dark:text-white">${{ user?.balance?.toFixed(2) || '0.00' }}</p>
                </div>
                <button class="mt-4 w-full rounded-xl bg-gray-900 py-3 text-sm font-medium text-white hover:bg-gray-800 disabled:opacity-50 dark:bg-white dark:text-gray-900" :disabled="submitting || selectedPlan.purchase_quote?.blocked" @click="purchaseSelectedPlanWithBalance">
                  <span v-if="submitting">购买中...</span><span v-else>{{ planButtonText(selectedPlan) }}</span>
                </button>
                <p class="mt-3 text-xs leading-5 text-gray-400">余额不足时，请联系 QQ 591719412 充值后再购买。</p>
                <router-link to="/redeem" class="mt-2 block w-full rounded-xl border border-gray-200 py-2.5 text-center text-sm text-gray-600 hover:bg-gray-50 dark:border-gray-600 dark:text-gray-300">已有兑换码？立即兑换</router-link>
              </div>
            </div>
          </template>
          <template v-else>
            <div class="grid gap-4 sm:grid-cols-2">
              <div class="rounded-2xl border border-gray-200 bg-white p-5 dark:border-gray-700 dark:bg-gray-800">
                <p class="text-sm text-gray-400">当前余额</p>
                <p class="mt-1 text-3xl font-bold text-gray-900 dark:text-white">${{ user?.balance?.toFixed(2) || '0.00' }}</p>
              </div>
              <div class="rounded-2xl border border-gray-200 bg-white p-5 dark:border-gray-700 dark:bg-gray-800">
                <p class="text-sm text-gray-400">客服充值</p>
                <div class="mt-2 flex items-center gap-2"><svg class="h-5 w-5 text-[#12B7F5]" viewBox="0 0 24 24" fill="currentColor"><path d="M12.003 2c-2.265 0-6.29 1.364-6.29 7.325v1.195S3.55 14.96 3.55 17.474c0 .665.17 1.025.396 1.025.19 0 .46-.18.758-.625.775-1.15 1.525-2.76 1.997-3.775.14.02.282.03.425.03 1.47 0 2.97-.765 3.852-2.115.88 1.35 2.382 2.115 3.852 2.115.143 0 .284-.01.425-.03.472 1.015 1.222 2.625 1.997 3.775.298.445.568.625.758.625.226 0 .396-.36.396-1.025 0-2.514-2.163-6.954-2.163-6.954v-1.195C18.293 3.364 14.268 2 12.003 2z"/></svg><span class="text-lg font-semibold text-gray-900 dark:text-white">QQ: 591719412</span></div>
                <p class="mt-2 text-xs text-gray-400">加客服获取付款码 &rarr; 转账 &rarr; 获取兑换码</p>
              </div>
            </div>
            <div class="rounded-2xl border border-gray-200 bg-white p-5 dark:border-gray-700 dark:bg-gray-800">
              <p class="mb-3 text-sm text-gray-500 dark:text-gray-400">选择充值金额</p>
              <div class="mt-3 flex items-center gap-3">
                <button v-for="amt in [10, 20, 50, 100, 200, 500]" :key="amt" class="rounded-xl border px-3 py-2.5 text-center text-sm font-medium transition-all active:scale-[0.97]" :class="qqSelectedAmount === amt ? 'border-gray-900 bg-gray-900 text-white dark:border-white dark:bg-white dark:text-gray-900' : 'border-gray-200 text-gray-600 hover:border-gray-400 dark:border-gray-600 dark:text-gray-300 dark:hover:border-gray-400'" @click="customAmount = ''; qqSelectedAmount = amt">&yen;{{ amt }}</button>
                <div class="relative flex-1">
                  <span class="pointer-events-none absolute inset-y-0 left-3 flex items-center text-sm text-gray-400">&yen;</span>
                  <input v-model="customAmount" type="number" min="1" step="0.01" placeholder="任意金额" class="w-full rounded-xl border border-gray-200 bg-white py-2.5 pl-7 pr-14 text-sm text-gray-900 placeholder-gray-400 outline-none transition-colors focus:border-gray-400 dark:border-gray-600 dark:bg-gray-700 dark:text-white dark:placeholder-gray-500 dark:focus:border-gray-400" @input="qqSelectedAmount = 0" />
                  <span class="pointer-events-none absolute inset-y-0 right-3 flex items-center text-sm text-gray-400">人民币</span>
                </div>
              </div>
              <p v-if="estimatedBalance > 0" class="mt-3 text-sm text-gray-500 dark:text-gray-400">预计到账：<span class="font-semibold text-gray-900 dark:text-white">${{ estimatedBalance.toFixed(2) }}</span></p>
              <p class="mt-2 text-xs text-gray-400">示例：&yen;50 &rarr; $50 API 余额。这里的 $ 是平台内 API 余额计价单位，用于抵扣模型调用费用，不代表提现或法币兑换；支持最多两位小数。充值页已取消"充值满 &yen;50 额外赠送 $50 API 余额"活动，当前以页面实际到账与站内公告为准。</p>
            </div>
            <!-- Redeem Code -->
            <div class="grid gap-4 lg:grid-cols-2">
              <div class="rounded-2xl border border-gray-200 bg-white p-5 dark:border-gray-700 dark:bg-gray-800">
                <p class="mb-1 text-sm font-medium text-gray-700 dark:text-gray-200">兑换码充值</p>
                <p class="mb-3 text-xs text-gray-400">购买后收到的卡密在这里兑换到账户余额。</p>
                <div class="flex gap-2">
                  <input v-model="redeemCode" type="text" placeholder="输入兑换码" class="flex-1 rounded-xl border border-gray-200 bg-white px-4 py-2.5 text-sm text-gray-900 placeholder-gray-400 outline-none transition-colors focus:border-gray-400 dark:border-gray-600 dark:bg-gray-700 dark:text-white dark:placeholder-gray-500 dark:focus:border-gray-400" @keyup.enter="handleRedeem" />
                  <button class="rounded-xl bg-gray-900 px-5 py-2.5 text-sm font-medium text-white transition-colors hover:bg-gray-800 disabled:opacity-50 dark:bg-white dark:text-gray-900 dark:hover:bg-gray-100" :disabled="!redeemCode || redeeming" @click="handleRedeem">
                    <span v-if="redeeming">兑换中...</span><span v-else>兑换</span>
                  </button>
                </div>
                <p v-if="redeemError" class="mt-2 text-xs text-red-500">{{ redeemError }}</p>
                <p v-if="redeemSuccess" class="mt-2 text-xs text-green-500">{{ redeemSuccess }}</p>
              </div>
              <div class="rounded-2xl border border-gray-200 bg-white p-5 dark:border-gray-700 dark:bg-gray-800">
                <p class="mb-1 text-sm font-medium text-gray-700 dark:text-gray-200">优惠码领取</p>
                <p class="mb-3 text-xs text-gray-400">注册后有优惠码，也可以在这里领取一次性赠送余额。</p>
                <div class="flex gap-2">
                  <input v-model="promoCode" type="text" placeholder="输入优惠码" class="flex-1 rounded-xl border border-gray-200 bg-white px-4 py-2.5 text-sm text-gray-900 placeholder-gray-400 outline-none transition-colors focus:border-gray-400 dark:border-gray-600 dark:bg-gray-700 dark:text-white dark:placeholder-gray-500 dark:focus:border-gray-400" @keyup.enter="handleRedeemPromo" />
                  <button class="rounded-xl border border-gray-900 px-5 py-2.5 text-sm font-medium text-gray-900 transition-colors hover:bg-gray-50 disabled:opacity-50 dark:border-white dark:text-white dark:hover:bg-gray-700" :disabled="!promoCode || promoRedeeming" @click="handleRedeemPromo">
                    <span v-if="promoRedeeming">领取中...</span><span v-else>领取</span>
                  </button>
                </div>
                <p v-if="promoError" class="mt-2 text-xs text-red-500">{{ promoError }}</p>
                <p v-if="promoSuccess" class="mt-2 text-xs text-green-500">{{ promoSuccess }}</p>
              </div>
            </div>
            <div>
              <h2 class="mb-4 text-lg font-semibold text-gray-900 dark:text-white">订阅套餐</h2>
              <div v-if="checkout.plans.length === 0" class="rounded-2xl border border-gray-200 bg-white py-16 text-center dark:border-gray-700 dark:bg-gray-800"><p class="text-gray-400">暂无套餐</p></div>
              <div v-else class="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
                <div v-for="plan in sortedPlans" :key="plan.id" class="group relative rounded-2xl border border-gray-200 bg-white p-5 transition-all hover:border-gray-300 dark:border-gray-700 dark:bg-gray-800 dark:hover:border-gray-500" :class="{ 'ring-2 ring-gray-900 dark:ring-white': plan._recommended }">
                  <div v-if="plan._recommended" class="absolute -top-3 left-4 rounded-full bg-gray-900 px-3 py-1 text-xs font-medium text-white dark:bg-white dark:text-gray-900">推荐</div>
                  <div class="mb-4">
                    <div class="flex items-center justify-between"><span class="text-base font-bold text-gray-900 dark:text-white">{{ plan.name }}</span><span class="rounded-md bg-gray-100 px-2 py-0.5 text-xs text-gray-500 dark:bg-gray-700 dark:text-gray-400">{{ platformLabel(plan.group_platform || '') }}</span></div>
                    <div class="mt-2 flex items-baseline gap-2"><span v-if="plan.original_price" class="text-xs text-gray-400 line-through">&yen;{{ plan.original_price }}</span><span class="text-2xl font-bold text-gray-900 dark:text-white">&yen;{{ planDisplayAmount(plan) }}</span><span class="text-xs text-gray-400">{{ planPriceSuffix(plan) }}</span></div>
                    <p v-if="plan.purchase_quote?.action === 'upgrade'" class="mt-1 text-xs text-blue-600 dark:text-blue-400">升档补差价，剩余时间按秒折算</p>
                    <p v-else-if="plan.purchase_quote?.action === 'extend'" class="mt-1 text-xs text-green-600 dark:text-green-400">当前套餐，购买后自动延期</p>
                    <p v-else-if="plan.purchase_quote?.blocked" class="mt-1 text-xs text-amber-600 dark:text-amber-400">{{ plan.purchase_quote.reason || '当前只支持升档' }}</p>
                    <p v-else-if="plan.original_price" class="mt-1 text-xs text-gray-400">立省 &yen;{{ (plan.original_price - plan.price).toFixed(0) }}</p>
                  </div>
                  <div class="mb-4 space-y-2 text-sm text-gray-500 dark:text-gray-400">
                    <p v-if="plan.description" class="leading-5">{{ plan.description }}</p>
                    <ul v-if="planFeatures(plan).length" class="space-y-1.5 text-gray-600 dark:text-gray-300">
                      <li v-for="feature in planFeatures(plan).slice(0, 4)" :key="feature" class="flex gap-2">
                        <span class="mt-1.5 h-1.5 w-1.5 shrink-0 rounded-full bg-gray-300 dark:bg-gray-500"></span>
                        <span>{{ feature }}</span>
                      </li>
                    </ul>
                  </div>
                  <button class="w-full rounded-xl py-2.5 text-sm font-medium transition-colors disabled:opacity-50" :class="plan._recommended ? 'bg-gray-900 text-white hover:bg-gray-800 dark:bg-white dark:text-gray-900 dark:hover:bg-gray-100' : 'border border-gray-200 text-gray-700 hover:bg-gray-50 dark:border-gray-600 dark:text-gray-300 dark:hover:bg-gray-700'" :disabled="submitting || plan.purchase_quote?.blocked" @click="purchasePlanWithBalance(plan)">
                    <span v-if="submitting && selectedPlan?.id === plan.id">购买中...</span><span v-else>{{ planButtonText(plan) }}</span>
                  </button>
                </div>
              </div>
              <div v-if="activeSubscriptions.length > 0" class="mt-6">
                <h3 class="mb-3 text-sm font-medium text-gray-500 dark:text-gray-400">当前订阅</h3>
                <div class="space-y-2">
                  <div v-for="sub in activeSubscriptions" :key="sub.id" class="flex items-center gap-3 rounded-xl border border-gray-200 bg-white px-4 py-3 dark:border-gray-700 dark:bg-gray-800">
                    <div class="min-w-0 flex-1">
                      <div class="flex items-center gap-2"><span class="text-sm font-semibold text-gray-900 dark:text-white">{{ sub.group?.name || 'Group ' + sub.group_id }}</span></div>
                      <p class="mt-0.5 text-xs text-gray-400"><span v-if="sub.expires_at">剩余 {{ getDaysRemaining(sub.expires_at) }} 天</span><span v-else>永久有效</span></p>
                    </div>
                    <span class="rounded-full bg-green-50 px-2 py-0.5 text-[10px] font-medium text-green-600 dark:bg-green-900/30 dark:text-green-400">生效中</span>
                  </div>
                </div>
              </div>
            </div>
          </template>
        </template>
      </template>
    </div>

    <Teleport to="body">
      <Transition name="modal">
        <div v-if="previewImage" class="fixed inset-0 z-[60] flex items-center justify-center bg-black/70 backdrop-blur-sm" @click="previewImage = ''">
          <img :src="previewImage" alt="" class="max-h-[85vh] max-w-[90vw] rounded-xl object-contain shadow-2xl" />
        </div>
      </Transition>
    </Teleport>

    <ConfirmDialog
      :show="showBalancePurchaseConfirm"
      title="确认购买套餐"
      :message="balancePurchaseConfirmMessage"
      :confirm-text="pendingBalancePlan ? planButtonText(pendingBalancePlan) : '确认购买'"
      cancel-text="再想想"
      @confirm="confirmBalancePurchase"
      @cancel="cancelBalancePurchase"
    />
  </AppLayout>
</template>


<script setup lang="ts">
// @ts-nocheck
import { ref, computed, onMounted, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute, useRouter } from 'vue-router'
import { useAuthStore } from '@/stores/auth'
import { usePaymentStore } from '@/stores/payment'
import { useSubscriptionStore } from '@/stores/subscriptions'
import { useAppStore } from '@/stores'
import { paymentAPI } from '@/api/payment'
import { redeemAPI } from '@/api'
import { extractApiErrorMessage, extractI18nErrorMessage } from '@/utils/apiError'
import { isMobileDevice } from '@/utils/device'
import type { SubscriptionPlan, CheckoutInfoResponse, CreateOrderResult, OrderType } from '@/types/payment'
import AppLayout from '@/components/layout/AppLayout.vue'
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
import PaymentStatusPanel from '@/components/payment/PaymentStatusPanel.vue'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
import { buildPaymentErrorToastMessage, describePaymentScenarioError } from './paymentUx'
import { hasWechatResumeQuery, parseWechatResumeRoute, stripWechatResumeQuery } from './paymentWechatResume'

const { t } = useI18n()
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
const qqSelectedAmount = ref<number | null>(null)
const customAmount = ref('')
const estimatedBalance = computed(() => {
  const amt = customAmount.value ? parseFloat(customAmount.value) : (qqSelectedAmount.value || 0)
  return amt > 0 ? amt : 0
})
const redeemCode = ref('')
const redeeming = ref(false)
const redeemError = ref('')
const redeemSuccess = ref('')
const promoCode = ref('')
const promoRedeeming = ref(false)
const promoError = ref('')
const promoSuccess = ref('')
const showBalancePurchaseConfirm = ref(false)
const pendingBalancePlan = ref<SubscriptionPlan | null>(null)
const balancePurchaseConfirmMessage = computed(() => {
  const plan = pendingBalancePlan.value
  if (!plan) return ''
  const action = plan.purchase_quote?.action
  const amount = planDisplayAmount(plan)
  const balance = user.value?.balance?.toFixed?.(2) || '0.00'
  if (action === 'upgrade') {
    return `确认将当前套餐升档为「${plan.name}」吗？本次将从账户余额扣除 $${amount}，升档后当前周期已使用量会保留。当前余额 $${balance}。`
  }
  if (action === 'extend') {
    return `确认续费「${plan.name}」吗？本次将从账户余额扣除 $${amount}，购买成功后自动延长 ${plan.validity_days} 天。当前余额 $${balance}。`
  }
  return `确认购买「${plan.name}」吗？本次将从账户余额扣除 $${amount}，购买成功后立即生效。当前余额 $${balance}。`
})

async function handleRedeem() {
  if (!redeemCode.value || redeeming.value) return
  redeeming.value = true
  redeemError.value = ''
  redeemSuccess.value = ''
  try {
    await redeemAPI.redeem(redeemCode.value.trim())
    redeemSuccess.value = '兑换成功！余额已更新'
    redeemCode.value = ''
    await authStore.refreshUser()
    setTimeout(() => { redeemSuccess.value = '' }, 3000)
  } catch (e: any) {
    redeemError.value = extractApiErrorMessage(e) || '兑换失败'
  } finally {
    redeeming.value = false
  }
}

async function handleRedeemPromo() {
  if (!promoCode.value || promoRedeeming.value) return
  promoRedeeming.value = true
  promoError.value = ''
  promoSuccess.value = ''
  try {
    const result = await redeemAPI.redeemPromo(promoCode.value.trim())
    promoSuccess.value = result?.bonus_amount
      ? `领取成功！已到账 $${result.bonus_amount.toFixed(2)}`
      : (result?.message || '领取成功！余额已更新')
    promoCode.value = ''
    await authStore.refreshUser()
    setTimeout(() => { promoSuccess.value = '' }, 3000)
  } catch (e: any) {
    promoError.value = extractApiErrorMessage(e) || '领取失败'
  } finally {
    promoRedeeming.value = false
  }
}
const previewImage = ref('')

const paymentPhase = ref<'select' | 'paying'>('select')

interface CreateOrderOptions {
  openid?: string
  wechatResumeToken?: string
  paymentType?: string
  isResume?: boolean
  mobileQrFallbackAttempted?: boolean
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

const visibleMethods = computed(() => getVisibleMethods(checkout.value.methods))
const enabledMethods = computed(() => Object.keys(visibleMethods.value))
const validAmount = computed(() => amount.value ?? 0)

// Adaptive grid: center single card, 2-col for 2 plans, 3-col for 3+
const sortedPlans = computed(() => {
  const plans = [...checkout.value.plans].sort((a, b) => a.price - b.price)
  return plans.map(p => ({ ...p, _recommended: p.name === 'Standard' || p.name === '进阶' || p.group_name === 'Standard' || p.group_name === '进阶' }))
})

function planFeatures(plan: SubscriptionPlan | null): string[] {
  if (!plan || !plan.features) return []
  if (Array.isArray(plan.features)) return plan.features.filter(Boolean)
  return String(plan.features).split('\n').map(item => item.trim()).filter(Boolean)
}

function effectivePlanAmount(plan: SubscriptionPlan): number {
  return Number(plan.purchase_quote?.display_amount ?? plan.purchase_quote?.amount ?? plan.price)
}

function planDisplayAmount(plan: SubscriptionPlan): string {
  const amount = effectivePlanAmount(plan)
  return Number.isInteger(amount) ? String(amount) : amount.toFixed(2)
}

function planPriceSuffix(plan: SubscriptionPlan): string {
  if (plan.purchase_quote?.action === 'upgrade') return '升档补差价'
  if (plan.purchase_quote?.action === 'extend') return `/${plan.validity_days}天延期`
  if (plan.purchase_quote?.blocked) return '不可降档'
  return `/${plan.validity_days}天`
}

function planButtonText(plan: SubscriptionPlan): string {
  if (plan.purchase_quote?.blocked) return '不可降档'
  if (plan.purchase_quote?.action === 'upgrade') return '余额升档'
  if (plan.purchase_quote?.action === 'extend') return '余额延期'
  return '余额购买'
}

// Check if an amount fits a method's [min, max]. 0 = no limit.
function amountFitsMethod(amt: number, methodType: string): boolean {
  if (amt <= 0) return true
  const ml = visibleMethods.value[methodType]
  if (!ml) return false
  if (ml.single_min > 0 && amt < ml.single_min) return false
  if (ml.single_max > 0 && amt > ml.single_max) return false
  return true
}

// Selected method's limits (for validation and error messages)
const selectedLimit = computed(() => visibleMethods.value[selectedMethod.value])

// Auto-switch to first available method when current selection can't handle the amount
watch(() => [validAmount.value, selectedMethod.value] as const, ([amt, method]) => {
  if (amt <= 0 || amountFitsMethod(amt, method)) return
  const available = enabledMethods.value.find((m) => amountFitsMethod(amt, m))
  if (available) selectedMethod.value = available
})

function selectPlan(plan: SubscriptionPlan) {
  selectedPlan.value = plan
  errorMessage.value = ''
}

async function purchaseSelectedPlanWithBalance() {
  if (!selectedPlan.value) return
  openBalancePurchaseConfirm(selectedPlan.value)
}

function openBalancePurchaseConfirm(plan: SubscriptionPlan) {
  if (!plan || submitting.value || plan.purchase_quote?.blocked) return
  selectedPlan.value = plan
  pendingBalancePlan.value = plan
  showBalancePurchaseConfirm.value = true
}

function cancelBalancePurchase() {
  showBalancePurchaseConfirm.value = false
  pendingBalancePlan.value = null
}

async function confirmBalancePurchase() {
  const plan = pendingBalancePlan.value
  showBalancePurchaseConfirm.value = false
  pendingBalancePlan.value = null
  if (!plan) return
  await executeBalancePurchase(plan)
}

async function purchasePlanWithBalance(plan: SubscriptionPlan) {
  openBalancePurchaseConfirm(plan)
}

async function executeBalancePurchase(plan: SubscriptionPlan) {
  if (!plan || submitting.value) return
  selectedPlan.value = plan
  submitting.value = true
  errorMessage.value = ''
  errorHintMessage.value = ''
  try {
    const result = await paymentStore.purchaseSubscriptionWithBalance(plan.id)
    await Promise.all([
      authStore.refreshUser().catch(() => {}),
      subscriptionStore.fetchActiveSubscriptions(true).catch(() => {}),
    ])
    selectedPlan.value = null
    appStore.showSuccess?.(`${planButtonText(plan)}成功，已扣除 $${Number(result.amount || effectivePlanAmount(plan)).toFixed(2)}`)
  } catch (err: unknown) {
    const metadata = (err && typeof err === 'object' && 'metadata' in err) ? (err as any).metadata : null
    if ((err as any)?.reason === 'INSUFFICIENT_BALANCE') {
      const balance = metadata?.balance ?? user.value?.balance?.toFixed?.(2) ?? '0.00'
      const required = metadata?.required ?? effectivePlanAmount(plan).toFixed(2)
      errorMessage.value = `余额不足：当前可用 $${balance}，套餐需要 $${required}`
      errorHintMessage.value = '请联系 QQ 591719412 充值后再购买。'
    } else {
      errorMessage.value = extractApiErrorMessage(err) || '购买失败'
      errorHintMessage.value = ''
    }
    appStore.showError(buildPaymentErrorToastMessage(errorMessage.value, errorHintMessage.value))
  } finally {
    submitting.value = false
  }
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
    const stripeRouteUrl = result.client_secret
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
    const decision = decidePaymentLaunch(result, {
      visibleMethod,
      orderType,
      isMobile: isMobileDevice(),
      isWechatBrowser: typeof window !== 'undefined' && /MicroMessenger/i.test(window.navigator.userAgent),
      stripePopupUrl: stripeRouteUrl,
      stripeRouteUrl,
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
        }
      }
    }
  } catch (err: unknown) { appStore.showError(extractI18nErrorMessage(err, t, 'payment.errors', t('common.error'))) }
  finally { loading.value = false }
  // Fetch active subscriptions (uses cache, non-blocking)
  subscriptionStore.fetchActiveSubscriptions().catch(() => {})
})
</script>
