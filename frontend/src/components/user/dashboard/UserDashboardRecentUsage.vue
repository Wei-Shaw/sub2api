<template>
  <Surface :title="t('dashboard.recentUsage')" flush data-testid="dashboard-recent-usage">
    <template #actions>
      <Badge tone="neutral">{{ t('dashboard.last7Days') }}</Badge>
    </template>

    <!-- Flat hairline placeholder. No shimmer sweep, no spinner jump. -->
    <div v-if="loading" class="space-y-3 p-4" data-testid="dashboard-recent-usage-loading">
      <div class="skeleton h-3 w-full"></div>
      <div class="skeleton h-3 w-4/5"></div>
      <div class="skeleton h-3 w-2/3"></div>
    </div>

    <div v-else-if="data.length === 0" class="py-4">
      <EmptyState :title="t('dashboard.noUsageRecords')" :description="t('dashboard.startUsingApi')" />
    </div>

    <div v-else class="overflow-x-auto">
      <table class="table min-w-[36rem]" data-testid="dashboard-recent-usage-table">
        <thead>
          <tr>
            <th scope="col">{{ t('dashboard.model') }}</th>
            <th scope="col">{{ t('usage.time') }}</th>
            <th scope="col" class="is-numeric">{{ t('dashboard.tokens') }}</th>
            <th scope="col" class="is-numeric">{{ t('dashboard.actual') }}</th>
            <th scope="col" class="is-numeric">{{ t('dashboard.standard') }}</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="log in data" :key="log.id">
            <th scope="row" class="max-w-[12rem] text-left font-normal text-ink">
              <span class="block truncate text-xs" :title="log.model">{{ log.model }}</span>
            </th>
            <td class="whitespace-nowrap font-mono text-xs tabular-nums">
              {{ formatDateTime(log.created_at) }}
            </td>
            <td class="is-numeric"><NumCell :value="totalTokens(log)" /></td>
            <td class="is-numeric"><NumCell :value="numOrNull(log.actual_cost)" :precision="4" /></td>
            <td class="is-numeric"><NumCell :value="numOrNull(log.total_cost)" :precision="4" /></td>
          </tr>
        </tbody>
      </table>
    </div>

    <template v-if="!loading && data.length > 0" #footer>
      <router-link
        to="/usage"
        class="inline-flex items-center gap-1.5 text-xs font-medium text-accent underline-offset-2 transition-colors duration-fast hover:underline"
      >
        {{ t('dashboard.viewAllUsage') }}
        <Icon name="arrowRight" size="xs" />
      </router-link>
    </template>
  </Surface>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'

import Badge from '@/components/common/Badge.vue'
import EmptyState from '@/components/common/EmptyState.vue'
import NumCell from '@/components/common/NumCell.vue'
import Surface from '@/components/common/Surface.vue'
import Icon from '@/components/icons/Icon.vue'
import { formatDateTime } from '@/utils/format'
import type { UsageLog } from '@/types'

/**
 * Five rows of usage. Previously rendered as a list of rounded tinted blocks,
 * each with a 40px pastel icon tile and its costs in green — five quantities
 * that could not be compared to each other because none of them shared a
 * column. It is a table now: one row per request, numbers in mono tabular
 * figures, right aligned.
 */
defineProps<{
  data: UsageLog[]
  loading: boolean
}>()

const { t } = useI18n()

function numOrNull(value: unknown): number | null {
  if (value === null || value === undefined || value === '') return null
  const n = Number(value)
  return Number.isFinite(n) ? n : null
}

/** Input + output only, matching what the row used to print. */
function totalTokens(log: UsageLog): number | null {
  const input = numOrNull(log.input_tokens)
  const output = numOrNull(log.output_tokens)
  if (input === null && output === null) return null
  return (input ?? 0) + (output ?? 0)
}
</script>
