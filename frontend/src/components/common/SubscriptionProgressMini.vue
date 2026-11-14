<template>
  <div v-if="subscriptionFeatureEnabled && hasActiveSubscriptions" ref="containerRef" class="subscription-widget relative">
    <button
      type="button"
      class="subscription-trigger"
      :class="{ 'is-open': tooltipOpen }"
      :title="t('subscriptionProgress.viewDetails')"
      :aria-expanded="tooltipOpen"
      @click="toggleTooltip"
    >
      <Icon name="creditCard" size="sm" class="subscription-trigger-icon" />
      <span class="hidden sm:inline">{{ t('subscriptionProgress.title') }}</span>
      <span class="subscription-count">{{ activeSubscriptions.length }}</span>
      <span class="subscription-signals" aria-hidden="true">
        <span v-for="sub in displaySubscriptions.slice(0, 3)" :key="sub.id" class="h-1 w-1 rounded-full" :class="getProgressDotClass(sub)"></span>
      </span>
      <Icon name="chevronDown" size="xs" class="subscription-chevron" :class="{ 'rotate-180': tooltipOpen }" />
    </button>

    <transition name="dropdown">
      <div v-if="tooltipOpen" class="subscription-popover">
        <div class="subscription-heading">
          <h3>{{ t('subscriptionProgress.title') }}</h3>
          <span>{{ t('subscriptionProgress.activeCount', { count: activeSubscriptions.length }) }}</span>
        </div>

        <div class="subscription-list">
          <section v-for="subscription in displaySubscriptions" :key="subscription.id" class="subscription-item">
            <div class="subscription-identity">
              <div class="min-w-0 flex-1">
                <div class="subscription-platform">
                  <span class="h-1.5 w-1.5 rounded-full" :class="getSubscriptionAccentClass(subscription)"></span>
                  {{ subscription.group?.platform || 'API' }}
                </div>
                <h4 :title="subscription.group?.name || `Group #${subscription.group_id}`">
                  {{ subscription.group?.name || `Group #${subscription.group_id}` }}
                </h4>
              </div>
              <span
                v-if="subscription.expires_at"
                class="subscription-expiry"
                :class="getDaysRemainingClass(subscription.expires_at)"
              >
                {{ formatDaysRemaining(subscription.expires_at) }}
              </span>
            </div>

            <div v-if="isUnlimited(subscription)" class="subscription-unlimited">
              <span class="text-lg leading-none">∞</span>
              {{ t('subscriptionProgress.unlimited') }}
            </div>
            <div v-else class="subscription-meters">
              <div v-if="subscription.group?.daily_limit_usd" class="subscription-meter">
                <span class="subscription-period">{{ t('subscriptionProgress.daily') }}</span>
                <span class="subscription-value">{{ formatUsage(subscription.daily_usage_usd, subscription.group?.daily_limit_usd, subscription.group) }}</span>
                <div class="subscription-track">
                  <div :class="getProgressBarClass(subscription.daily_usage_usd, subscription.group?.daily_limit_usd)" :style="{ width: getProgressWidth(subscription.daily_usage_usd, subscription.group?.daily_limit_usd) }"></div>
                </div>
              </div>
              <div v-if="subscription.group?.weekly_limit_usd" class="subscription-meter">
                <span class="subscription-period">{{ t('subscriptionProgress.weekly') }}</span>
                <span class="subscription-value">{{ formatUsage(subscription.weekly_usage_usd, subscription.group?.weekly_limit_usd, subscription.group) }}</span>
                <div class="subscription-track">
                  <div :class="getProgressBarClass(subscription.weekly_usage_usd, subscription.group?.weekly_limit_usd)" :style="{ width: getProgressWidth(subscription.weekly_usage_usd, subscription.group?.weekly_limit_usd) }"></div>
                </div>
              </div>
              <div v-if="subscription.group?.monthly_limit_usd" class="subscription-meter">
                <span class="subscription-period">{{ t('subscriptionProgress.monthly') }}</span>
                <span class="subscription-value">{{ formatUsage(subscription.monthly_usage_usd, subscription.group?.monthly_limit_usd, subscription.group) }}</span>
                <div class="subscription-track">
                  <div :class="getProgressBarClass(subscription.monthly_usage_usd, subscription.group?.monthly_limit_usd)" :style="{ width: getProgressWidth(subscription.monthly_usage_usd, subscription.group?.monthly_limit_usd) }"></div>
                </div>
              </div>
            </div>
          </section>
        </div>

        <router-link to="/subscriptions" class="subscription-footer" @click="closeTooltip">
          <span>{{ t('subscriptionProgress.viewAll') }}</span>
          <Icon name="arrowRight" size="sm" />
        </router-link>
      </div>
    </transition>
  </div>
</template>

<script setup lang="ts">
import { useQuotaUsageDisplay, type QuotaGroup } from '@/composables/useSubscriptionUnit'
const formatQuotaUsage = useQuotaUsageDisplay(2)
import { ref, computed, onMounted, onBeforeUnmount } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
import { useSubscriptionStore } from '@/stores'
import { FeatureFlags, isFeatureFlagEnabled } from '@/utils/featureFlags'
import type { UserSubscription } from '@/types'
import { getExpirationDateRelation } from '@/utils/subscriptionQuota'

const { t } = useI18n()

const subscriptionStore = useSubscriptionStore()

const containerRef = ref<HTMLElement | null>(null)
const tooltipOpen = ref(false)

// Use store data instead of local state
const activeSubscriptions = computed(() => subscriptionStore.activeSubscriptions)
const hasActiveSubscriptions = computed(() => subscriptionStore.hasActiveSubscriptions)
// 订阅功能关闭后，即使用户仍持有后台分配的订阅，顶栏也不再露出订阅进度与「查看全部订阅」入口。
const subscriptionFeatureEnabled = computed(() => isFeatureFlagEnabled(FeatureFlags.subscription))

const displaySubscriptions = computed(() => {
  // Sort by most usage (highest percentage first)
  return [...activeSubscriptions.value].sort((a, b) => {
    const aMax = getMaxUsagePercentage(a)
    const bMax = getMaxUsagePercentage(b)
    return bMax - aMax
  })
})

function getMaxUsagePercentage(sub: UserSubscription): number {
  const percentages: number[] = []
  if (sub.group?.daily_limit_usd) {
    percentages.push(((sub.daily_usage_usd || 0) / sub.group.daily_limit_usd) * 100)
  }
  if (sub.group?.weekly_limit_usd) {
    percentages.push(((sub.weekly_usage_usd || 0) / sub.group.weekly_limit_usd) * 100)
  }
  if (sub.group?.monthly_limit_usd) {
    percentages.push(((sub.monthly_usage_usd || 0) / sub.group.monthly_limit_usd) * 100)
  }
  return percentages.length > 0 ? Math.max(...percentages) : 0
}

function isUnlimited(sub: UserSubscription): boolean {
  return (
    !sub.group?.daily_limit_usd &&
    !sub.group?.weekly_limit_usd &&
    !sub.group?.monthly_limit_usd
  )
}

function getProgressDotClass(sub: UserSubscription): string {
  // Unlimited subscriptions get a special color
  if (isUnlimited(sub)) {
    return 'bg-emerald-500'
  }
  const maxPercentage = getMaxUsagePercentage(sub)
  if (maxPercentage >= 90) return 'bg-rose-500'
  if (maxPercentage >= 70) return 'bg-cyan-500'
  return 'bg-emerald-500'
}

function getProgressBarClass(used: number | undefined, limit: number | null | undefined): string {
  if (!limit || limit === 0) return 'bg-gray-400'
  const percentage = ((used || 0) / limit) * 100
  if (percentage >= 90) return 'bg-rose-500'
  if (percentage >= 70) return 'bg-cyan-500'
  return 'bg-emerald-500'
}

function getProgressWidth(used: number | undefined, limit: number | null | undefined): string {
  if (!limit || limit === 0) return '0%'
  const percentage = Math.min(((used || 0) / limit) * 100, 100)
  return `${percentage}%`
}

function getSubscriptionAccentClass(sub: UserSubscription): string {
  if (isUnlimited(sub)) return 'bg-emerald-500'
  const percentage = getMaxUsagePercentage(sub)
  if (percentage >= 90) return 'bg-rose-500'
  if (percentage >= 70) return 'bg-cyan-500'
  return 'bg-emerald-500'
}

function formatUsage(used: number | undefined, limit: number | null | undefined, group?: QuotaGroup): string {
  return formatQuotaUsage(used, limit, group)
}

function formatDaysRemaining(expiresAt: string): string {
  const now = new Date()
  const expires = new Date(expiresAt)
  const diff = expires.getTime() - now.getTime()
  const relation = getExpirationDateRelation(expires, now)
  if (relation === 'expired') return t('subscriptionProgress.expired')
  if (relation === 'today') return t('subscriptionProgress.expiresToday')
  if (relation === 'tomorrow') return t('subscriptionProgress.expiresTomorrow')
  const days = Math.ceil(diff / (1000 * 60 * 60 * 24))
  return t('subscriptionProgress.daysRemaining', { days })
}

function getDaysRemainingClass(expiresAt: string): string {
  const now = new Date()
  const expires = new Date(expiresAt)
  const diff = expires.getTime() - now.getTime()
  const days = Math.ceil(diff / (1000 * 60 * 60 * 24))
  if (days <= 3) return 'border-rose-200 bg-rose-50 text-rose-600 dark:border-rose-400/30 dark:bg-rose-400/10 dark:text-rose-300'
  if (days <= 7) return 'border-cyan-200 bg-cyan-50 text-cyan-700 dark:border-cyan-400/30 dark:bg-cyan-400/10 dark:text-cyan-300'
  return 'border-slate-200 bg-slate-50 text-slate-500 dark:border-slate-700 dark:bg-slate-800 dark:text-slate-400'
}

function toggleTooltip() {
  tooltipOpen.value = !tooltipOpen.value
}

function closeTooltip() {
  tooltipOpen.value = false
}

function handleClickOutside(event: MouseEvent) {
  if (containerRef.value && !containerRef.value.contains(event.target as Node)) {
    closeTooltip()
  }
}

onMounted(() => {
  document.addEventListener('click', handleClickOutside)
  // Trigger initial fetch if not already loaded
  // The actual data loading is handled by App.vue globally
  if (!subscriptionFeatureEnabled.value) return
  subscriptionStore.fetchActiveSubscriptions().catch((error) => {
    console.error('Failed to load subscriptions in SubscriptionProgressMini:', error)
  })
})

onBeforeUnmount(() => {
  document.removeEventListener('click', handleClickOutside)
})
</script>

<style scoped>
.subscription-trigger {
  @apply flex h-9 items-center gap-2 rounded-lg border border-transparent px-2.5 text-xs font-medium text-slate-600 transition-colors hover:border-slate-200 hover:bg-white dark:text-slate-300 dark:hover:border-slate-700 dark:hover:bg-slate-800;
}
.subscription-trigger.is-open {
  @apply border-slate-200 bg-white text-slate-900 dark:border-slate-700 dark:bg-slate-800 dark:text-white;
}
.subscription-trigger-icon {
  @apply text-teal-600 dark:text-teal-400;
}
.subscription-count {
  @apply flex h-5 min-w-5 items-center justify-center rounded-md bg-slate-100 px-1 text-[11px] font-semibold tabular-nums text-slate-600 dark:bg-slate-700/60 dark:text-slate-200;
}
.subscription-signals {
  @apply flex items-center gap-1 border-l border-slate-200 pl-2 dark:border-slate-700;
}
.subscription-chevron {
  @apply text-slate-400 transition-transform;
}
.subscription-popover {
  @apply absolute right-0 z-50 mt-3 overflow-hidden rounded-xl border border-slate-200 bg-white dark:border-slate-700 dark:bg-slate-900;
  width: min(368px, calc(100vw - 1.5rem));
  box-shadow: 0 16px 48px -12px rgb(0 0 0 / 30%), 0 2px 8px rgb(0 0 0 / 5%);
  transform-origin: top right;
}
.subscription-heading {
  @apply flex items-center justify-between gap-3 border-b border-slate-100 px-4 py-3 dark:border-slate-800;
}
.subscription-heading h3 {
  @apply text-sm font-semibold text-slate-900 dark:text-white;
}
.subscription-heading > span {
  @apply text-[11px] text-slate-500 dark:text-slate-400;
}
.subscription-list {
  --subscription-scrollbar-thumb: #cbd5e1;
  --subscription-scrollbar-thumb-hover: #94a3b8;
  max-height: min(480px, calc(100dvh - 12rem));
  overflow-y: auto;
  overscroll-behavior: contain;
  scrollbar-gutter: stable;
  /* Chromium 需要 auto，才能使用下方的圆角滚动条样式。 */
  scrollbar-width: auto;
  scrollbar-color: auto;
}
.dark .subscription-list {
  --subscription-scrollbar-thumb: #475569;
  --subscription-scrollbar-thumb-hover: #64748b;
}
.subscription-list::-webkit-scrollbar {
  width: 8px;
}
.subscription-list::-webkit-scrollbar-track {
  margin-block: 8px;
  background: transparent;
}
.subscription-list::-webkit-scrollbar-thumb,
.subscription-list:hover::-webkit-scrollbar-thumb,
.subscription-list:focus-within::-webkit-scrollbar-thumb {
  border: 2px solid transparent;
  border-radius: 999px;
  background-color: var(--subscription-scrollbar-thumb);
  background-clip: padding-box;
}
.subscription-list:hover::-webkit-scrollbar-thumb:hover {
  background-color: var(--subscription-scrollbar-thumb-hover);
}
.subscription-list::-webkit-scrollbar-button {
  display: none;
  width: 0;
  height: 0;
}
@supports (-moz-appearance: none) {
  .subscription-list {
    scrollbar-width: thin;
    scrollbar-color: var(--subscription-scrollbar-thumb) transparent;
  }
  .subscription-list:hover,
  .subscription-list:focus-within {
    scrollbar-color: var(--subscription-scrollbar-thumb-hover) transparent;
  }
}
.subscription-item {
  @apply border-b border-slate-100 px-4 py-4 last:border-b-0 dark:border-slate-800;
}
.subscription-identity {
  @apply mb-3 flex items-start justify-between gap-3;
}
.subscription-platform {
  @apply mb-1 flex items-center gap-1.5 text-[10px] font-medium uppercase tracking-wider text-slate-500 dark:text-slate-400;
}
.subscription-identity h4 {
  @apply text-sm font-semibold leading-5 text-slate-900 dark:text-slate-100;
  overflow-wrap: anywhere;
}
.subscription-expiry {
  @apply mt-0.5 shrink-0 rounded-md border px-2 py-1 text-[10px] font-medium leading-4;
}
.subscription-meters {
  @apply space-y-2.5;
}
.subscription-meter {
  display: grid;
  grid-template-columns: 32px minmax(0, 1fr);
  column-gap: 12px;
  row-gap: 4px;
  align-items: center;
}
.subscription-period {
  @apply text-[11px] leading-4 text-slate-500 dark:text-slate-400;
  grid-row: span 2;
}
.subscription-value {
  @apply text-right text-xs font-medium leading-4 tabular-nums text-slate-700 dark:text-slate-200;
  min-width: 0;
  white-space: normal;
  overflow-wrap: anywhere;
}
.subscription-track {
  @apply h-[3px] overflow-hidden rounded-full bg-slate-100 dark:bg-slate-800;
  grid-column: 2;
}
.subscription-track > div {
  @apply h-full rounded-full transition-all;
}
.subscription-unlimited {
  @apply flex items-center gap-2 text-xs text-emerald-600 dark:text-emerald-400;
}
.subscription-footer {
  @apply flex items-center justify-between border-t border-slate-100 bg-slate-50/80 px-4 py-3 text-xs font-medium text-teal-700 transition-colors hover:bg-teal-50 dark:border-slate-800 dark:bg-slate-800/40 dark:text-teal-300 dark:hover:bg-teal-400/10;
}
.dropdown-enter-active,
.dropdown-leave-active {
  transition: opacity 0.15s ease, transform 0.15s ease;
}
.dropdown-enter-from,
.dropdown-leave-to {
  opacity: 0;
  transform: translateY(-6px);
}
@media (prefers-reduced-motion: reduce) {
  .dropdown-enter-active,
  .dropdown-leave-active {
    transition: none;
  }
}
</style>
