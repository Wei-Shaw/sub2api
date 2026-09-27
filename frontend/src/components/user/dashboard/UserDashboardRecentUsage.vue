<template>
  <section class="card">
    <div class="card-header flex items-center justify-between gap-3">
      <h2 class="card-title">{{ t('dashboard.recentUsage') }}</h2>
      <span class="text-meta text-fg-muted">{{ t('dashboard.last7Days') }}</span>
    </div>
    <DataTable
      :columns="columns"
      :data="data"
      :loading="loading"
      row-key="id"
      :sticky-first-column="false"
      :sticky-actions-column="false"
    >
      <template #cell-model="{ row }">
        <span class="font-medium text-fg">{{ row.model }}</span>
      </template>
      <template #cell-created_at="{ row }">
        <span class="text-meta text-fg-muted">{{ formatDateTime(row.created_at) }}</span>
      </template>
      <template #cell-cost="{ row }">
        <span class="tabular-nums text-fg" :title="t('dashboard.actual')">${{ formatCost(row.actual_cost) }}</span>
        <span class="tabular-nums text-fg-subtle" :title="t('dashboard.standard')"> / ${{ formatCost(row.total_cost) }}</span>
      </template>
      <template #cell-tokens="{ row }">
        <span class="tabular-nums text-fg">{{ (row.input_tokens + row.output_tokens).toLocaleString() }}</span>
      </template>
      <template #empty>
        <EmptyState
          :title="t('dashboard.noUsageRecords')"
          :description="t('dashboard.startUsingApi')"
          :action-text="t('dashboard.createApiKey')"
          action-to="/keys"
        />
      </template>
    </DataTable>
    <div class="card-footer flex justify-center">
      <router-link to="/usage" class="btn btn-ghost btn-sm">
        {{ t('dashboard.viewAllUsage') }}
        <Icon name="arrowRight" size="sm" aria-hidden="true" />
      </router-link>
    </div>
  </section>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import DataTable from '@/components/common/DataTable.vue'
import EmptyState from '@/components/common/EmptyState.vue'
import Icon from '@/components/icons/Icon.vue'
import type { Column } from '@/components/common/types'
import { formatDateTime } from '@/utils/format'
import type { UsageLog } from '@/types'

defineProps<{
  data: UsageLog[]
  loading: boolean
}>()
const { t } = useI18n()
const formatCost = (c: number) => c.toFixed(4)

const columns = computed<Column[]>(() => [
  { key: 'model', label: t('dashboard.model') },
  { key: 'created_at', label: t('usage.time') },
  { key: 'cost', label: `${t('dashboard.actual')} / ${t('dashboard.standard')}`, class: 'text-right' },
  { key: 'tokens', label: t('dashboard.tokens'), class: 'text-right' }
])
</script>
