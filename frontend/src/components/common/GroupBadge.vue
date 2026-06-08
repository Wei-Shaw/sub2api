<template>
  <span
    :class="[
      'inline-flex items-center gap-1.5 rounded-md px-2 py-0.5 text-xs font-medium transition-colors',
      badgeClass
    ]"
  >
    <!-- Platform logo -->
    <PlatformIcon v-if="platform" :platform="platform" size="sm" />
    <!-- Group name -->
    <span class="truncate">{{ name }}</span>
    <!-- Right side label -->
    <span v-if="showLabel" :class="labelClass">
      <template v-if="hasCustomRate">
        <!-- original rate strikethrough + custom rate highlight -->
        <span class="line-through opacity-50 mr-0.5">{{ rateMultiplier }}x</span>
        <span class="font-bold">{{ userRateMultiplier }}x</span>
      </template>
      <template v-else>
        {{ labelText }}
      </template>
    </span>
  </span>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import type { SubscriptionType, GroupPlatform } from '@/types'
import { PlatformIcon } from '@sub2api/plugin-sdk'
import { usePlatforms } from '@/composables/usePlatforms'
import {
  dynamicGroupSubBadgeClass,
  dynamicGroupStdBadgeClass,
  dynamicSubLabelClass,
} from '@/utils/platformColors'

interface Props {
  name: string
  platform?: GroupPlatform
  subscriptionType?: SubscriptionType
  rateMultiplier?: number
  userRateMultiplier?: number | null
  showRate?: boolean
  daysRemaining?: number | null
  /**
   * When enabled, subscription groups also show rate in the right-side label
   * (with subscription theme color), for contexts that care about cost rate
   * rather than expiration.
   */
  alwaysShowRate?: boolean
}

const props = withDefaults(defineProps<Props>(), {
  subscriptionType: 'standard',
  showRate: true,
  daysRemaining: null,
  userRateMultiplier: null,
  alwaysShowRate: false
})

const { t } = useI18n()
const { getPlatformDecl } = usePlatforms()

const isSubscription = computed(() => props.subscriptionType === 'subscription')

const hasCustomRate = computed(() => {
  return (
    props.userRateMultiplier !== null &&
    props.userRateMultiplier !== undefined &&
    props.rateMultiplier !== undefined &&
    props.userRateMultiplier !== props.rateMultiplier
  )
})

const showLabel = computed(() => {
  if (!props.showRate) return false
  if (isSubscription.value) return true
  return props.rateMultiplier !== undefined || hasCustomRate.value
})

const labelText = computed(() => {
  const rateLabel = props.rateMultiplier !== undefined ? `${props.rateMultiplier}x` : ''
  if (isSubscription.value && !props.alwaysShowRate) {
    if (props.daysRemaining !== null && props.daysRemaining !== undefined) {
      if (props.daysRemaining <= 0) {
        return t('admin.users.expired')
      }
      return t('admin.users.daysRemaining', { days: props.daysRemaining })
    }
    return t('groups.subscription')
  }
  return rateLabel
})

// -- Hardcoded fallbacks for builtin platforms ------------------------

const FALLBACK_SUB_BADGE: Record<string, string> = {
  anthropic: 'bg-orange-100 text-orange-700 dark:bg-orange-900/30 dark:text-orange-400',
  openai: 'bg-emerald-100 text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-400',
  gemini: 'bg-blue-100 text-blue-700 dark:bg-blue-900/30 dark:text-blue-400',
}

const FALLBACK_STD_BADGE: Record<string, string> = {
  anthropic: 'bg-amber-50 text-amber-700 dark:bg-amber-900/20 dark:text-amber-400',
  openai: 'bg-green-50 text-green-700 dark:bg-green-900/20 dark:text-green-400',
  gemini: 'bg-sky-50 text-sky-700 dark:bg-sky-900/20 dark:text-sky-400',
}

const FALLBACK_SUB_LABEL: Record<string, string> = {
  anthropic: 'bg-orange-200/60 text-orange-800 dark:bg-orange-800/40 dark:text-orange-300',
  openai: 'bg-emerald-200/60 text-emerald-800 dark:bg-emerald-800/40 dark:text-emerald-300',
  gemini: 'bg-blue-200/60 text-blue-800 dark:bg-blue-800/40 dark:text-blue-300',
}

const DEFAULT_SUB_BADGE = 'bg-violet-100 text-violet-700 dark:bg-violet-900/30 dark:text-violet-400'
const DEFAULT_STD_BADGE = 'bg-emerald-100 text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-400'
const DEFAULT_SUB_LABEL = 'bg-violet-200/60 text-violet-800 dark:bg-violet-800/40 dark:text-violet-300'

// -- Label style based on type and days remaining ---------------------

const labelClass = computed(() => {
  const base = 'px-1.5 py-0.5 rounded text-[10px] font-semibold'

  if (!isSubscription.value) {
    return `${base} bg-black/10 dark:bg-white/10`
  }

  // Subscription: urgent/warning colors by days remaining
  if (props.daysRemaining !== null && props.daysRemaining !== undefined) {
    if (props.daysRemaining <= 0 || props.daysRemaining <= 3) {
      return `${base} bg-red-200/80 text-red-800 dark:bg-red-800/50 dark:text-red-300`
    }
    if (props.daysRemaining <= 7) {
      return `${base} bg-amber-200/80 text-amber-800 dark:bg-amber-800/50 dark:text-amber-300`
    }
  }

  // Normal subscription: use plugin theme_color or fallback
  const decl = props.platform ? getPlatformDecl(props.platform) : undefined
  if (decl?.theme_color) {
    return `${base} ${dynamicSubLabelClass(decl.theme_color)}`
  }
  const fb = props.platform ? FALLBACK_SUB_LABEL[props.platform] : undefined
  return `${base} ${fb ?? DEFAULT_SUB_LABEL}`
})

// -- Badge color based on platform and subscription type --------------

const badgeClass = computed(() => {
  const decl = props.platform ? getPlatformDecl(props.platform) : undefined

  if (decl?.theme_color) {
    return isSubscription.value
      ? dynamicGroupSubBadgeClass(decl.theme_color)
      : dynamicGroupStdBadgeClass(decl.theme_color)
  }

  // Fallback for builtin platforms
  if (props.platform) {
    if (isSubscription.value) {
      return FALLBACK_SUB_BADGE[props.platform] ?? DEFAULT_SUB_BADGE
    }
    return FALLBACK_STD_BADGE[props.platform] ?? DEFAULT_STD_BADGE
  }

  return isSubscription.value ? DEFAULT_SUB_BADGE : DEFAULT_STD_BADGE
})
</script>