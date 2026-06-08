<template>
  <div class="grid grid-cols-1 gap-3 md:grid-cols-2 xl:grid-cols-4">
    <div
      v-for="item in ctx.overviewItems.value"
      :key="item.key"
      class="rounded-lg border border-gray-100 bg-white px-4 py-3 shadow-sm dark:border-dark-700 dark:bg-dark-800"
    >
      <div class="flex min-w-0 items-center gap-3">
        <div class="flex h-9 w-9 flex-shrink-0 items-center justify-center rounded-lg" :class="item.iconClass">
          <Icon :name="item.icon" size="sm" />
        </div>
        <div class="min-w-0 flex-1">
          <div class="flex min-w-0 items-center justify-between gap-2">
            <p class="truncate text-xs font-medium text-gray-500 dark:text-gray-400">{{ item.label }}</p>
            <span
              v-if="item.badge"
              class="inline-flex flex-shrink-0 items-center rounded-full px-2 py-0.5 text-xs font-medium"
              :class="item.badgeClass"
            >
              {{ item.badge }}
            </span>
          </div>
          <div class="mt-1 flex min-w-0 items-baseline gap-2">
            <p class="truncate text-xl font-semibold leading-7 text-gray-900 dark:text-white">{{ item.value }}</p>
            <p v-if="item.meta" class="truncate text-xs text-gray-500 dark:text-gray-400">{{ item.meta }}</p>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { inject } from 'vue'
import { Icon } from '@sub2api/plugin-sdk'
import { RiskControlKey } from './riskControlContext'

const ctx = inject(RiskControlKey)!
</script>
