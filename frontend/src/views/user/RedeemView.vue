<template>
  <AppLayout>
    <div class="grid grid-cols-1 items-start gap-6 lg:grid-cols-3 lg:grid-rows-[auto_1fr]">
      <!-- Main: redeem + history -->
      <div class="min-w-0 space-y-6 lg:col-span-2 lg:row-span-2 lg:row-start-1">
        <!-- Redeem Form -->
        <div class="card">
          <div class="p-5">
            <form @submit.prevent="handleRedeem" class="space-y-4">
              <div>
                <label for="code" class="input-label">
                  {{ t('redeem.redeemCodeLabel') }}
                </label>
                <div class="relative mt-1">
                  <div class="pointer-events-none absolute inset-y-0 left-0 flex items-center pl-3.5">
                    <Icon name="gift" size="sm" class="text-gray-400 dark:text-dark-500" />
                  </div>
                  <input
                    id="code"
                    v-model="redeemCode"
                    type="text"
                    required
                    :placeholder="t('redeem.redeemCodePlaceholder')"
                    :disabled="submitting"
                    class="input h-11 pl-11 font-mono tracking-wide"
                  />
                </div>
                <p class="input-hint">
                  {{ t('redeem.redeemCodeHint') }}
                </p>
              </div>

              <button
                type="submit"
                :disabled="!redeemCode || submitting"
                class="btn btn-primary w-full sm:w-auto sm:min-w-[8rem]"
              >
                <svg
                  v-if="submitting"
                  class="h-4 w-4 animate-spin"
                  fill="none"
                  viewBox="0 0 24 24"
                >
                  <circle
                    class="opacity-25"
                    cx="12"
                    cy="12"
                    r="10"
                    stroke="currentColor"
                    stroke-width="4"
                  ></circle>
                  <path
                    class="opacity-75"
                    fill="currentColor"
                    d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"
                  ></path>
                </svg>
                <Icon v-else name="checkCircle" size="sm" />
                {{ submitting ? t('redeem.redeeming') : t('redeem.redeemButton') }}
              </button>
            </form>
          </div>
        </div>

        <!-- Success Message -->
        <transition name="fade">
          <div
            v-if="redeemResult"
            class="card border-emerald-200/80 bg-emerald-50/70 dark:border-emerald-500/20 dark:bg-emerald-500/10"
          >
            <div class="p-4">
              <div class="flex items-start gap-3">
                <div
                  class="flex h-8 w-8 flex-shrink-0 items-center justify-center rounded-lg bg-emerald-100 dark:bg-emerald-500/15"
                >
                  <Icon name="checkCircle" size="sm" class="text-emerald-600 dark:text-emerald-400" />
                </div>
                <div class="flex-1">
                  <h3 class="text-sm font-semibold text-emerald-800 dark:text-emerald-300">
                    {{ t('redeem.redeemSuccess') }}
                  </h3>
                  <div class="mt-2 text-sm text-emerald-700 dark:text-emerald-400">
                    <p>{{ redeemResult.message }}</p>
                    <div class="mt-3 space-y-1">
                      <p v-if="redeemResult.type === 'balance'" class="font-medium">
                        {{ t('redeem.added') }}: ${{ redeemResult.value.toFixed(2) }}
                      </p>
                      <p v-else-if="redeemResult.type === 'concurrency'" class="font-medium">
                        {{ t('redeem.added') }}: {{ redeemResult.value }}
                        {{ t('redeem.concurrentRequests') }}
                      </p>
                      <p v-else-if="redeemResult.type === 'subscription'" class="font-medium">
                        {{ t('redeem.subscriptionAssigned') }}
                        <span v-if="redeemResult.group_name"> - {{ redeemResult.group_name }}</span>
                        <span v-if="redeemResult.validity_days">
                          ({{
                            t('redeem.subscriptionDays', { days: redeemResult.validity_days })
                          }})</span
                        >
                      </p>
                      <p v-if="redeemResult.new_balance !== undefined">
                        {{ t('redeem.newBalance') }}:
                        <span class="font-semibold">${{ redeemResult.new_balance.toFixed(2) }}</span>
                      </p>
                      <p v-if="redeemResult.new_concurrency !== undefined">
                        {{ t('redeem.newConcurrency') }}:
                        <span class="font-semibold"
                          >{{ redeemResult.new_concurrency }} {{ t('redeem.requests') }}</span
                        >
                      </p>
                    </div>
                  </div>
                </div>
              </div>
            </div>
          </div>
        </transition>

        <!-- Error Message -->
        <transition name="fade">
          <div
            v-if="errorMessage"
            class="card border-red-200/80 bg-red-50/70 dark:border-red-500/20 dark:bg-red-500/10"
          >
            <div class="p-4">
              <div class="flex items-start gap-3">
                <div
                  class="flex h-8 w-8 flex-shrink-0 items-center justify-center rounded-lg bg-red-100 dark:bg-red-500/15"
                >
                  <Icon
                    name="exclamationCircle"
                    size="sm"
                    class="text-red-600 dark:text-red-400"
                  />
                </div>
                <div class="flex-1">
                  <h3 class="text-sm font-semibold text-red-800 dark:text-red-300">
                    {{ t('redeem.redeemFailed') }}
                  </h3>
                  <p class="mt-2 text-sm text-red-700 dark:text-red-400">
                    {{ errorMessage }}
                  </p>
                </div>
              </div>
            </div>
          </div>
        </transition>

        <!-- Recent Activity -->
        <div class="card">
          <div class="card-header">
            <h2 class="text-sm font-semibold text-gray-900 dark:text-white">
              {{ t('redeem.recentActivity') }}
            </h2>
          </div>
          <div class="px-5 pb-4 pt-1">
            <!-- Loading State -->
            <div v-if="loadingHistory" class="flex items-center justify-center py-8">
              <svg class="h-6 w-6 animate-spin text-primary-500" fill="none" viewBox="0 0 24 24">
                <circle
                  class="opacity-25"
                  cx="12"
                  cy="12"
                  r="10"
                  stroke="currentColor"
                  stroke-width="4"
                ></circle>
                <path
                  class="opacity-75"
                  fill="currentColor"
                  d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"
                ></path>
              </svg>
            </div>

            <!-- History List -->
            <div v-else-if="history.length > 0" class="divide-y divide-gray-100 dark:divide-dark-800">
              <div
                v-for="item in history"
                :key="item.id"
                class="flex items-center justify-between gap-4 py-3"
              >
                <div class="flex min-w-0 items-center gap-3">
                  <div
                    :class="[
                      'flex h-9 w-9 flex-shrink-0 items-center justify-center rounded-lg',
                      isBalanceType(item.type)
                        ? item.value >= 0
                          ? 'bg-emerald-50 dark:bg-emerald-500/10'
                          : 'bg-red-50 dark:bg-red-500/10'
                        : isSubscriptionType(item.type)
                          ? 'bg-purple-50 dark:bg-purple-500/10'
                          : item.value >= 0
                            ? 'bg-blue-50 dark:bg-blue-500/10'
                            : 'bg-orange-50 dark:bg-orange-500/10'
                    ]"
                  >
                    <!-- 余额类型图标 -->
                    <Icon
                      v-if="isBalanceType(item.type)"
                      name="dollar"
                      size="sm"
                      :class="
                        item.value >= 0
                          ? 'text-emerald-600 dark:text-emerald-400'
                          : 'text-red-600 dark:text-red-400'
                      "
                    />
                    <!-- 订阅类型图标 -->
                    <Icon
                      v-else-if="isSubscriptionType(item.type)"
                      name="badge"
                      size="sm"
                      class="text-purple-600 dark:text-purple-400"
                    />
                    <!-- 并发类型图标 -->
                    <Icon
                      v-else
                      name="bolt"
                      size="sm"
                      :class="
                        item.value >= 0
                          ? 'text-blue-600 dark:text-blue-400'
                          : 'text-orange-600 dark:text-orange-400'
                      "
                    />
                  </div>
                  <div>
                    <p class="text-sm font-medium text-gray-900 dark:text-white">
                      {{ getHistoryItemTitle(item) }}
                    </p>
                    <p class="text-xs text-gray-500 dark:text-dark-400">
                      {{ formatDateTime(item.used_at) }}
                    </p>
                  </div>
                </div>
                <div class="text-right">
                  <p
                    :class="[
                      'text-sm font-semibold tabular-nums',
                      isBalanceType(item.type)
                        ? item.value >= 0
                          ? 'text-emerald-600 dark:text-emerald-400'
                          : 'text-red-600 dark:text-red-400'
                        : isSubscriptionType(item.type)
                          ? 'text-purple-600 dark:text-purple-400'
                          : item.value >= 0
                            ? 'text-blue-600 dark:text-blue-400'
                            : 'text-orange-600 dark:text-orange-400'
                    ]"
                  >
                    {{ formatHistoryValue(item) }}
                  </p>
                  <p
                    v-if="!isAdminAdjustment(item.type)"
                    class="font-mono text-xs text-gray-400 dark:text-dark-500"
                  >
                    {{ item.code.slice(0, 8) }}...
                  </p>
                  <p v-else class="text-xs text-gray-400 dark:text-dark-500">
                    {{ t('redeem.adminAdjustment') }}
                  </p>
                  <!-- Display notes for admin adjustments -->
                  <p
                    v-if="item.notes"
                    class="mt-1 text-xs text-gray-500 dark:text-dark-400 italic max-w-[200px] truncate"
                    :title="item.notes"
                  >
                    {{ item.notes }}
                  </p>
                </div>
              </div>
            </div>

            <!-- Empty State -->
            <div v-else class="empty-state py-10">
              <div
                class="mb-3 flex h-11 w-11 items-center justify-center rounded-xl bg-gray-50 ring-1 ring-inset ring-gray-200/70 dark:bg-dark-800 dark:ring-dark-700"
              >
                <Icon name="clock" size="md" class="text-gray-400 dark:text-dark-500" />
              </div>
              <p class="text-sm text-gray-500 dark:text-dark-400">
                {{ t('redeem.historyWillAppear') }}
              </p>
            </div>
            <div class="mt-2 flex flex-wrap items-center gap-3 border-t border-gray-100 pt-4 text-sm text-gray-500 dark:border-dark-800 dark:text-dark-400">
              <span class="tabular-nums">{{ t('common.total') }}: {{ historyTotal }} {{ t('pagination.results') }}</span>
              <label class="flex items-center gap-2">
                {{ t('pagination.perPage') }}
                <select
                  v-model="historyPageSize"
                  class="input w-20 py-1.5"
                  :disabled="loadingHistory || submitting"
                  @change="fetchHistory(1)"
                >
                  <option v-for="size in [20, 50, 100]" :key="size" :value="size">{{ size }}</option>
                </select>
              </label>
              <button
                class="btn btn-secondary btn-sm ml-auto"
                :disabled="loadingHistory || submitting || historyPage <= 1"
                @click="fetchHistory(historyPage - 1)"
              >{{ t('pagination.previous') }}</button>
              <button
                class="btn btn-secondary btn-sm"
                :disabled="loadingHistory || submitting || historyPage * historyPageSize >= historyTotal"
                @click="fetchHistory(historyPage + 1)"
              >{{ t('pagination.next') }}</button>
            </div>
          </div>
        </div>
      </div>

      <!-- Aside: balance leads on mobile, rules follow the main column -->
      <div class="order-first lg:order-none lg:col-start-3 lg:row-start-1">
        <!-- Current Balance Card -->
        <div class="kpi-card p-5">
          <div class="kpi-head">
            <p class="kpi-label text-sm">{{ t('redeem.currentBalance') }}</p>
            <Icon name="creditCard" size="sm" class="kpi-icon" />
          </div>
          <p class="kpi-value text-3xl">
            ${{ user?.balance?.toFixed(2) || '0.00' }}
          </p>
          <p class="kpi-meta text-sm">
            {{ t('redeem.concurrency') }}: {{ user?.concurrency || 0 }} {{ t('redeem.requests') }}
          </p>
        </div>

      </div>

      <div class="lg:col-start-3 lg:row-start-2">
        <!-- Information Card -->
        <div class="card">
          <div class="p-5">
            <div class="flex items-start gap-3">
              <Icon name="infoCircle" size="sm" class="mt-0.5 flex-shrink-0 text-gray-400 dark:text-dark-500" />
              <div class="min-w-0 flex-1">
                <h3 class="text-sm font-semibold text-gray-900 dark:text-white">
                  {{ t('redeem.aboutCodes') }}
                </h3>
                <ul
                  class="mt-2 list-outside list-disc space-y-1.5 pl-4 text-sm leading-relaxed text-gray-600 marker:text-gray-300 dark:text-dark-300 dark:marker:text-dark-600"
                >
                  <li>{{ t('redeem.codeRule1') }}</li>
                  <li>{{ t('redeem.codeRule2') }}</li>
                  <li>
                    {{ t('redeem.codeRule3') }}
                    <span
                      v-if="contactInfo"
                      class="badge badge-gray ml-1.5"
                    >
                      {{ contactInfo }}
                    </span>
                  </li>
                  <li>{{ t('redeem.codeRule4') }}</li>
                </ul>
              </div>
            </div>
          </div>
        </div>

      </div>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAuthStore } from '@/stores/auth'
import { useAppStore } from '@/stores/app'
import { useSubscriptionStore } from '@/stores/subscriptions'
import { redeemAPI, authAPI, type RedeemHistoryItem } from '@/api'
import AppLayout from '@/components/layout/AppLayout.vue'
import Icon from '@/components/icons/Icon.vue'
import { formatDateTime } from '@/utils/format'

const { t } = useI18n()
const authStore = useAuthStore()
const appStore = useAppStore()
const subscriptionStore = useSubscriptionStore()

const user = computed(() => authStore.user)

const redeemCode = ref('')
const submitting = ref(false)
const redeemResult = ref<{
  message: string
  type: string
  value: number
  new_balance?: number
  new_concurrency?: number
  group_name?: string
  validity_days?: number
} | null>(null)
const errorMessage = ref('')

// History data
const history = ref<RedeemHistoryItem[]>([])
const loadingHistory = ref(false)
const historyPage = ref(1)
const historyPageSize = ref(20)
const historyTotal = ref(0)
let historyRequest = 0
let loadedHistoryPageSize = 20
const contactInfo = ref('')

// Helper functions for history display
const isBalanceType = (type: string) => {
  return type === 'balance' || type === 'admin_balance'
}

const isSubscriptionType = (type: string) => {
  return type === 'subscription'
}

const isAdminAdjustment = (type: string) => {
  return type === 'admin_balance' || type === 'admin_concurrency'
}

const getHistoryItemTitle = (item: RedeemHistoryItem) => {
  if (item.type === 'balance') {
    return t('redeem.balanceAddedRedeem')
  } else if (item.type === 'admin_balance') {
    return item.value >= 0 ? t('redeem.balanceAddedAdmin') : t('redeem.balanceDeductedAdmin')
  } else if (item.type === 'concurrency') {
    return t('redeem.concurrencyAddedRedeem')
  } else if (item.type === 'admin_concurrency') {
    return item.value >= 0 ? t('redeem.concurrencyAddedAdmin') : t('redeem.concurrencyReducedAdmin')
  } else if (item.type === 'subscription') {
    return t('redeem.subscriptionAssigned')
  }
  return t('common.unknown')
}

const formatHistoryValue = (item: RedeemHistoryItem) => {
  if (isBalanceType(item.type)) {
    const sign = item.value >= 0 ? '+' : ''
    return `${sign}$${item.value.toFixed(2)}`
  } else if (isSubscriptionType(item.type)) {
    // 订阅类型显示有效天数和分组名称
    const days = item.validity_days || Math.round(item.value)
    const groupName = item.group?.name || ''
    return groupName ? `${days}${t('redeem.days')} - ${groupName}` : `${days}${t('redeem.days')}`
  } else {
    const sign = item.value >= 0 ? '+' : ''
    return `${sign}${item.value} ${t('redeem.requests')}`
  }
}

const fetchHistory = async (page = 1) => {
  const request = ++historyRequest
  const pageSize = historyPageSize.value
  loadingHistory.value = true
  try {
    const result = await redeemAPI.getHistory(page, pageSize)
    if (request !== historyRequest) return
    history.value = result.items
    historyTotal.value = result.total
    historyPage.value = page
    historyPageSize.value = pageSize
    loadedHistoryPageSize = pageSize
  } catch (error) {
    if (request !== historyRequest) return
    historyPageSize.value = loadedHistoryPageSize
    appStore.showError(t('redeem.historyLoadFailed'))
    console.error('Failed to fetch history:', error)
  } finally {
    if (request === historyRequest) loadingHistory.value = false
  }
}

const handleRedeem = async () => {
  if (!redeemCode.value.trim()) {
    appStore.showError(t('redeem.pleaseEnterCode'))
    return
  }

  submitting.value = true
  errorMessage.value = ''
  redeemResult.value = null

  try {
    const result = await redeemAPI.redeem(redeemCode.value.trim())

    redeemResult.value = result

    // Refresh user data to get updated balance/concurrency
    try {
      await authStore.refreshUser()
    } catch (error) {
      console.error('Failed to refresh user after redeem:', error)
      appStore.showWarning(t('redeem.userRefreshFailed'))
    }

    // If subscription type, immediately refresh subscription status
    if (result.type === 'subscription') {
      try {
        await subscriptionStore.fetchActiveSubscriptions(true) // force refresh
      } catch (error) {
        console.error('Failed to refresh subscriptions after redeem:', error)
        appStore.showWarning(t('redeem.subscriptionRefreshFailed'))
      }
    }

    // Clear the input
    redeemCode.value = ''

    // Refresh history
    await fetchHistory()

    // Show success toast
    appStore.showSuccess(t('redeem.codeRedeemSuccess'))
  } catch (error: any) {
    errorMessage.value = error.response?.data?.detail || t('redeem.failedToRedeem')

    appStore.showError(t('redeem.redeemFailed'))
  } finally {
    submitting.value = false
  }
}

onMounted(async () => {
  fetchHistory()
  try {
    const settings = await authAPI.getPublicSettings()
    contactInfo.value = settings.contact_info || ''
  } catch (error) {
    console.error('Failed to load contact info:', error)
  }
})
</script>

<style scoped>
.fade-enter-active,
.fade-leave-active {
  transition: all 0.3s ease;
}

.fade-enter-from,
.fade-leave-to {
  opacity: 0;
  transform: translateY(-8px);
}
</style>
