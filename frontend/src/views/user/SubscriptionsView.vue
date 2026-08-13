<template>
  <AppLayout>
    <div class="space-y-6">
      <!-- Loading — flat hairline panels, no centred spinner on an empty page. -->
      <div v-if="loading" class="grid gap-4 lg:grid-cols-2" data-testid="subscriptions-loading">
        <div v-for="i in 2" :key="i" class="rounded border border-line bg-surface">
          <div class="border-b border-line px-4 py-3">
            <div class="skeleton h-3 w-32"></div>
          </div>
          <div class="space-y-3 p-4">
            <div class="skeleton h-3 w-full"></div>
            <div class="skeleton h-3 w-4/5"></div>
            <div class="skeleton h-3 w-2/3"></div>
          </div>
        </div>
      </div>

      <Surface v-else-if="subscriptions.length === 0">
        <div class="empty-state">
          <Icon name="creditCard" size="lg" class="mb-4 text-ink-disabled" />
          <h3 class="empty-state-title">{{ t('userSubscriptions.noActiveSubscriptions') }}</h3>
          <p class="empty-state-description">
            {{ t('userSubscriptions.noActiveSubscriptionsDesc') }}
          </p>
        </div>
      </Surface>

      <div v-else class="grid gap-4 lg:grid-cols-2">
        <!--
          One hairline panel per subscription. The platform is a category label,
          not a colour: the tinted border, the coloured dot, the coloured badge
          and the platform-coloured button are all gone, and with them six
          parallel `dark:` colour ramps.
        -->
        <Surface v-for="subscription in subscriptions" :key="subscription.id" flush>
          <div class="flex items-start justify-between gap-4 border-b border-line px-4 py-3">
            <div class="min-w-0">
              <div class="flex flex-wrap items-center gap-2">
                <h3 class="min-w-0 truncate text-sm font-medium text-ink">
                  {{ subscription.group?.name || t('payment.groupFallback', { id: subscription.group_id }) }}
                </h3>
                <Badge caps>{{ platformLabel(subscription.group?.platform || '') }}</Badge>
              </div>
              <p v-if="subscription.group?.description" class="mt-0.5 text-xs text-ink-tertiary">
                {{ subscription.group.description }}
              </p>
              <div class="mt-1 flex flex-wrap gap-x-3 gap-y-1 text-2xs text-ink-tertiary">
                <span>
                  {{ t('payment.planCard.rate') }}:
                  <span class="font-mono tabular-nums">×{{ subscription.group?.rate_multiplier ?? 1 }}</span>
                </span>
                <span v-if="subscriptionHasPeakRate(subscription)">
                  {{ t('payment.planCard.peakRate') }}: {{ subscriptionPeakRateLabel(subscription) }}
                </span>
              </div>
            </div>

            <div class="flex shrink-0 items-center gap-3">
              <StatusDot
                :tone="statusTone(subscription.status)"
                :label="statusLabel(subscription.status)"
                :muted="statusTone(subscription.status) === 'neutral'"
              />
              <Button
                v-if="subscription.status === 'active'"
                tone="accent"
                variant="outline"
                @click="router.push({ path: '/purchase', query: { tab: 'subscription', group: String(subscription.group_id) } })"
              >
                {{ t('payment.renewNow') }}
              </Button>
            </div>
          </div>

          <dl class="divide-y divide-line-subtle">
            <div class="flex items-baseline justify-between gap-4 px-4 py-2">
              <dt class="text-xs text-ink-secondary">{{ t('userSubscriptions.expires') }}</dt>
              <dd
                v-if="subscription.expires_at"
                class="font-mono text-xs tabular-nums"
                :class="expirationToneClass(subscription.expires_at)"
              >
                {{ formatExpirationDate(subscription.expires_at) }}
              </dd>
              <dd v-else class="text-xs text-ink">{{ t('userSubscriptions.noExpiration') }}</dd>
            </div>

            <!--
              Was three 8px rounded pill bars with a green/orange/red fill that
              was green on the overwhelming majority of rows — which is exactly
              how a colour stops meaning anything. `Meter` is a 4px flat rule
              that stays neutral until the value crosses a declared threshold,
              and the numbers underneath are the primary channel.
            -->
            <div
              v-for="window in usageWindows(subscription)"
              :key="window.key"
              class="space-y-1.5 px-4 py-3"
            >
              <Meter
                :label="window.label"
                :value="window.used"
                :max="window.limit"
                :warn-at="0.7"
                :danger-at="0.9"
              />
              <div class="flex flex-wrap items-baseline justify-between gap-x-4 gap-y-1">
                <span class="inline-flex items-baseline gap-1">
                  <NumCell :value="window.used" :precision="2" />
                  <span class="font-mono text-2xs text-ink-tertiary" aria-hidden="true">/</span>
                  <NumCell :value="window.limit" :precision="2" unit="USD" />
                </span>
                <span v-if="window.caption" class="text-2xs text-ink-tertiary">
                  {{ window.caption }}
                </span>
              </div>
            </div>

            <div
              v-if="hasNoLimits(subscription)"
              class="flex items-baseline justify-between gap-4 px-4 py-2"
            >
              <dt class="text-xs text-ink-secondary">{{ t('userSubscriptions.unlimited') }}</dt>
              <dd class="text-xs text-ink-tertiary">{{ t('userSubscriptions.unlimitedDesc') }}</dd>
            </div>
          </dl>
        </Surface>
      </div>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'

import subscriptionsAPI from '@/api/subscriptions'
import Badge from '@/components/common/Badge.vue'
import Button from '@/components/common/Button.vue'
import Meter from '@/components/common/Meter.vue'
import NumCell from '@/components/common/NumCell.vue'
import StatusDot from '@/components/common/StatusDot.vue'
import Surface from '@/components/common/Surface.vue'
import type { Tone } from '@/components/common/primitives'
import Icon from '@/components/icons/Icon.vue'
import AppLayout from '@/components/layout/AppLayout.vue'
import { useAppStore } from '@/stores/app'
import type { UserSubscription } from '@/types'
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

/**
 * Subscription state.
 *
 * `accent` never appears here — it means interactive or selected, and the Renew
 * button on this same row is the thing that owns it. A subscription that has
 * simply run its course is neutral and muted rather than red: an expiry is not
 * a failure, and spending danger on it leaves nothing for a revocation.
 */
const STATUS_TONE: Record<UserSubscription['status'], Tone> = {
  active: 'success',
  expired: 'neutral',
  revoked: 'danger',
  suspended: 'warn',
}

/** `suspended` has no translation; its raw value beats printing a key path. */
const TRANSLATED_STATUSES = new Set(['active', 'expired', 'revoked'])

function statusTone(status: UserSubscription['status']): Tone {
  return STATUS_TONE[status] ?? 'neutral'
}

function statusLabel(status: UserSubscription['status']): string {
  return TRANSLATED_STATUSES.has(status) ? t(`userSubscriptions.status.${status}`) : String(status)
}

interface UsageWindow {
  key: string
  label: string
  used: number
  limit: number
  caption: string
}

function hasNoLimits(subscription: UserSubscription): boolean {
  const group = subscription.group
  return !group?.daily_limit_usd && !group?.weekly_limit_usd && !group?.monthly_limit_usd
}

/** Only windows the group actually caps. A meter with no max measures nothing. */
function usageWindows(subscription: UserSubscription): UsageWindow[] {
  const group = subscription.group
  if (!group) return []

  const windows: UsageWindow[] = []

  if (group.daily_limit_usd) {
    windows.push({
      key: 'daily',
      label: t('userSubscriptions.daily'),
      used: subscription.daily_usage_usd || 0,
      limit: group.daily_limit_usd,
      caption: subscription.daily_window_start ? formatDailyUsageWindow(subscription) : '',
    })
  }
  if (group.weekly_limit_usd) {
    windows.push({
      key: 'weekly',
      label: t('userSubscriptions.weekly'),
      used: subscription.weekly_usage_usd || 0,
      limit: group.weekly_limit_usd,
      caption: subscription.weekly_window_start
        ? t('userSubscriptions.resetIn', {
            time: formatResetTime(subscription.weekly_window_start, 168)
          })
        : '',
    })
  }
  if (group.monthly_limit_usd) {
    windows.push({
      key: 'monthly',
      label: t('userSubscriptions.monthly'),
      used: subscription.monthly_usage_usd || 0,
      limit: group.monthly_limit_usd,
      caption: subscription.monthly_window_start
        ? t('userSubscriptions.resetIn', {
            time: formatResetTime(subscription.monthly_window_start, 720)
          })
        : '',
    })
  }

  return windows
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

/** Semantic colour only once a threshold is crossed; a healthy date is ink. */
function expirationToneClass(expiresAt: string): string {
  const diff = new Date(expiresAt).getTime() - Date.now()
  const days = Math.ceil(diff / (1000 * 60 * 60 * 24))

  if (diff <= 0) return 'font-medium text-danger'
  if (days <= 3) return 'text-danger'
  if (days <= 7) return 'text-warn'
  return 'text-ink'
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
