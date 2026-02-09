<template>
  <div class="flex min-w-0 flex-1 items-center justify-between gap-2">
    <div
      class="flex min-w-0 flex-1 flex-col items-start gap-1"
      :title="description || undefined"
    >
      <GroupBadge
        :name="name"
        :platform="platform"
        :subscription-type="subscriptionType"
        :rate-multiplier="rateMultiplier"
        :user-rate-multiplier="userRateMultiplier"
      />
      <span
        v-if="description"
        class="w-full text-left text-xs text-gray-500 dark:text-gray-400 line-clamp-2"
      >
        {{ description REDACTEDREDACTED
      </span>
    </div>
    <svg
      v-if="showCheckmark && selected"
      class="h-4 w-4 shrink-0 text-primary-600 dark:text-primary-400"
      fill="none"
      stroke="currentColor"
      viewBox="0 0 24 24"
      stroke-width="2"
    >
      <path stroke-linecap="round" stroke-linejoin="round" d="M5 13l4 4L19 7" />
    </svg>
  </div>
</template>

<script setup lang="ts">
import GroupBadge from './GroupBadge.vue'
import type { SubscriptionType, GroupPlatform REDACTED from '@/types'

interface Props {
  name: string
  platform: GroupPlatform
  subscriptionType?: SubscriptionType
  rateMultiplier?: number
  userRateMultiplier?: number | null
  description?: string | null
  selected?: boolean
  showCheckmark?: boolean
REDACTED

withDefaults(defineProps<Props>(), {
  subscriptionType: 'standard',
  selected: false,
  showCheckmark: true,
  userRateMultiplier: null
REDACTED)
</script>
