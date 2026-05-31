<template>
  <div class="card p-4">
    <div class="mb-4 flex items-center justify-between gap-3">
      <div class="flex items-center gap-2">
        <span class="rounded-lg bg-amber-100 p-2 text-amber-600 dark:bg-amber-900/30 dark:text-amber-300">
          <Icon name="fire" size="sm" />
        </span>
        <h3 class="text-sm font-semibold text-gray-900 dark:text-white">
          {{ t('admin.usage.tokenLeaderboardTitle') }}
        </h3>
      </div>
      <span class="text-xs font-medium text-gray-500 dark:text-gray-400">
        {{ t('admin.usage.tokenLeaderboardTop', { count: maxRows }) }}
      </span>
    </div>

    <div v-if="loading" class="flex h-40 items-center justify-center">
      <LoadingSpinner />
    </div>
    <div
      v-else-if="error"
      class="flex h-40 items-center justify-center text-sm text-red-500 dark:text-red-400"
    >
      {{ t('admin.usage.tokenLeaderboardLoadFailed') }}
    </div>
    <div
      v-else-if="items.length === 0"
      class="flex h-40 items-center justify-center text-sm text-gray-500 dark:text-gray-400"
    >
      {{ t('admin.usage.tokenLeaderboardNoData') }}
    </div>

    <div v-else class="overflow-x-auto">
      <table class="w-full min-w-[680px] text-sm">
        <thead>
          <tr class="border-b border-gray-100 text-xs text-gray-500 dark:border-dark-700 dark:text-gray-400">
            <th class="w-16 pb-2 text-left font-medium">{{ t('admin.usage.tokenLeaderboardRank') }}</th>
            <th class="pb-2 text-left font-medium">{{ t('admin.usage.tokenLeaderboardUser') }}</th>
            <th class="pb-2 text-right font-medium">{{ t('admin.usage.tokenLeaderboardTokens') }}</th>
            <th class="pb-2 text-right font-medium">{{ t('admin.usage.tokenLeaderboardRequests') }}</th>
            <th class="pb-2 text-right font-medium">{{ t('admin.usage.tokenLeaderboardCost') }}</th>
          </tr>
        </thead>
        <tbody>
          <tr
            v-for="(item, index) in items"
            :key="item.user_id"
            class="border-b border-gray-50 last:border-0 dark:border-dark-700/60"
          >
            <td class="py-3">
              <span
                :class="[
                  'flex h-7 w-7 items-center justify-center rounded-full text-xs font-bold',
                  rankClass(index)
                ]"
              >
                {{ index + 1 }}
              </span>
            </td>
            <td class="py-3">
              <button
                type="button"
                class="max-w-[280px] truncate text-left font-medium text-gray-800 hover:text-primary-600 dark:text-gray-100 dark:hover:text-primary-400"
                :title="userLabel(item)"
                @click="$emit('userClick', item.user_id)"
              >
                {{ userLabel(item) }}
              </button>
              <div class="mt-2 h-1.5 w-full max-w-xs overflow-hidden rounded-full bg-gray-100 dark:bg-dark-700">
                <div
                  class="h-full rounded-full bg-amber-500"
                  :style="{ width: `${tokenPercent(item.total_tokens)}%` }"
                />
              </div>
            </td>
            <td class="py-3 text-right font-semibold text-gray-900 dark:text-white">
              {{ formatTokens(item.total_tokens) }}
            </td>
            <td class="py-3 text-right text-gray-600 dark:text-gray-300">
              {{ item.requests.toLocaleString() }}
            </td>
            <td class="py-3 text-right text-green-600 dark:text-green-400">
              ${{ formatCost(item.actual_cost) }}
            </td>
          </tr>
        </tbody>
      </table>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import type { UserBreakdownItem } from '@/types'

const props = withDefaults(defineProps<{
  items: UserBreakdownItem[]
  loading?: boolean
  error?: boolean
  maxRows?: number
}>(), {
  loading: false,
  error: false,
  maxRows: 10
})

defineEmits<{
  userClick: [userId: number]
}>()

const { t } = useI18n()

const maxTokens = computed(() => Math.max(...props.items.map((item) => item.total_tokens), 0))

const userLabel = (item: UserBreakdownItem): string => item.email || `User #${item.user_id}`

const tokenPercent = (tokens: number): number => {
  if (maxTokens.value <= 0) return 0
  return Math.max(4, Math.round((tokens / maxTokens.value) * 100))
}

const rankClass = (index: number): string => {
  if (index === 0) return 'bg-yellow-100 text-yellow-700 dark:bg-yellow-900/30 dark:text-yellow-300'
  if (index === 1) return 'bg-gray-200 text-gray-700 dark:bg-gray-700 dark:text-gray-200'
  if (index === 2) return 'bg-amber-100 text-amber-700 dark:bg-amber-900/30 dark:text-amber-300'
  return 'bg-gray-100 text-gray-500 dark:bg-dark-700 dark:text-gray-400'
}

const formatTokens = (value: number): string => {
  if (value >= 1_000_000_000) return `${(value / 1_000_000_000).toFixed(2)}B`
  if (value >= 1_000_000) return `${(value / 1_000_000).toFixed(2)}M`
  if (value >= 1_000) return `${(value / 1_000).toFixed(2)}K`
  return value.toLocaleString()
}

const formatCost = (value: number): string => {
  if (value >= 1000) return `${(value / 1000).toFixed(2)}K`
  if (value >= 1) return value.toFixed(2)
  if (value >= 0.01) return value.toFixed(3)
  return value.toFixed(4)
}
</script>
