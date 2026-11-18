<template>
  <div v-if="groups.length" class="space-y-1.5">
    <table v-for="group in groups" :key="group[0].key"
      class="quota-matrix border-separate border-spacing-x-1.5 text-right text-[10px] tabular-nums">
      <thead class="text-gray-500 dark:text-gray-400">
        <tr>
          <th scope="col" class="font-normal">{{ t('admin.accounts.usageWindow.remainingQuota') }}</th>
          <th v-for="col in group" :key="col.key" scope="col" class="font-medium">{{ col.name }}</th>
        </tr>
      </thead>
      <tbody>
        <tr v-for="row in rows(group)" :key="row.period">
          <th scope="row" class="align-middle font-medium text-gray-500">{{ row.period }}</th>
          <td v-for="{ key, usage } in row.cells" :key="key">
            <UsageProgressBar v-if="usage" matrix :label="row.period" color="indigo"
              :utilization="usage.utilization"
              :utilization-unknown="usage.utilization_unknown"
              :resets-at="usage.resets_at"
              :window-stats="usage.window_stats"
              :estimated-total-cost="usage.estimatedTotalCost"
              :show-now-when-idle="showNowWhenIdle" />
            <span v-else class="text-gray-400">—</span>
          </td>
        </tr>
      </tbody>
    </table>
  </div>
</template>

<script lang="ts">
import type { UsageProgress } from '@/types'

type Window = Pick<UsageProgress, 'utilization' | 'utilization_unknown' | 'resets_at' | 'window_stats'>
export interface QuotaColumn {
  key: string
  name: string
  windows: Record<string, (Window & { estimatedTotalCost?: number | null }) | undefined | null>
}
</script>

<script setup lang="ts">
import { computed } from 'vue'
import { useMediaQuery } from '@vueuse/core'
import { useI18n } from 'vue-i18n'
import UsageProgressBar from './UsageProgressBar.vue'

const props = defineProps<{ columns: QuotaColumn[]; showNowWhenIdle?: boolean }>()
const { t } = useI18n()
const desktop = useMediaQuery('(min-width: 768px)')
const groups = computed(() => {
  const columns = props.columns.filter(col => Object.values(col.windows).some(Boolean))
  if (desktop.value) return columns.length ? [columns] : []
  return Array.from({ length: Math.ceil(columns.length / 2) }, (_, i) => columns.slice(i * 2, i * 2 + 2))
})
function rows(columns: QuotaColumn[]) {
  const periods = new Set(columns.flatMap(col => Object.keys(col.windows).filter(key => col.windows[key])))
  return [...periods].map(period => ({
    period, cells: columns.map(col => ({ key: col.key, usage: col.windows[period] }))
  }))
}
</script>

<style scoped>
/* The matrix can sit inside the admin table or a mobile account card. */
.quota-matrix th,
.quota-matrix td {
  padding: 0.125rem 0;
  border: 0;
  background: transparent;
  text-align: right;
  font-size: 0.625rem;
  white-space: nowrap;
}
.quota-matrix th:first-child {
  text-align: left;
}
</style>
