<template>
  <AppLayout>
    <div class="billing-shell mx-auto max-w-[1480px] space-y-6">
      <!-- Loading State -->
      <div v-if="loading" class="flex justify-center py-12">
        <div
          class="h-8 w-8 animate-spin rounded-full border-2 border-primary-500 border-t-transparent"
        ></div>
      </div>

      <template v-else>
        <BillingHero :title="t('userSubscriptions.title')" :description="t('userSubscriptions.description')">
          <div class="billing-hero__stat">
            <span class="billing-icon"><Icon name="creditCard" size="md" /></span>
            <div><p>{{ t('userSubscriptions.totalSubscriptions') }}</p><strong>{{ subscriptions.length }}</strong></div>
          </div>
          <div class="billing-hero__stat">
            <span class="billing-icon"><Icon name="checkCircle" size="md" /></span>
            <div><p>{{ t('userSubscriptions.activeSubscriptions') }}</p><strong>{{ activeSubscriptionCount }}</strong></div>
          </div>
        </BillingHero>

        <!-- Empty State -->
        <div v-if="subscriptions.length === 0" class="relative overflow-hidden rounded-[1.75rem] border border-dashed border-gray-300 bg-white px-6 py-16 text-center shadow-sm dark:border-dark-600 dark:bg-dark-800/80 sm:px-12">
          <div class="pointer-events-none absolute inset-x-0 top-0 h-32 bg-gradient-to-b from-primary-50 to-transparent dark:from-primary-950/30" />
          <div class="relative">
            <div class="mx-auto mb-5 flex h-20 w-20 items-center justify-center rounded-3xl bg-gradient-to-br from-primary-100 to-cyan-100 shadow-inner dark:from-primary-900/50 dark:to-cyan-900/40">
              <Icon name="creditCard" size="xl" class="text-primary-600 dark:text-primary-300" />
            </div>
            <h3 class="mb-2 text-xl font-semibold tracking-tight text-gray-900 dark:text-white">
              {{ t('userSubscriptions.noActiveSubscriptions') }}
            </h3>
            <p class="mx-auto max-w-md text-sm leading-6 text-gray-500 dark:text-dark-400">
              {{ t('userSubscriptions.noActiveSubscriptionsDesc') }}
            </p>
          </div>
        </div>

        <!-- Subscriptions Grid -->
        <div v-else class="grid gap-6 lg:grid-cols-2">
            <div
              v-for="subscription in subscriptions"
              :key="subscription.id"
              class="billing-panel flex flex-col overflow-hidden"
            >
              <div :class="['h-1.5', platformAccentBarClass(subscription.group?.platform || '')]" />
              <!-- Header -->
              <div
                class="flex flex-wrap items-start justify-between gap-4 border-b border-gray-100 p-6 dark:border-white/10"
              >
                <div class="flex min-w-0 flex-1 items-start gap-3">
                  <div :class="['flex h-11 w-11 shrink-0 items-center justify-center rounded-2xl border', platformBadgeClass(subscription.group?.platform || '')]">
                    <Icon name="creditCard" size="md" />
                  </div>
                  <div class="min-w-0">
                    <div class="flex flex-wrap items-center gap-2">
                      <h3 class="break-words text-lg font-semibold tracking-tight text-gray-900 dark:text-white">
                        {{ subscription.group?.name || `Group #${subscription.group_id}` }}
                      </h3>
                      <span :class="['rounded-md border px-2 py-0.5 text-[11px] font-medium', platformBadgeClass(subscription.group?.platform || '')]">
                        {{ platformLabel(subscription.group?.platform || '') }}
                      </span>
                    </div>
                    <p v-if="subscription.group?.description" class="mt-2 break-words text-sm leading-6 text-gray-500 dark:text-dark-400">
                      {{ subscription.group.description }}
                    </p>
                    <div class="mt-3 flex flex-wrap gap-x-3 gap-y-1 text-xs text-gray-500 dark:text-gray-400">
                      <span>{{ t('payment.planCard.rate') }}: ×{{ subscription.group?.rate_multiplier ?? 1 }}</span>
                      <span v-if="subscriptionHasPeakRate(subscription)" class="text-amber-700 dark:text-amber-300">
                        {{ t('payment.planCard.peakRate') }}: {{ subscriptionPeakRateLabel(subscription) }}
                      </span>
                    </div>
                  </div>
                </div>
                <div class="flex shrink-0 items-center gap-2">
                  <span
                    :class="[
                      'rounded-full px-3 py-1 text-xs font-medium',
                      subscription.status === 'active'
                        ? 'bg-emerald-100 text-emerald-700 dark:bg-emerald-900/40 dark:text-emerald-300'
                        : subscription.status === 'expired'
                          ? 'bg-gray-100 text-gray-600 dark:bg-dark-700 dark:text-gray-400'
                          : 'bg-red-100 text-red-700 dark:bg-red-900/40 dark:text-red-300'
                    ]"
                  >
                    {{ t(`userSubscriptions.status.${subscription.status}`) }}
                  </span>
                </div>
              </div>

              <!-- Usage Progress -->
              <div class="flex-1 space-y-6 p-6">
                <!-- Expiration Info -->
                <div v-if="subscription.expires_at" class="billing-inset flex flex-wrap items-center justify-between gap-2 px-4 py-3 text-sm">
                  <span class="text-gray-500 dark:text-dark-400">{{
                    t('userSubscriptions.expires')
                  }}</span>
                  <span :class="getExpirationClass(subscription.expires_at)">
                    {{ formatExpirationDate(subscription.expires_at) }}
                  </span>
                </div>
                <div v-else class="billing-inset flex flex-wrap items-center justify-between gap-2 px-4 py-3 text-sm">
                  <span class="text-gray-500 dark:text-dark-400">{{
                    t('userSubscriptions.expires')
                  }}</span>
                  <span class="text-gray-700 dark:text-gray-300">{{
                    t('userSubscriptions.noExpiration')
                  }}</span>
                </div>

                <!-- Daily Usage -->
                <div v-if="subscription.group?.daily_limit_usd" class="space-y-2">
                  <div class="flex flex-wrap items-center justify-between gap-2">
                    <span class="text-sm font-medium text-gray-700 dark:text-gray-300">
                      {{ t('userSubscriptions.daily') }}
                    </span>
                    <span class="text-sm text-gray-500 dark:text-dark-400">
                      {{ formatQuotaUsage(subscription.daily_usage_usd, subscription.group.daily_limit_usd, subscription.group) }}
                    </span>
                  </div>
                  <div class="relative h-2.5 overflow-hidden rounded-full bg-gray-100 dark:bg-white/10">
                    <div
                      class="absolute inset-y-0 left-0 rounded-full transition-all duration-300"
                      :class="
                        getProgressBarClass(
                          subscription.daily_usage_usd,
                          subscription.group.daily_limit_usd
                        )
                      "
                      :style="{
                        width: getProgressWidth(
                          subscription.daily_usage_usd,
                          subscription.group.daily_limit_usd
                        )
                      }"
                    ></div>
                  </div>
                  <p
                    v-if="subscription.daily_window_start"
                    class="text-xs text-gray-500 dark:text-dark-400"
                  >
                    {{ formatDailyUsageWindow(subscription) }}
                  </p>
                </div>

                <!-- Weekly Usage -->
                <div v-if="subscription.group?.weekly_limit_usd" class="space-y-2">
                  <div class="flex flex-wrap items-center justify-between gap-2">
                    <span class="text-sm font-medium text-gray-700 dark:text-gray-300">
                      {{ t('userSubscriptions.weekly') }}
                    </span>
                    <span class="text-sm text-gray-500 dark:text-dark-400">
                      {{ formatQuotaUsage(subscription.weekly_usage_usd, subscription.group.weekly_limit_usd, subscription.group) }}
                    </span>
                  </div>
                  <div class="relative h-2.5 overflow-hidden rounded-full bg-gray-100 dark:bg-white/10">
                    <div
                      class="absolute inset-y-0 left-0 rounded-full transition-all duration-300"
                      :class="
                        getProgressBarClass(
                          subscription.weekly_usage_usd,
                          subscription.group.weekly_limit_usd
                        )
                      "
                      :style="{
                        width: getProgressWidth(
                          subscription.weekly_usage_usd,
                          subscription.group.weekly_limit_usd
                        )
                      }"
                    ></div>
                  </div>
                  <p
                    v-if="subscription.weekly_window_start"
                    class="text-xs text-gray-500 dark:text-dark-400"
                  >
                    {{
                      t('userSubscriptions.resetIn', {
                        time: formatResetTime(subscription.weekly_window_start, 168)
                      })
                    }}
                  </p>
                </div>

                <!-- Monthly Usage -->
                <div v-if="subscription.group?.monthly_limit_usd" class="space-y-2">
                  <div class="flex flex-wrap items-center justify-between gap-2">
                    <span class="text-sm font-medium text-gray-700 dark:text-gray-300">
                      {{ t('userSubscriptions.monthly') }}
                    </span>
                    <span class="text-sm text-gray-500 dark:text-dark-400">
                      {{ formatQuotaUsage(subscription.monthly_usage_usd, subscription.group.monthly_limit_usd, subscription.group) }}
                    </span>
                  </div>
                  <div class="relative h-2.5 overflow-hidden rounded-full bg-gray-100 dark:bg-white/10">
                    <div
                      class="absolute inset-y-0 left-0 rounded-full transition-all duration-300"
                      :class="
                        getProgressBarClass(
                          subscription.monthly_usage_usd,
                          subscription.group.monthly_limit_usd
                        )
                      "
                      :style="{
                        width: getProgressWidth(
                          subscription.monthly_usage_usd,
                          subscription.group.monthly_limit_usd
                        )
                      }"
                    ></div>
                  </div>
                  <p
                    v-if="subscription.monthly_window_start"
                    class="text-xs text-gray-500 dark:text-dark-400"
                  >
                    {{
                      t('userSubscriptions.resetIn', {
                        time: formatResetTime(subscription.monthly_window_start, 720)
                      })
                    }}
                  </p>
                </div>

                <!-- No limits configured - Unlimited badge -->
                <div
                  v-if="
                    !subscription.group?.daily_limit_usd &&
                    !subscription.group?.weekly_limit_usd &&
                    !subscription.group?.monthly_limit_usd
                  "
                  class="flex items-center justify-center rounded-2xl border border-emerald-100 bg-gradient-to-r from-emerald-50 to-teal-50 py-7 dark:border-emerald-900/40 dark:from-emerald-900/20 dark:to-teal-900/20"
                >
                  <div class="flex items-center gap-3">
                    <span class="text-4xl text-emerald-600 dark:text-emerald-400">∞</span>
                    <div>
                      <p class="text-sm font-medium text-emerald-700 dark:text-emerald-300">
                        {{ t('userSubscriptions.unlimited') }}
                      </p>
                      <p class="text-xs text-emerald-600/70 dark:text-emerald-400/70">
                        {{ t('userSubscriptions.unlimitedDesc') }}
                      </p>
                    </div>
                  </div>
                </div>
              </div>
              <div v-if="subscription.status === 'active'" class="border-t border-gray-100 px-6 py-4 dark:border-dark-700">
                <button
                  type="button"
                  :class="['billing-primary-button w-full px-4 py-3 text-sm font-semibold transition-colors focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-primary-500', platformButtonClass(subscription.group?.platform || '')]"
                  @click="router.push({ path: '/purchase', query: { tab: 'subscription', group: String(subscription.group_id) } })"
                >
                  {{ t('payment.renewNow') }}
                </button>
              </div>
            </div>
        </div>
      </template>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { useQuotaUsageDisplay } from '@/composables/useSubscriptionUnit'
const formatQuotaUsage = useQuotaUsageDisplay()
import { ref, computed, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'
import { useAppStore } from '@/stores/app'
import subscriptionsAPI from '@/api/subscriptions'
import type { UserSubscription } from '@/types'
import AppLayout from '@/components/layout/AppLayout.vue'
import BillingHero from '@/components/payment/BillingHero.vue'
import Icon from '@/components/icons/Icon.vue'
import { formatDateTimeToMinute } from '@/utils/format'
import { hasPeakRate, formatPeakRateWindow, serverTimezoneLabel } from '@/utils/peak-rate'
import { platformAccentBarClass, platformBadgeClass, platformButtonClass, platformLabel } from '@/utils/platformColors'
import {
  getExpirationDateRelation,
  getRemainingDurationParts,
  isOneTimeDailyQuota,
  type RemainingDurationParts
} from '@/utils/subscriptionQuota'

const { t } = useI18n()
const router = useRouter()
const appStore = useAppStore()

const subscriptions = ref<UserSubscription[]>([])
const loading = ref(true)
const activeSubscriptionCount = computed(() => subscriptions.value.filter(subscription => subscription.status === 'active').length)

function subscriptionHasPeakRate(subscription: UserSubscription): boolean {
  return hasPeakRate(subscription.group)
}

function subscriptionPeakRateLabel(subscription: UserSubscription): string {
  return formatPeakRateWindow(subscription.group, serverTimezoneLabel(appStore.cachedPublicSettings?.server_utc_offset))
}

async function loadSubscriptions() {
  try {
    loading.value = true
    subscriptions.value = await subscriptionsAPI.getMySubscriptions()
  } catch (error) {
    console.error('Failed to load subscriptions:', error)
    appStore.showError(t('userSubscriptions.failedToLoad'))
  } finally {
    loading.value = false
  }
}

function getProgressWidth(used: number | undefined, limit: number | null | undefined): string {
  if (!limit || limit === 0) return '0%'
  const percentage = Math.min(((used || 0) / limit) * 100, 100)
  return `${percentage}%`
}

function getProgressBarClass(used: number | undefined, limit: number | null | undefined): string {
  if (!limit || limit === 0) return 'bg-gray-400'
  const percentage = ((used || 0) / limit) * 100
  if (percentage >= 90) return 'bg-red-500'
  if (percentage >= 70) return 'bg-orange-500'
  return 'bg-gradient-to-r from-emerald-500 to-teal-400'
}

function formatExpirationDate(expiresAt: string): string {
  const now = new Date()
  const expires = new Date(expiresAt)
  const diff = expires.getTime() - now.getTime()
  const days = Math.ceil(diff / (1000 * 60 * 60 * 24))
  const relation = getExpirationDateRelation(expires, now)

  if (relation === null) return ''

  if (relation === 'expired') {
    return t('userSubscriptions.status.expired')
  }

  const dateStr = formatDateTimeToMinute(expires)

  if (relation === 'today') {
    return `${dateStr} (${t('common.today')})`
  }
  if (relation === 'tomorrow') {
    return `${dateStr} (${t('common.tomorrow')})`
  }

  return t('userSubscriptions.daysRemaining', { days }) + ` (${dateStr})`
}

function getExpirationClass(expiresAt: string): string {
  const now = new Date()
  const expires = new Date(expiresAt)
  const diff = expires.getTime() - now.getTime()
  const days = Math.ceil(diff / (1000 * 60 * 60 * 24))

  if (diff <= 0) return 'text-red-600 dark:text-red-400 font-medium'
  if (days <= 3) return 'text-red-600 dark:text-red-400'
  if (days <= 7) return 'text-orange-600 dark:text-orange-400'
  return 'text-gray-700 dark:text-gray-300'
}

function formatDurationParts(parts: RemainingDurationParts): string {
  if (parts.days > 0) {
    return `${parts.days}d ${parts.hours}h`
  }

  if (parts.hours > 0) {
    return `${parts.hours}h ${parts.minutes}m`
  }

  return `${parts.minutes}m`
}

function formatDailyUsageWindow(subscription: UserSubscription): string {
  if (isOneTimeDailyQuota(subscription) && subscription.expires_at) {
    const parts = getRemainingDurationParts(subscription.expires_at)
    if (!parts) return t('userSubscriptions.windowNotActive')
    return t('userSubscriptions.quotaEndsIn', { time: formatDurationParts(parts) })
  }

  return t('userSubscriptions.resetIn', {
    time: formatResetTime(subscription.daily_window_start, 24)
  })
}

function formatResetTime(windowStart: string | null, windowHours: number): string {
  if (!windowStart) return t('userSubscriptions.windowNotActive')

  const start = new Date(windowStart)
  const end = new Date(start.getTime() + windowHours * 60 * 60 * 1000)
  const parts = getRemainingDurationParts(end)

  return parts ? formatDurationParts(parts) : t('userSubscriptions.windowNotActive')
}

onMounted(() => {
  loadSubscriptions()
})
</script>
