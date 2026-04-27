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
 * V5 W9 — 渠道分组徽章. 简化版 (vs host frontend/src/components/common/GroupBadge.vue):
 *   - 删除 daysRemaining 分支 (Available Channels 仅在 alwaysShowRate=true
 *     的语境下使用, 不需要剩余天数样式)
 *   - 删除复杂的过期警示色 (red / amber)
 *   - 仅保留按 platform 着色 + 专属倍率高亮 (line-through old, bold new)
 *
 * 这样 plugin bundle 不引入 admin.users.expired / admin.users.daysRemaining
 * 等 host i18n key.
 */
import { computed } from 'vue'
import { PlatformIcon } from '@sub2api/plugin-sdk'

interface Props {
  name: string
  platform?: string
  subscriptionType?: string
  rateMultiplier?: number
  userRateMultiplier?: number | null
  alwaysShowRate?: boolean
}

const props = withDefaults(defineProps<Props>(), {
  platform: '',
  subscriptionType: 'standard',
  userRateMultiplier: null,
  alwaysShowRate: false,
})

const isSubscription = computed(() => props.subscriptionType === 'subscription')

const hasCustomRate = computed(
  () =>
    props.userRateMultiplier !== null &&
    props.userRateMultiplier !== undefined &&
    props.rateMultiplier !== undefined &&
    props.userRateMultiplier !== props.rateMultiplier,
)

const showLabel = computed(() => props.rateMultiplier !== undefined || hasCustomRate.value)

const labelText = computed(() =>
  props.rateMultiplier !== undefined ? `${props.rateMultiplier}x` : '',
)

const labelClass = computed(() => {
  const base = 'px-1.5 py-0.5 rounded text-[10px] font-semibold'
  return `${base} bg-black/10 dark:bg-white/10`
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
