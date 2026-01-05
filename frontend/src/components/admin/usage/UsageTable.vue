<template>
  <div class="card overflow-hidden"><div class="overflow-auto">
    <DataTable :columns="cols" :data="data" :loading="loading">
      <template #cell-user="{ row REDACTED"><div class="text-sm"><span class="font-medium text-gray-900 dark:text-white">{{ row.user?.email || '-' REDACTEDREDACTED</span><span class="ml-1 text-xs text-gray-400">#{{ row.user_id REDACTEDREDACTED</span></div></template>
      <template #cell-model="{ value REDACTED"><span class="font-medium">{{ value REDACTEDREDACTED</span></template>
      <template #cell-tokens="{ row REDACTED"><div class="text-sm">In: {{ row.input_tokens.toLocaleString() REDACTEDREDACTED / Out: {{ row.output_tokens.toLocaleString() REDACTEDREDACTED</div></template>
      <template #cell-cost="{ row REDACTED"><span class="font-medium text-green-600">${{ row.actual_cost.toFixed(6) REDACTEDREDACTED</span></template>
      <template #cell-created_at="{ value REDACTED"><span class="text-sm text-gray-500">{{ formatDateTime(value) REDACTEDREDACTED</span></template>
      <template #empty><EmptyState :message="t('usage.noRecords')" /></template>
    </DataTable>
  </div></div>
</template>

<script setup lang="ts">
import { computed REDACTED from 'vue'; import { useI18n REDACTED from 'vue-i18n'; import { formatDateTime REDACTED from '@/utils/format'; import DataTable from '@/components/common/DataTable.vue'; import EmptyState from '@/components/common/EmptyState.vue'
defineProps(['data', 'loading']); const { t REDACTED = useI18n()
const cols = computed(() => [
  { key: 'user', label: t('admin.usage.user') REDACTED, { key: 'model', label: t('usage.model'), sortable: true REDACTED,
  { key: 'tokens', label: t('usage.tokens') REDACTED, { key: 'cost', label: t('usage.cost') REDACTED,
  { key: 'created_at', label: t('usage.time'), sortable: true REDACTED
])
</script>