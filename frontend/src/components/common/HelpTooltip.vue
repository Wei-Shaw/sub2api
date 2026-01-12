<script setup lang="ts">
import { ref REDACTED from 'vue'

defineProps<{
  content?: string
REDACTED>()

const show = ref(false)
</script>

<template>
  <div
    class="group relative ml-1 inline-flex items-center align-middle"
    @mouseenter="show = true"
    @mouseleave="show = false"
  >
    <!-- Trigger Icon -->
    <slot name="trigger">
      <svg
        class="h-4 w-4 cursor-help text-gray-400 transition-colors hover:text-primary-600 dark:text-gray-500 dark:hover:text-primary-400"
        fill="none"
        viewBox="0 0 24 24"
        stroke="currentColor"
        stroke-width="2"
      >
        <path
          stroke-linecap="round"
          stroke-linejoin="round"
          d="M13 16h-1v-4h-1m1-4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z"
        />
      </svg>
    </slot>

    <!-- Popover Content -->
    <div
      v-show="show"
      class="absolute bottom-full left-1/2 z-50 mb-2 w-64 -translate-x-1/2 rounded-lg bg-gray-900 p-3 text-xs leading-relaxed text-white shadow-xl ring-1 ring-white/10 opacity-0 transition-opacity duration-200 group-hover:opacity-100 dark:bg-gray-800"
    >
      <slot>{{ content REDACTEDREDACTED</slot>
      <div class="absolute -bottom-1 left-1/2 h-2 w-2 -translate-x-1/2 rotate-45 bg-gray-900 dark:bg-gray-800"></div>
    </div>
  </div>
</template>

