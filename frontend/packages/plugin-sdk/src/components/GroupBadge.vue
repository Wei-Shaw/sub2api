<template>
  <span
    :class="[
      'inline-flex items-center gap-1.5 rounded-md px-2 py-0.5 text-xs font-medium transition-colors',
      badgeClass,
    ]"
  >
    <PlatformIcon v-if="platform" :platform="platform" size="sm" />
    <span class="truncate">{{ name }}</span>
    <span v-if="showLabel" :class="labelClass">
      <template v-if="hasCustomRate">
        <span class="line-through opacity-50 mr-0.5">{{ rateMultiplier }}x</span>
        <span class="font-bold">{{ userRateMultiplier }}x</span>
      </template>
      <template v-else>{{ labelText }}</template>
    </span>
  </span>
</template>

<script setup lang="ts">
/**
 * Unified GroupBadge — merged from plugins/payment + plugins/channel-management.
 *
 * Renders a group chip with optional platform-colored badge background, custom
 * rate-multiplier highlight (line-through old, bold new), and an optional
 * subscription-mode right label that turns red/amber as the remaining days
 * drop. Used by every plugin that shows group chips (payment plans,
 * available-channels, plan editor, etc.).
 *
 * i18n keys 'admin.users.expired', 'admin.users.daysRemaining',
 * 'groups.subscription' are registered by the host namespace and reachable
 * because plugin Vue apps share the host vue-i18n instance via importmap.
 */
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import PlatformIcon from './PlatformIcon.vue'

interface Props {
  name: string
  platform?: string
  subscriptionType?: string
  rateMultiplier?: number
  userRateMultiplier?: number | null
  showRate?: boolean
  daysRemaining?: number | null
  /**
   * Force showing rate even when subscription_type === 'subscription'.
   * Used by available-channels view where only the rate matters.
   */
  alwaysShowRate?: boolean
}

const props = withDefaults(defineProps<Props>(), {
  platform: '',
  subscriptionType: 'standard',
  showRate: true,
  daysRemaining: null,
  userRateMultiplier: null,
  alwaysShowRate: false,
})

const { t } = useI18n()

const isSubscription = computed(() => props.subscriptionType === 'subscription')

const hasCustomRate = computed(
  () =>
    props.userRateMultiplier !== null &&
    props.userRateMultiplier !== undefined &&
    props.rateMultiplier !== undefined &&
    props.userRateMultiplier !== props.rateMultiplier,
)

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

const labelClass = computed(() => {
  const base = 'px-1.5 py-0.5 rounded text-[10px] font-semibold'

  if (!isSubscription.value) {
    return `${base} bg-black/10 dark:bg-white/10`
  }

  if (props.daysRemaining !== null && props.daysRemaining !== undefined) {
    if (props.daysRemaining <= 0 || props.daysRemaining <= 3) {
      return `${base} bg-red-200/80 text-red-800 dark:bg-red-800/50 dark:text-red-300`
    }
    if (props.daysRemaining <= 7) {
      return `${base} bg-amber-200/80 text-amber-800 dark:bg-amber-800/50 dark:text-amber-300`
    }
  }

  if (props.platform === 'anthropic') {
    return `${base} bg-orange-200/60 text-orange-800 dark:bg-orange-800/40 dark:text-orange-300`
  }
  if (props.platform === 'openai') {
    return `${base} bg-emerald-200/60 text-emerald-800 dark:bg-emerald-800/40 dark:text-emerald-300`
  }
  if (props.platform === 'gemini') {
    return `${base} bg-blue-200/60 text-blue-800 dark:bg-blue-800/40 dark:text-blue-300`
  }
  return `${base} bg-violet-200/60 text-violet-800 dark:bg-violet-800/40 dark:text-violet-300`
})

const badgeClass = computed(() => {
  if (props.platform === 'anthropic') {
    return isSubscription.value
      ? 'bg-orange-100 text-orange-700 dark:bg-orange-900/30 dark:text-orange-400'
      : 'bg-amber-50 text-amber-700 dark:bg-amber-900/20 dark:text-amber-400'
  }
  if (props.platform === 'openai') {
    return isSubscription.value
      ? 'bg-emerald-100 text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-400'
      : 'bg-green-50 text-green-700 dark:bg-green-900/20 dark:text-green-400'
  }
  if (props.platform === 'gemini') {
    return isSubscription.value
      ? 'bg-blue-100 text-blue-700 dark:bg-blue-900/30 dark:text-blue-400'
      : 'bg-sky-50 text-sky-700 dark:bg-sky-900/20 dark:text-sky-400'
  }
  return isSubscription.value
    ? 'bg-violet-100 text-violet-700 dark:bg-violet-900/30 dark:text-violet-400'
    : 'bg-emerald-100 text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-400'
})
</script>
