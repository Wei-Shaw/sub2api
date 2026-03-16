<template>
  <div class="bg-gray-50/50 dark:bg-dark-700/30">
    <div v-if="loading" class="flex items-center justify-center py-3">
      <LoadingSpinner />
    </div>
    <div v-else-if="items.length === 0" class="py-2 text-center text-xs text-gray-400">
      {{ t('admin.dashboard.noDataAvailable') REDACTEDREDACTED
    </div>
    <table v-else class="w-full text-xs">
      <tbody>
        <tr
          v-for="user in items"
          :key="user.user_id"
          class="border-t border-gray-100/50 dark:border-gray-700/50"
        >
          <td class="max-w-[120px] truncate py-1 pl-6 text-gray-600 dark:text-gray-300" :title="user.email">
            {{ user.email || `User #${user.user_idREDACTED` REDACTEDREDACTED
          </td>
          <td class="py-1 text-right text-gray-500 dark:text-gray-400">
            {{ user.requests.toLocaleString() REDACTEDREDACTED
          </td>
          <td class="py-1 text-right text-gray-500 dark:text-gray-400">
            {{ formatTokens(user.total_tokens) REDACTEDREDACTED
          </td>
          <td class="py-1 text-right text-green-600 dark:text-green-400">
            ${{ formatCost(user.actual_cost) REDACTEDREDACTED
          </td>
          <td class="py-1 pr-1 text-right text-gray-400 dark:text-gray-500">
            ${{ formatCost(user.cost) REDACTEDREDACTED
          </td>
        </tr>
      </tbody>
    </table>
  </div>
</template>

<script setup lang="ts">
import { useI18n REDACTED from 'vue-i18n'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import type { UserBreakdownItem REDACTED from '@/types'

const { t REDACTED = useI18n()

defineProps<{
  items: UserBreakdownItem[]
  loading?: boolean
REDACTED>()

const formatTokens = (value: number): string => {
  if (value >= 1_000_000_000) return `${(value / 1_000_000_000).toFixed(2)REDACTEDB`
  if (value >= 1_000_000) return `${(value / 1_000_000).toFixed(2)REDACTEDM`
  if (value >= 1_000) return `${(value / 1_000).toFixed(2)REDACTEDK`
  return value.toLocaleString()
REDACTED

const formatCost = (value: number): string => {
  if (value >= 1000) return (value / 1000).toFixed(2) + 'K'
  if (value >= 1) return value.toFixed(2)
  if (value >= 0.01) return value.toFixed(3)
  return value.toFixed(4)
REDACTED
</script>
