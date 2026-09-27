<template>
  <AppLayout>
    <div class="space-y-6">
      <!-- Loading State -->
      <div v-if="loading" class="flex justify-center py-12">
        <div class="spinner h-8 w-8 text-accent"></div>
      </div>

      <EmptyState
        v-else-if="loadError"
        class="card"
        :title="t('userSubscriptions.failedToLoad')"
        :description="t('errors.tryAgain')"
        :action-text="t('common.refresh')"
        :action-icon="false"
        @action="loadSubscriptions"
      />

      <!-- Empty State -->
      <div v-else-if="subscriptions.length === 0" class="card empty-state">
        <Icon name="creditCard" size="xl" class="empty-state-icon" />
        <h3 class="empty-state-title">
          {{ t('userSubscriptions.noActiveSubscriptions') }}
        </h3>
        <p class="empty-state-description">
          {{ t('userSubscriptions.noActiveSubscriptionsDesc') }}
        </p>
      </div>

      <!-- Subscriptions: one bill section per plan, usage windows as ruled rows with a ruler -->
      <div v-else class="space-y-6">
        <section
          v-for="subscription in subscriptions"
          :key="subscription.id"
          class="card"
        >
          <!-- Header -->
          <div class="card-header flex flex-wrap items-start justify-between gap-3">
            <div class="min-w-0">
              <div class="flex flex-wrap items-center gap-2">
                <h3 class="card-title break-words">
                  {{ subscription.group?.name || `Group #${subscription.group_id}` }}
                </h3>
                <span class="badge">
                  {{ platformLabel(subscription.group?.platform || '') }}
                </span>
                <span
                  :class="[
                    'badge',
                    subscription.status === 'active'
                      ? 'badge-success'
                      : subscription.status === 'expired'
                        ? 'badge-gray'
                        : 'badge-danger'
                  ]"
                >
                  {{ t(`userSubscriptions.status.${subscription.status}`) }}
                </span>
              </div>
              <p v-if="subscription.group?.description" class="mt-0.5 text-meta text-fg-muted">
                {{ subscription.group.description }}
              </p>
            </div>
            <button
              v-if="subscription.status === 'active'"
              class="btn btn-primary btn-sm"
              @click="router.push({ path: '/purchase', query: { tab: 'subscription', group: String(subscription.group_id) } })"
            >
              {{ t('payment.renewNow') }}
            </button>
          </div>

          <!-- Tier terms -->
          <dl class="divide-y divide-border border-b border-border text-label">
            <div class="flex items-center justify-between gap-3 px-5 py-2">
              <dt class="text-fg-muted">{{ t('payment.planCard.rate') }}</dt>
              <dd class="font-semibold tabular-nums text-fg">×{{ subscription.group?.rate_multiplier ?? 1 }}</dd>
            </div>
            <div v-if="subscriptionHasPeakRate(subscription)" class="flex items-center justify-between gap-3 px-5 py-2">
              <dt class="text-fg-muted">{{ t('payment.planCard.peakRate') }}</dt>
              <dd class="text-right font-semibold text-warning">{{ subscriptionPeakRateLabel(subscription) }}</dd>
            </div>
            <div class="flex items-center justify-between gap-3 px-5 py-2">
              <dt class="text-fg-muted">{{ t('userSubscriptions.expires') }}</dt>
              <dd v-if="subscription.expires_at" class="text-right tabular-nums" :class="getExpirationClass(subscription.expires_at)">
                {{ formatExpirationDate(subscription.expires_at) }}
              </dd>
              <dd v-else class="text-right text-fg">{{ t('userSubscriptions.noExpiration') }}</dd>
            </div>
          </dl>

          <!-- Usage rulers -->
          <div class="divide-y divide-border">
            <!-- Daily Usage -->
            <div v-if="subscription.group?.daily_limit_usd" class="space-y-1.5 px-5 py-3">
              <div class="flex items-baseline justify-between gap-3">
                <span class="text-label font-semibold text-fg">{{ t('userSubscriptions.daily') }}</span>
                <span class="text-label tabular-nums text-fg">
                  ${{ (subscription.daily_usage_usd || 0).toFixed(2) }}
                  <span class="text-fg-subtle">/ ${{ subscription.group.daily_limit_usd.toFixed(2) }}</span>
                </span>
              </div>
              <div class="progress">
                <div
                  class="progress-bar"
                  :class="getProgressBarClass(subscription.daily_usage_usd, subscription.group.daily_limit_usd)"
                  :style="{ width: getProgressWidth(subscription.daily_usage_usd, subscription.group.daily_limit_usd) }"
                ></div>
              </div>
              <p v-if="subscription.daily_window_start" class="text-meta text-fg-muted">
                {{ formatDailyUsageWindow(subscription) }}
              </p>
            </div>

            <!-- Weekly Usage -->
            <div v-if="subscription.group?.weekly_limit_usd" class="space-y-1.5 px-5 py-3">
              <div class="flex items-baseline justify-between gap-3">
                <span class="text-label font-semibold text-fg">{{ t('userSubscriptions.weekly') }}</span>
                <span class="text-label tabular-nums text-fg">
                  ${{ (subscription.weekly_usage_usd || 0).toFixed(2) }}
                  <span class="text-fg-subtle">/ ${{ subscription.group.weekly_limit_usd.toFixed(2) }}</span>
                </span>
              </div>
              <div class="progress">
                <div
                  class="progress-bar"
                  :class="getProgressBarClass(subscription.weekly_usage_usd, subscription.group.weekly_limit_usd)"
                  :style="{ width: getProgressWidth(subscription.weekly_usage_usd, subscription.group.weekly_limit_usd) }"
                ></div>
              </div>
              <p v-if="subscription.weekly_window_start" class="text-meta text-fg-muted">
                {{ t('userSubscriptions.resetIn', { time: formatResetTime(subscription.weekly_window_start, 168) }) }}
              </p>
            </div>

            <!-- Monthly Usage -->
            <div v-if="subscription.group?.monthly_limit_usd" class="space-y-1.5 px-5 py-3">
              <div class="flex items-baseline justify-between gap-3">
                <span class="text-label font-semibold text-fg">{{ t('userSubscriptions.monthly') }}</span>
                <span class="text-label tabular-nums text-fg">
                  ${{ (subscription.monthly_usage_usd || 0).toFixed(2) }}
                  <span class="text-fg-subtle">/ ${{ subscription.group.monthly_limit_usd.toFixed(2) }}</span>
                </span>
              </div>
              <div class="progress">
                <div
                  class="progress-bar"
                  :class="getProgressBarClass(subscription.monthly_usage_usd, subscription.group.monthly_limit_usd)"
                  :style="{ width: getProgressWidth(subscription.monthly_usage_usd, subscription.group.monthly_limit_usd) }"
                ></div>
              </div>
              <p v-if="subscription.monthly_window_start" class="text-meta text-fg-muted">
                {{ t('userSubscriptions.resetIn', { time: formatResetTime(subscription.monthly_window_start, 720) }) }}
              </p>
            </div>

            <!-- No limits configured -->
            <div
              v-if="
                !subscription.group?.daily_limit_usd &&
                !subscription.group?.weekly_limit_usd &&
                !subscription.group?.monthly_limit_usd
              "
              class="flex items-center gap-3 px-5 py-3"
            >
              <span class="text-h1 font-bold leading-none text-success">∞</span>
              <div>
                <p class="text-label font-semibold text-success-strong">{{ t('userSubscriptions.unlimited') }}</p>
                <p class="text-meta text-fg-muted">{{ t('userSubscriptions.unlimitedDesc') }}</p>
              </div>
            </div>
          </div>
        </section>
      </div>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'
import { useAppStore } from '@/stores/app'
import subscriptionsAPI from '@/api/subscriptions'
import type { UserSubscription } from '@/types'
import AppLayout from '@/components/layout/AppLayout.vue'
import Icon from '@/components/icons/Icon.vue'
import EmptyState from '@/components/common/EmptyState.vue'
import { formatDateTimeToMinute } from '@/utils/format'
import { hasPeakRate, formatPeakRateWindow, serverTimezoneLabel } from '@/utils/peak-rate'
import { platformLabel } from '@/utils/platformColors'
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
const loadError = ref(false)

function subscriptionHasPeakRate(subscription: UserSubscription): boolean {
  return hasPeakRate(subscription.group)
}

function subscriptionPeakRateLabel(subscription: UserSubscription): string {
  return formatPeakRateWindow(subscription.group, serverTimezoneLabel(appStore.cachedPublicSettings?.server_utc_offset))
}

async function loadSubscriptions() {
  try {
    loading.value = true
    loadError.value = false
    subscriptions.value = await subscriptionsAPI.getMySubscriptions()
  } catch (error) {
    console.error('Failed to load subscriptions:', error)
    loadError.value = true
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
  if (!limit || limit === 0) return 'bg-fg-subtle'
  const percentage = ((used || 0) / limit) * 100
  if (percentage >= 90) return 'bg-danger'
  if (percentage >= 70) return 'bg-warning'
  return ''
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

  if (diff <= 0) return 'text-danger font-semibold'
  if (days <= 3) return 'text-danger'
  if (days <= 7) return 'text-warning'
  return 'text-fg'
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
