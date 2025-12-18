<template>
  <div>
    <!-- Loading state -->
    <div v-if="loading" class="space-y-0.5">
      <div class="h-3 w-12 bg-gray-200 dark:bg-gray-700 rounded animate-pulse"></div>
      <div class="h-3 w-16 bg-gray-200 dark:bg-gray-700 rounded animate-pulse"></div>
      <div class="h-3 w-10 bg-gray-200 dark:bg-gray-700 rounded animate-pulse"></div>
    </div>

    <!-- Error state -->
    <div v-else-if="error" class="text-xs text-red-500">
      {{ error REDACTEDREDACTED
    </div>

    <!-- Stats data -->
    <div v-else-if="stats" class="space-y-0.5 text-xs">
      <!-- Requests -->
      <div class="flex items-center gap-1">
        <span class="text-gray-500 dark:text-gray-400">Req:</span>
        <span class="font-medium text-gray-700 dark:text-gray-300">{{ formatNumber(stats.requests) REDACTEDREDACTED</span>
      </div>
      <!-- Tokens -->
      <div class="flex items-center gap-1">
        <span class="text-gray-500 dark:text-gray-400">Tok:</span>
        <span class="font-medium text-gray-700 dark:text-gray-300">{{ formatTokens(stats.tokens) REDACTEDREDACTED</span>
      </div>
      <!-- Cost -->
      <div class="flex items-center gap-1">
        <span class="text-gray-500 dark:text-gray-400">Cost:</span>
        <span class="font-medium text-emerald-600 dark:text-emerald-400">{{ formatCurrency(stats.cost) REDACTEDREDACTED</span>
      </div>
    </div>

    <!-- No data -->
    <div v-else class="text-xs text-gray-400">
      -
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted REDACTED from 'vue'
import { adminAPI REDACTED from '@/api/admin'
import type { Account, WindowStats REDACTED from '@/types'
import { formatNumber, formatCurrency REDACTED from '@/utils/format'

const props = defineProps<{
  account: Account
REDACTED>()

const loading = ref(false)
const error = ref<string | null>(null)
const stats = ref<WindowStats | null>(null)

// Format large token numbers (e.g., 1234567 -> 1.23M)
const formatTokens = (tokens: number): string => {
  if (tokens >= 1000000) {
    return `${(tokens / 1000000).toFixed(2)REDACTEDM`
  REDACTED else if (tokens >= 1000) {
    return `${(tokens / 1000).toFixed(1)REDACTEDK`
  REDACTED
  return tokens.toString()
REDACTED

const loadStats = async () => {
  loading.value = true
  error.value = null

  try {
    stats.value = await adminAPI.accounts.getTodayStats(props.account.id)
  REDACTED catch (e: any) {
    error.value = 'Failed'
    console.error('Failed to load today stats:', e)
  REDACTED finally {
    loading.value = false
  REDACTED
REDACTED

onMounted(() => {
  loadStats()
REDACTED)
</script>
