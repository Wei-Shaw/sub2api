<template>
  <div class="card">
    <div class="flex flex-col gap-4 border-b border-gray-100 px-6 py-4 dark:border-dark-700">
      <div class="flex flex-col gap-3 lg:flex-row lg:items-center lg:justify-between">
        <div>
          <h2 class="text-lg font-semibold text-gray-900 dark:text-white">{{ t('admin.riskControl.records') }}</h2>
          <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">{{ t('admin.riskControl.recordsHint') }}</p>
        </div>
        <button type="button" class="btn btn-secondary inline-flex items-center gap-2" :disabled="ctx.logsLoading.value" @click="ctx.loadLogs">
          <Icon name="refresh" size="sm" :class="ctx.logsLoading.value ? 'animate-spin' : ''" />
          {{ t('admin.riskControl.refresh') }}
        </button>
      </div>

      <RiskAuditLogFilters />
    </div>

    <div class="overflow-x-auto">
      <table class="min-w-full divide-y divide-gray-200 dark:divide-dark-700">
        <thead class="bg-gray-50 dark:bg-dark-800">
          <tr>
            <th class="px-5 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-gray-400">{{ t('admin.riskControl.table.time') }}</th>
            <th class="px-5 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-gray-400">{{ t('admin.riskControl.table.group') }}</th>
            <th class="px-5 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-gray-400">{{ t('admin.riskControl.table.user') }}</th>
            <th class="px-5 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-gray-400">{{ t('admin.riskControl.table.apiKey') }}</th>
            <th class="px-5 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-gray-400">{{ t('admin.riskControl.table.endpoint') }}</th>
            <th class="px-5 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-gray-400">{{ t('admin.riskControl.table.result') }}</th>
            <th class="px-5 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-gray-400">{{ t('admin.riskControl.table.highest') }}</th>
            <th class="px-5 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-gray-400">{{ t('admin.riskControl.table.actionMeta') }}</th>
            <th class="px-5 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-gray-400">{{ t('admin.riskControl.table.latency') }}</th>
            <th class="px-5 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-gray-400">{{ t('admin.riskControl.table.input') }}</th>
          </tr>
        </thead>
        <tbody class="divide-y divide-gray-100 bg-white dark:divide-dark-800 dark:bg-dark-800">
          <tr v-if="ctx.logsLoading.value">
            <td colspan="10" class="px-5 py-12 text-center text-sm text-gray-500 dark:text-gray-400">{{ t('common.loading') }}</td>
          </tr>
          <tr v-else-if="ctx.logs.value.length === 0">
            <td colspan="10" class="px-5 py-12 text-center text-sm text-gray-500 dark:text-gray-400">{{ t('admin.riskControl.emptyLogs') }}</td>
          </tr>
          <template v-else>
            <RiskAuditLogRow v-for="row in ctx.logs.value" :key="row.id" :row="row" />
          </template>
        </tbody>
      </table>
    </div>

    <Pagination
      v-if="ctx.pagination.total > 0"
      :page="ctx.pagination.page"
      :total="ctx.pagination.total"
      :page-size="ctx.pagination.page_size"
      @update:page="ctx.onPageChange"
      @update:pageSize="ctx.onPageSizeChange"
    />
  </div>
</template>

<script setup lang="ts">
import { inject } from 'vue'
import { useI18n } from 'vue-i18n'
import { Icon, Pagination } from '@sub2api/plugin-sdk'
import { RiskControlKey } from './riskControlContext'
import RiskAuditLogFilters from './RiskAuditLogFilters.vue'
import RiskAuditLogRow from './RiskAuditLogRow.vue'

const { t } = useI18n()
const ctx = inject(RiskControlKey)!
</script>
