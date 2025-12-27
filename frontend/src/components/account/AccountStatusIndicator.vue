<template>
  <div class="flex items-center gap-2">
    <!-- Main Status Badge -->
    <span :class="['badge text-xs', statusClass]">
      {{ statusText }}
    </span>

    <!-- Error Info Indicator -->
    <div v-if="hasError && account.error_message" class="group/error relative">
      <svg
        class="h-4 w-4 cursor-help text-red-500 transition-colors hover:text-red-600 dark:text-red-400 dark:hover:text-red-300"
        fill="none"
        viewBox="0 0 24 24"
        stroke="currentColor"
        stroke-width="2"
      >
        <path
          stroke-linecap="round"
          stroke-linejoin="round"
          d="M9.879 7.519c1.171-1.025 3.071-1.025 4.242 0 1.172 1.025 1.172 2.687 0 3.712-.203.179-.43.326-.67.442-.745.361-1.45.999-1.45 1.827v.75M21 12a9 9 0 11-18 0 9 9 0 0118 0zm-9 5.25h.008v.008H12v-.008z"
        />
      </svg>
      <!-- Tooltip - 向下显示 -->
      <div
        class="invisible absolute left-0 top-full z-[100] mt-1.5 min-w-[200px] max-w-[300px] rounded-lg bg-gray-800 px-3 py-2 text-xs text-white opacity-0 shadow-xl transition-all duration-200 group-hover/error:visible group-hover/error:opacity-100 dark:bg-gray-900"
      >
        <div class="whitespace-pre-wrap break-words leading-relaxed text-gray-300">
          {{ account.error_message }}
        </div>
        <!-- 上方小三角 -->
        <div
          class="absolute bottom-full left-3 border-[6px] border-transparent border-b-gray-800 dark:border-b-gray-900"
        ></div>
      </div>
    </div>

    <!-- Rate Limit Indicator (429) -->
    <div v-if="isRateLimited" class="group relative">
      <span
        class="inline-flex items-center gap-1 rounded bg-amber-100 px-1.5 py-0.5 text-xs font-medium text-amber-700 dark:bg-amber-900/30 dark:text-amber-400"
      >
        <svg class="h-3 w-3" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
          <path
            stroke-linecap="round"
            stroke-linejoin="round"
            d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-3L13.732 4c-.77-1.333-2.694-1.333-3.464 0L3.34 16c-.77 1.333.192 3 1.732 3z"
          />
        </svg>
        429
      </span>
      <!-- Tooltip -->
      <div
        class="pointer-events-none absolute bottom-full left-1/2 z-50 mb-2 -translate-x-1/2 whitespace-nowrap rounded bg-gray-900 px-2 py-1 text-xs text-white opacity-0 transition-opacity group-hover:opacity-100 dark:bg-gray-700"
      >
        Rate limited until {{ formatTime(account.rate_limit_reset_at) }}
        <div
          class="absolute left-1/2 top-full -translate-x-1/2 border-4 border-transparent border-t-gray-900 dark:border-t-gray-700"
        ></div>
      </div>
    </div>

    <!-- Overload Indicator (529) -->
    <div v-if="isOverloaded" class="group relative">
      <span
        class="inline-flex items-center gap-1 rounded bg-red-100 px-1.5 py-0.5 text-xs font-medium text-red-700 dark:bg-red-900/30 dark:text-red-400"
      >
        <svg class="h-3 w-3" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
          <path
            stroke-linecap="round"
            stroke-linejoin="round"
            d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-3L13.732 4c-.77-1.333-2.694-1.333-3.464 0L3.34 16c-.77 1.333.192 3 1.732 3z"
          />
        </svg>
        529
      </span>
      <!-- Tooltip -->
      <div
        class="pointer-events-none absolute bottom-full left-1/2 z-50 mb-2 -translate-x-1/2 whitespace-nowrap rounded bg-gray-900 px-2 py-1 text-xs text-white opacity-0 transition-opacity group-hover:opacity-100 dark:bg-gray-700"
      >
        Overloaded until {{ formatTime(account.overload_until) }}
        <div
          class="absolute left-1/2 top-full -translate-x-1/2 border-4 border-transparent border-t-gray-900 dark:border-t-gray-700"
        ></div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import type { Account } from '@/types'
import { formatTime } from '@/utils/format'

const props = defineProps<{
  account: Account
}>()

// Computed: is rate limited (429)
const isRateLimited = computed(() => {
  if (!props.account.rate_limit_reset_at) return false
  return new Date(props.account.rate_limit_reset_at) > new Date()
})

// Computed: is overloaded (529)
const isOverloaded = computed(() => {
  if (!props.account.overload_until) return false
  return new Date(props.account.overload_until) > new Date()
})

// Computed: has error status
const hasError = computed(() => {
  return props.account.status === 'error'
})

// Computed: status badge class
const statusClass = computed(() => {
  if (!props.account.schedulable || isRateLimited.value || isOverloaded.value) {
    return 'badge-gray'
  }
  switch (props.account.status) {
    case 'active':
      return 'badge-success'
    case 'inactive':
      return 'badge-gray'
    case 'error':
      return 'badge-danger'
    default:
      return 'badge-gray'
  }
})

// Computed: status text
const statusText = computed(() => {
  if (!props.account.schedulable) {
    return 'Paused'
  }
  if (isRateLimited.value || isOverloaded.value) {
    return 'Limited'
  }
  return props.account.status
})

</script>
