<template>
  <div class="newapi-recent-card card">
    <div class="flex items-center justify-between border-b border-gray-100 px-4 py-3 dark:border-dark-700">
      <div>
        <h2 class="text-sm font-semibold text-gray-900 dark:text-white">{{ t('dashboard.recentUsage') }}</h2>
        <p class="mt-0.5 text-xs text-gray-500 dark:text-dark-400">{{ t('dashboard.last7Days') }}</p>
      </div>
      <span class="badge badge-gray">{{ t('dashboard.last7Days') }}</span>
    </div>
    <div class="p-0">
      <div v-if="loading" class="flex items-center justify-center py-12">
        <LoadingSpinner size="lg" />
      </div>
      <div v-else-if="data.length === 0" class="py-8">
        <EmptyState :title="t('dashboard.noUsageRecords')" :description="t('dashboard.startUsingApi')" />
      </div>
      <div v-else>
        <div class="grid grid-cols-[minmax(0,1.6fr)_minmax(100px,0.8fr)_minmax(90px,0.7fr)] border-b border-gray-100 bg-gray-50/70 px-4 py-2 text-[10px] font-semibold uppercase tracking-wide text-gray-500 dark:border-dark-700 dark:bg-dark-800/60 dark:text-dark-400">
          <span>{{ t('dashboard.model') }}</span><span>{{ t('dashboard.time') }}</span><span class="text-right">{{ t('dashboard.cost') }}</span>
        </div>
        <div v-for="log in data" :key="log.id" class="grid grid-cols-[minmax(0,1.6fr)_minmax(100px,0.8fr)_minmax(90px,0.7fr)] items-center border-b border-gray-100 px-4 py-3 transition-colors hover:bg-gray-50 dark:border-dark-700 dark:hover:bg-dark-800/70">
          <div class="flex min-w-0 items-center gap-2">
            <span class="flex h-6 w-6 shrink-0 items-center justify-center rounded bg-blue-50 text-blue-600 dark:bg-blue-900/30 dark:text-blue-300"><Icon name="beaker" size="xs" /></span>
            <div class="min-w-0"><p class="truncate text-xs font-medium text-gray-900 dark:text-white">{{ log.model }}</p><p class="text-[10px] text-gray-500 dark:text-dark-400">{{ (log.input_tokens + log.output_tokens).toLocaleString() }} tokens</p></div>
          </div>
          <span class="text-[11px] text-gray-500 dark:text-dark-400">{{ formatDateTime(log.created_at) }}</span>
          <span class="text-right text-xs font-semibold text-emerald-600 dark:text-emerald-400">${{ formatCost(log.actual_cost) }}</span>
        </div>

        <router-link to="/usage" class="flex items-center justify-center gap-2 px-4 py-3 text-xs font-medium text-blue-600 transition-colors hover:bg-blue-50 hover:text-blue-700 dark:text-blue-400 dark:hover:bg-blue-900/20 dark:hover:text-blue-300">
          {{ t('dashboard.viewAllUsage') }}
          <Icon name="arrowRight" size="sm" />
        </router-link>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import EmptyState from '@/components/common/EmptyState.vue'
import Icon from '@/components/icons/Icon.vue'
import { formatDateTime } from '@/utils/format'
import type { UsageLog } from '@/types'

defineProps<{
  data: UsageLog[]
  loading: boolean
}>()
const { t } = useI18n()
const formatCost = (c: number) => c.toFixed(4)
</script>
