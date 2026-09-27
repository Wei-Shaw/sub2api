<template>
  <div v-if="subscriptionFeatureEnabled && hasActiveSubscriptions" class="relative" ref="containerRef">
    <!-- Mini Progress Display -->
    <button
      @click="toggleTooltip"
      class="flex cursor-pointer items-center gap-2 rounded-sm bg-surface-sunken px-3 py-1.5 transition-colors hover:bg-surface-sunken"
      :title="t('subscriptionProgress.viewDetails')"
      :aria-expanded="tooltipOpen"
    >
      <Icon name="creditCard" size="sm" class="text-fg-muted" />
      <div class="flex items-center gap-1.5">
        <!-- Combined progress indicator -->
        <div class="flex items-center gap-0.5">
          <div
            v-for="(sub, index) in displaySubscriptions.slice(0, 3)"
            :key="index"
            class="h-2 w-2 rounded-full"
            :class="getProgressDotClass(sub)"
          ></div>
        </div>
        <span class="text-xs font-medium text-fg">
          {{ activeSubscriptions.length }}
        </span>
      </div>
    </button>

    <!-- Hover/Click Tooltip -->
    <transition name="dropdown">
      <div
        v-if="tooltipOpen"
        class="fixed inset-x-2 top-16 z-50 overflow-hidden rounded-sm border border-border bg-surface shadow-overlay sm:absolute sm:inset-x-auto sm:right-0 sm:top-auto sm:mt-2 sm:w-[340px]"
      >
        <div class="border-b border-border p-3">
          <h3 class="text-sm font-semibold text-fg">
            {{ t('subscriptionProgress.title') }}
          </h3>
          <p class="mt-0.5 text-xs text-fg-muted">
            {{ t('subscriptionProgress.activeCount', { count: activeSubscriptions.length }) }}
          </p>
        </div>

        <div class="max-h-64 overflow-y-auto">
          <div
            v-for="subscription in displaySubscriptions"
            :key="subscription.id"
            class="border-b border-border p-3 last:border-b-0"
          >
            <div class="mb-2 flex items-center justify-between">
              <span class="text-sm font-medium text-fg">
                {{ subscription.group?.name || `Group #${subscription.group_id}` }}
              </span>
              <span
                v-if="subscription.expires_at"
                class="text-xs"
                :class="getDaysRemainingClass(subscription.expires_at)"
              >
                {{ formatDaysRemaining(subscription.expires_at) }}
              </span>
            </div>

            <!-- Progress bars or Unlimited badge -->
            <div class="space-y-1.5">
              <!-- Unlimited subscription badge -->
              <div
                v-if="isUnlimited(subscription)"
                class="flex items-center gap-2 rounded-sm bg-success-weak px-2.5 py-1.5"
              >
                <span class="text-lg text-success">∞</span>
                <span class="text-xs font-medium text-success-strong">
                  {{ t('subscriptionProgress.unlimited') }}
                </span>
              </div>

              <!-- Progress bars for limited subscriptions -->
              <template v-else>
                <div v-if="subscription.group?.daily_limit_usd" class="flex items-center gap-2">
                  <span class="w-8 flex-shrink-0 text-[10px] text-fg-muted">{{
                    t('subscriptionProgress.daily')
                  }}</span>
                  <div class="h-1.5 min-w-0 flex-1 rounded-full bg-border">
                    <div
                      class="h-1.5 rounded-full transition-all"
                      :class="
                        getProgressBarClass(
                          subscription.daily_usage_usd,
                          subscription.group?.daily_limit_usd
                        )
                      "
                      :style="{
                        width: getProgressWidth(
                          subscription.daily_usage_usd,
                          subscription.group?.daily_limit_usd
                        )
                      }"
                    ></div>
                  </div>
                  <span class="w-24 flex-shrink-0 text-right text-[10px] text-fg-muted">
                    {{
                      formatUsage(subscription.daily_usage_usd, subscription.group?.daily_limit_usd)
                    }}
                  </span>
                </div>

                <div v-if="subscription.group?.weekly_limit_usd" class="flex items-center gap-2">
                  <span class="w-8 flex-shrink-0 text-[10px] text-fg-muted">{{
                    t('subscriptionProgress.weekly')
                  }}</span>
                  <div class="h-1.5 min-w-0 flex-1 rounded-full bg-border">
                    <div
                      class="h-1.5 rounded-full transition-all"
                      :class="
                        getProgressBarClass(
                          subscription.weekly_usage_usd,
                          subscription.group?.weekly_limit_usd
                        )
                      "
                      :style="{
                        width: getProgressWidth(
                          subscription.weekly_usage_usd,
                          subscription.group?.weekly_limit_usd
                        )
                      }"
                    ></div>
                  </div>
                  <span class="w-24 flex-shrink-0 text-right text-[10px] text-fg-muted">
                    {{
                      formatUsage(subscription.weekly_usage_usd, subscription.group?.weekly_limit_usd)
                    }}
                  </span>
                </div>

                <div v-if="subscription.group?.monthly_limit_usd" class="flex items-center gap-2">
                  <span class="w-8 flex-shrink-0 text-[10px] text-fg-muted">{{
                    t('subscriptionProgress.monthly')
                  }}</span>
                  <div class="h-1.5 min-w-0 flex-1 rounded-full bg-border">
                    <div
                      class="h-1.5 rounded-full transition-all"
                      :class="
                        getProgressBarClass(
                          subscription.monthly_usage_usd,
                          subscription.group?.monthly_limit_usd
                        )
                      "
                      :style="{
                        width: getProgressWidth(
                          subscription.monthly_usage_usd,
                          subscription.group?.monthly_limit_usd
                        )
                      }"
                    ></div>
                  </div>
                  <span class="w-24 flex-shrink-0 text-right text-[10px] text-fg-muted">
                    {{
                      formatUsage(
                        subscription.monthly_usage_usd,
                        subscription.group?.monthly_limit_usd
                      )
                    }}
                  </span>
                </div>
              </template>
            </div>
          </div>
        </div>

        <div class="border-t border-border p-2">
          <router-link
            to="/subscriptions"
            @click="closeTooltip"
            class="block w-full py-1 text-center text-xs text-accent hover:underline"
          >
            {{ t('subscriptionProgress.viewAll') }}
          </router-link>
        </div>
      </div>
    </transition>
  </div>
</template>

<script setup lang="ts">
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
    return 'bg-success'
  }
  const maxPercentage = getMaxUsagePercentage(sub)
  if (maxPercentage >= 90) return 'bg-danger'
  if (maxPercentage >= 70) return 'bg-warning'
  return 'bg-success'
}

function getProgressBarClass(used: number | undefined, limit: number | null | undefined): string {
  if (!limit || limit === 0) return 'bg-gray-400'
  const percentage = ((used || 0) / limit) * 100
  if (percentage >= 90) return 'bg-danger'
  if (percentage >= 70) return 'bg-warning'
  return 'bg-success'
}

function getProgressWidth(used: number | undefined, limit: number | null | undefined): string {
  if (!limit || limit === 0) return '0%'
  const percentage = Math.min(((used || 0) / limit) * 100, 100)
  return `${percentage}%`
}

function formatUsage(used: number | undefined, limit: number | null | undefined): string {
  const usedValue = (used || 0).toFixed(2)
  const limitValue = limit?.toFixed(2) || '∞'
  return `$${usedValue}/$${limitValue}`
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
  if (days <= 3) return 'text-danger'
  if (days <= 7) return 'text-warning'
  return 'text-fg-muted'
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
.dropdown-enter-active,
.dropdown-leave-active {
  transition: all 0.2s ease;
}

.dropdown-enter-from,
.dropdown-leave-to {
  opacity: 0;
  transform: scale(0.95) translateY(-4px);
}
</style>
