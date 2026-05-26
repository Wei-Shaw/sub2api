<!-- SubscriptionList: plan grid + active subscriptions display. -->
<template>
  <!-- Plan list -->
  <div v-if="plans.length === 0" class="card py-16 text-center">
    <Icon name="gift" size="xl" class="mx-auto mb-3 text-gray-300 dark:text-dark-600" />
    <p class="text-gray-500 dark:text-gray-400">{{ t('payment.noPlans') }}</p>
  </div>
  <div v-else :class="gridClass">
    <SubscriptionPlanCard v-for="plan in plans" :key="plan.id" :plan="plan" :active-subscriptions="activeSubscriptions" @select="$emit('select', $event)" />
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
            <span>{{ t('payment.planCard.rate') }}: &times;{{ sub.group?.rate_multiplier ?? 1 }}</span>
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

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { platformAccentBarClass, platformBadgeLightClass, platformLabel } from '@sub2api/plugin-sdk'
import SubscriptionPlanCard from './SubscriptionPlanCard.vue'
import { Icon } from '@sub2api/plugin-sdk'
import type { SubscriptionPlan } from '../../types/payment'

const { t } = useI18n()

interface ActiveSubscription {
  id: number
  group_id: number
  expires_at?: string
  group?: {
    name?: string
    platform?: string
    rate_multiplier?: number
    daily_limit_usd?: number | null
    weekly_limit_usd?: number | null
    monthly_limit_usd?: number | null
  }
}

defineProps<{
  plans: SubscriptionPlan[]
  activeSubscriptions: ActiveSubscription[]
  gridClass: string
}>()

defineEmits<{
  select: [plan: SubscriptionPlan]
}>()

function getDaysRemaining(expiresAt: string): number {
  const diff = new Date(expiresAt).getTime() - Date.now()
  return Math.max(0, Math.ceil(diff / (1000 * 60 * 60 * 24)))
}
</script>