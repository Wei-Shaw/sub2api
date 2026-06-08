<template>
  <div class="flex flex-col gap-2 rounded-lg border border-gray-100 bg-gray-50 px-3 py-2 dark:border-dark-700 dark:bg-dark-900/30 sm:flex-row sm:items-center sm:justify-between">
    <div class="flex min-w-0 items-center gap-2 text-sm text-gray-700 dark:text-gray-200">
      <Icon name="filter" size="sm" class="flex-shrink-0 text-gray-400" />
      <span class="font-medium">{{ t('admin.riskControl.modelFilter') }}</span>
      <span class="truncate text-gray-500 dark:text-gray-400">{{ ctx.modelFilterSummary.value }}</span>
    </div>
    <div v-if="ctx.modelFilterPreviewModels.value.length > 0" class="flex flex-wrap gap-1.5">
      <span
        v-for="model in ctx.modelFilterPreviewModels.value"
        :key="model"
        class="inline-flex max-w-[180px] items-center truncate rounded-md bg-white px-2 py-1 font-mono text-xs text-gray-600 shadow-sm dark:bg-dark-800 dark:text-gray-300"
      >
        {{ model }}
      </span>
      <span v-if="ctx.hiddenModelFilterModelCount.value > 0" class="inline-flex rounded-md bg-white px-2 py-1 text-xs text-gray-500 shadow-sm dark:bg-dark-800 dark:text-gray-400">
        +{{ ctx.hiddenModelFilterModelCount.value }}
      </span>
    </div>
  </div>

  <div class="grid grid-cols-1 gap-3 md:grid-cols-2 xl:grid-cols-6">
    <Select v-model="ctx.filters.result" :options="ctx.resultOptions.value" @change="ctx.reloadLogsFromFirstPage" />
    <Select v-model="ctx.filters.group_id" :options="ctx.groupFilterOptions.value" @change="ctx.reloadLogsFromFirstPage" />
    <Select v-model="ctx.filters.endpoint" :options="ctx.endpointOptions.value" @change="ctx.reloadLogsFromFirstPage" />
    <input v-model.trim="ctx.filters.search" type="search" class="input" :placeholder="t('admin.riskControl.filters.search')" @keyup.enter="ctx.reloadLogsFromFirstPage" />
    <input v-model="ctx.filters.from" type="datetime-local" class="input" :title="t('admin.riskControl.filters.from')" @change="ctx.reloadLogsFromFirstPage" />
    <input v-model="ctx.filters.to" type="datetime-local" class="input" :title="t('admin.riskControl.filters.to')" @change="ctx.reloadLogsFromFirstPage" />
  </div>
</template>

<script setup lang="ts">
import { inject } from 'vue'
import { useI18n } from 'vue-i18n'
import { Icon, Select } from '@sub2api/plugin-sdk'
import { RiskControlKey } from './riskControlContext'

const { t } = useI18n()
const ctx = inject(RiskControlKey)!
</script>
