<template>
  <span
    :class="[
      'inline-flex items-center gap-1.5 px-2 py-0.5 rounded-md text-xs font-medium transition-colors',
      isSubscription
        ? 'bg-violet-100 text-violet-700 dark:bg-violet-900/30 dark:text-violet-400'
        : 'bg-emerald-100 text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-400'
    ]"
  >
    <!-- Subscription type icon (calendar) -->
    <svg v-if="isSubscription" class="w-3 h-3" fill="none" stroke="currentColor" viewBox="0 0 24 24" stroke-width="2">
      <path stroke-linecap="round" stroke-linejoin="round" d="M6.75 3v2.25M17.25 3v2.25M3 18.75V7.5a2.25 2.25 0 012.25-2.25h13.5A2.25 2.25 0 0121 7.5v11.25m-18 0A2.25 2.25 0 005.25 21h13.5A2.25 2.25 0 0021 18.75m-18 0v-7.5A2.25 2.25 0 015.25 9h13.5A2.25 2.25 0 0121 11.25v7.5" />
    </svg>
    <!-- Standard type icon (wallet) -->
    <svg v-else class="w-3 h-3" fill="none" stroke="currentColor" viewBox="0 0 24 24" stroke-width="2">
      <path stroke-linecap="round" stroke-linejoin="round" d="M21 12a2.25 2.25 0 00-2.25-2.25H15a3 3 0 11-6 0H5.25A2.25 2.25 0 003 12m18 0v6a2.25 2.25 0 01-2.25 2.25H5.25A2.25 2.25 0 013 18v-6m18 0V9M3 12V9m18 0a2.25 2.25 0 00-2.25-2.25H5.25A2.25 2.25 0 003 9m18 0V6a2.25 2.25 0 00-2.25-2.25H5.25A2.25 2.25 0 003 6v3" />
    </svg>
    <span class="truncate">{{ name REDACTEDREDACTED</span>
    <span
      v-if="showRate && rateMultiplier !== undefined"
      :class="[
        'px-1 py-0.5 rounded text-[10px] font-semibold',
        isSubscription
          ? 'bg-violet-200/60 text-violet-800 dark:bg-violet-800/40 dark:text-violet-300'
          : 'bg-amber-100 text-amber-700 dark:bg-amber-900/30 dark:text-amber-400'
      ]"
    >
      {{ rateMultiplier REDACTEDREDACTEDx
    </span>
  </span>
</template>

<script setup lang="ts">
import { computed REDACTED from 'vue'
import type { SubscriptionType REDACTED from '@/types'

interface Props {
  name: string
  subscriptionType?: SubscriptionType
  rateMultiplier?: number
  showRate?: boolean
REDACTED

const props = withDefaults(defineProps<Props>(), {
  subscriptionType: 'standard',
  showRate: true
REDACTED)

const isSubscription = computed(() => props.subscriptionType === 'subscription')
</script>
