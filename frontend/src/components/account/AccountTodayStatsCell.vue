<template>
  <div>
    <!-- Loading state -->
    <div v-if="loading" class="space-y-0.5">
      <div class="h-3 w-12 animate-pulse rounded bg-gray-200 dark:bg-gray-700"></div>
      <div class="h-3 w-16 animate-pulse rounded bg-gray-200 dark:bg-gray-700"></div>
      <div class="h-3 w-10 animate-pulse rounded bg-gray-200 dark:bg-gray-700"></div>
    </div>

    <!-- Error state -->
    <div v-else-if="error" class="text-xs text-red-500">
      {{ error }}
    </div>

    <!-- Stats data -->
    <div v-else-if="stats" class="space-y-0.5 text-xs">
      <!-- Requests -->
      <div class="flex items-center gap-1">
        <span class="text-gray-500 dark:text-gray-400"
          >{{ t('admin.accounts.stats.requests') }}:</span
        >
        <span class="font-medium text-gray-700 dark:text-gray-300">{{
          formatNumber(stats.requests)
        }}</span>
      </div>
      <!-- Tokens -->
      <div class="flex items-center gap-1">
        <span class="text-gray-500 dark:text-gray-400"
          >{{ t('admin.accounts.stats.tokens') }}:</span
        >
        <span class="font-medium text-gray-700 dark:text-gray-300">{{
          formatTokens(stats.tokens)
        }}</span>
      </div>
      <!-- Cost -->
      <div class="flex items-center gap-1">
        <span class="text-gray-500 dark:text-gray-400"
          >{{ t('admin.accounts.stats.cost') }}:</span
        >
        <span class="font-medium text-emerald-600 dark:text-emerald-400">{{
          formatCurrency(stats.cost)
        }}</span>
      </div>
    </div>

    <!-- No data -->
    <div v-else class="text-xs text-gray-400">-</div>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { adminAPI } from '@/api/admin'
import type { Account, WindowStats } from '@/types'
import { formatNumber, formatCurrency } from '@/utils/format'

const props = defineProps<{
  account: Account
}>()

const { t } = useI18n()

const loading = ref(false)
const error = ref<string | null>(null)
const stats = ref<WindowStats | null>(null)

// Format large token numbers (e.g., 1234567 -> 1.23M)
const formatTokens = (tokens: number): string => {
  if (tokens >= 1000000) {
    return `${(tokens / 1000000).toFixed(2)}M`
  } else if (tokens >= 1000) {
    return `${(tokens / 1000).toFixed(1)}K`
  }
  return tokens.toString()
}

const loadStats = async () => {
  loading.value = true
  error.value = null

  try {
    stats.value = await adminAPI.accounts.getTodayStats(props.account.id)
  } catch (e: any) {
    error.value = 'Failed'
    console.error('Failed to load today stats:', e)
  } finally {
    loading.value = false
  }
}

onMounted(() => {
  loadStats()
})
</script>
