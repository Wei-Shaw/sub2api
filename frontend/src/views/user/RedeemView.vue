<template>
  <AppLayout>
    <div class="mx-auto max-w-2xl space-y-6">
      <!-- Current Balance: meter reading -->
      <div class="meter">
        <div class="meter-cell meter-cell-current">
          <span class="meter-label">{{ t('redeem.currentBalance') }}</span>
          <span class="meter-value">${{ user?.balance?.toFixed(2) || '0.00' }}</span>
        </div>
        <div class="meter-cell">
          <span class="meter-label">{{ t('redeem.concurrency') }}</span>
          <span class="meter-value">{{ user?.concurrency || 0 }}</span>
          <span class="meter-sub">{{ t('redeem.requests') }}</span>
        </div>
      </div>

      <!-- Redeem stub: code strip / perforation / result -->
      <div class="stub">
        <form @submit.prevent="handleRedeem" class="space-y-4 px-5 py-5">
          <div>
            <label for="code" class="input-label">
              {{ t('redeem.redeemCodeLabel') }}
            </label>
            <div class="mt-1 border border-dashed border-border-strong bg-surface-sunken p-2">
              <input
                id="code"
                v-model="redeemCode"
                type="text"
                required
                :placeholder="t('redeem.redeemCodePlaceholder')"
                :disabled="submitting"
                autocomplete="off"
                spellcheck="false"
                class="input py-3 text-center font-mono text-h3 tracking-widest"
              />
            </div>
            <p class="input-hint">
              {{ t('redeem.redeemCodeHint') }}
            </p>
          </div>

          <button
            type="submit"
            :disabled="!redeemCode || submitting"
            class="btn btn-primary btn-lg w-full"
          >
            <span v-if="submitting" class="spinner h-4 w-4"></span>
            {{ submitting ? t('redeem.redeeming') : t('redeem.redeemButton') }}
          </button>
        </form>

        <template v-if="redeemResult || errorMessage">
          <div class="stub-perforation" />

          <!-- Success Message -->
          <div v-if="redeemResult" class="space-y-3 px-5 py-4">
            <h3 class="flex items-center gap-2 text-h3 font-bold text-success">
              <Icon name="checkCircle" size="sm" />
              {{ t('redeem.redeemSuccess') }}
            </h3>
            <div class="divide-y divide-border border-y border-border text-body">
              <p v-if="redeemResult.type === 'balance'" class="py-2 font-semibold tabular-nums text-success">
                {{ t('redeem.added') }}: ${{ redeemResult.value.toFixed(2) }}
              </p>
              <p v-else-if="redeemResult.type === 'concurrency'" class="py-2 font-semibold tabular-nums text-success">
                {{ t('redeem.added') }}: {{ redeemResult.value }}
                {{ t('redeem.concurrentRequests') }}
              </p>
              <p v-else-if="redeemResult.type === 'subscription'" class="py-2 font-semibold text-success">
                {{ t('redeem.subscriptionAssigned') }}
                <span v-if="redeemResult.group_name"> - {{ redeemResult.group_name }}</span>
                <span v-if="redeemResult.validity_days">
                  ({{
                    t('redeem.subscriptionDays', { days: redeemResult.validity_days })
                  }})</span
                >
              </p>
              <p v-if="redeemResult.new_balance !== undefined" class="py-2 text-fg">
                {{ t('redeem.newBalance') }}:
                <span class="font-semibold tabular-nums">${{ redeemResult.new_balance.toFixed(2) }}</span>
              </p>
              <p v-if="redeemResult.new_concurrency !== undefined" class="py-2 text-fg">
                {{ t('redeem.newConcurrency') }}:
                <span class="font-semibold tabular-nums"
                  >{{ redeemResult.new_concurrency }} {{ t('redeem.requests') }}</span
                >
              </p>
            </div>
            <router-link
              :to="redeemResult.type === 'subscription' ? '/subscriptions' : '/keys'"
              class="btn btn-secondary btn-sm"
            >
              {{ redeemResult.type === 'subscription' ? t('nav.mySubscriptions') : t('nav.apiKeys') }}
            </router-link>
          </div>

          <!-- Error Message -->
          <div v-if="errorMessage" class="space-y-2 px-5 py-4">
            <span class="badge badge-danger px-2.5 py-1 text-label">
              <Icon name="exclamationCircle" size="sm" />
              {{ t('redeem.redeemFailed') }}
            </span>
            <p class="text-body text-danger">
              {{ errorMessage }}
            </p>
          </div>
        </template>
      </div>

      <!-- Information -->
      <section>
        <h2 class="bill-section-title">
          {{ t('redeem.aboutCodes') }}
        </h2>
        <ul class="divide-y divide-border text-body text-fg-muted">
          <li class="py-2">{{ t('redeem.codeRule1') }}</li>
          <li class="py-2">{{ t('redeem.codeRule2') }}</li>
          <li class="py-2">
            {{ t('redeem.codeRule3') }}
            <span
              v-if="contactInfo"
              class="ml-1.5 font-medium text-fg"
            >
              {{ contactInfo }}
            </span>
          </li>
          <li class="py-2">{{ t('redeem.codeRule4') }}</li>
        </ul>
      </section>

      <!-- Recent Activity -->
      <div class="card">
        <div class="card-header">
          <h2 class="card-title">
            {{ t('redeem.recentActivity') }}
          </h2>
        </div>
        <!-- Loading State -->
        <div v-if="loadingHistory" class="flex items-center justify-center py-8">
          <span class="spinner h-6 w-6 text-accent"></span>
        </div>

        <!-- History List: ruled register -->
        <div v-else-if="history.length > 0" class="divide-y divide-border">
          <div
            v-for="item in history"
            :key="item.id"
            class="flex items-start justify-between gap-4 px-5 py-3"
          >
            <div class="min-w-0">
              <p class="text-body font-medium text-fg">
                {{ getHistoryItemTitle(item) }}
              </p>
              <p class="text-meta tabular-nums text-fg-muted">
                {{ formatDateTime(item.used_at) }}
              </p>
            </div>
            <div class="min-w-0 text-right">
              <p
                :class="[
                  'text-body font-semibold tabular-nums',
                  isBalanceType(item.type)
                    ? item.value >= 0
                      ? 'text-success'
                      : 'text-danger'
                    : isSubscriptionType(item.type)
                      ? 'text-fg-muted'
                      : item.value >= 0
                        ? 'text-accent'
                        : 'text-warning'
                ]"
              >
                {{ formatHistoryValue(item) }}
              </p>
              <p
                v-if="!isAdminAdjustment(item.type)"
                class="font-mono text-meta text-fg-subtle"
              >
                {{ item.code.slice(0, 8) }}...
              </p>
              <p v-else class="text-meta text-fg-subtle">
                {{ t('redeem.adminAdjustment') }}
              </p>
              <!-- Display notes for admin adjustments -->
              <p
                v-if="item.notes"
                class="mt-1 max-w-[200px] truncate text-meta italic text-fg-muted"
                :title="item.notes"
              >
                {{ item.notes }}
              </p>
            </div>
          </div>
        </div>

        <!-- Empty State -->
        <div v-else class="empty-state py-8">
          <p class="text-body text-fg-muted">
            {{ t('redeem.historyWillAppear') }}
          </p>
        </div>
        <div class="mt-4 flex flex-wrap items-center justify-between gap-3 text-meta text-fg-muted">
          <span>{{ t('common.total') }}: {{ historyTotal }} {{ t('pagination.results') }}</span>
          <label>
            {{ t('pagination.perPage') }}
            <select
              v-model="historyPageSize"
              class="input w-20"
              :disabled="loadingHistory || submitting"
              @change="fetchHistory(1)"
            >
              <option v-for="size in [20, 50, 100]" :key="size" :value="size">{{ size }}</option>
            </select>
          </label>
          <button
            class="btn btn-secondary"
            :disabled="loadingHistory || submitting || historyPage <= 1"
            @click="fetchHistory(historyPage - 1)"
          >{{ t('pagination.previous') }}</button>
          <button
            class="btn btn-secondary"
            :disabled="loadingHistory || submitting || historyPage * historyPageSize >= historyTotal"
            @click="fetchHistory(historyPage + 1)"
          >{{ t('pagination.next') }}</button>
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
import { redeemAPI, type RedeemHistoryItem } from '@/api'
import AppLayout from '@/components/layout/AppLayout.vue'
import Icon from '@/components/icons/Icon.vue'
import { formatDateTime } from '@/utils/format'
import { extractI18nErrorMessage } from '@/utils/apiError'

const { t } = useI18n()
const authStore = useAuthStore()
const appStore = useAppStore()
const subscriptionStore = useSubscriptionStore()

const user = computed(() => authStore.user)

const redeemCode = ref('')
const submitting = ref(false)
// 接口只返回兑换码记录；新余额/并发来自刷新后的用户，分组名来自刷新后的订阅列表。
const redeemResult = ref<(RedeemHistoryItem & {
  new_balance?: number
  new_concurrency?: number
  group_name?: string
}) | null>(null)
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
  } else if (item.type === 'invitation') {
    return t('redeem.invitationCodeUsed')
  }
  return t('common.unknown')
}

const formatHistoryValue = (item: RedeemHistoryItem) => {
  // 注册时使用的邀请码没有可兑换的数值
  if (item.type === 'invitation') return ''
  if (isBalanceType(item.type)) {
    const sign = item.value >= 0 ? '+' : '-'
    return `${sign}$${Math.abs(item.value).toFixed(2)}`
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
      const refreshed = await authStore.refreshUser({ force: true })
      redeemResult.value = {
        ...result,
        new_balance: result.type === 'balance' ? refreshed.balance : undefined,
        new_concurrency: result.type === 'concurrency' ? refreshed.concurrency : undefined,
      }
    } catch (error) {
      console.error('Failed to refresh user after redeem:', error)
      appStore.showWarning(t('redeem.userRefreshFailed'))
    }

    // If subscription type, immediately refresh subscription status
    if (result.type === 'subscription') {
      try {
        const subscriptions = await subscriptionStore.fetchActiveSubscriptions(true) // force refresh
        const groupName = subscriptions.find((sub) => sub.group_id === result.group_id)?.group?.name
        redeemResult.value = { ...result, ...redeemResult.value, group_name: groupName }
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
  } catch (error: unknown) {
    errorMessage.value = extractI18nErrorMessage(error, t, 'redeem.errors', t('redeem.failedToRedeem'))

    appStore.showError(t('redeem.redeemFailed'))
  } finally {
    submitting.value = false
  }
}

onMounted(async () => {
  fetchHistory()
  try {
    const settings = await appStore.fetchPublicSettings()
    contactInfo.value = settings?.contact_info || ''
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
