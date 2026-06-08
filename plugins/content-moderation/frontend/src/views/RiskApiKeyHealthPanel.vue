<template>
  <div class="rounded-lg border border-gray-100 bg-gray-50 p-3 dark:border-dark-700 dark:bg-dark-900/30">
    <div class="mb-3 flex items-start justify-between gap-3">
      <div class="min-w-0">
        <p class="text-sm font-semibold text-gray-900 dark:text-white">{{ t('admin.riskControl.apiKeyHealth') }}</p>
        <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.riskControl.apiKeyFreezeRule') }}</p>
      </div>
      <span class="inline-flex shrink-0 items-center whitespace-nowrap rounded-full bg-white px-2 py-0.5 text-[11px] font-medium leading-5 text-gray-600 shadow-sm dark:bg-dark-800 dark:text-gray-300">
        {{ t('admin.riskControl.apiKeyRows', { count: ctx.apiKeyRows.value.length }) }}
      </span>
    </div>

    <div v-if="ctx.apiKeyRows.value.length === 0" class="flex min-h-32 flex-col items-center justify-center rounded-lg border border-dashed border-gray-200 bg-white px-4 py-6 text-center dark:border-dark-700 dark:bg-dark-800">
      <Icon name="infoCircle" size="lg" class="text-gray-300 dark:text-dark-500" />
      <p class="mt-2 text-sm font-medium text-gray-700 dark:text-gray-200">{{ t('admin.riskControl.apiKeyHealthEmpty') }}</p>
      <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.riskControl.apiKeyHealthEmptyHint') }}</p>
    </div>
    <div v-else class="space-y-2">
      <div class="space-y-2" :class="ctx.apiKeyRowsExpanded.value ? 'max-h-72 overflow-y-auto pr-1' : ''">
        <RiskApiKeyRow
          v-for="(row, index) in ctx.visibleApiKeyRows.value"
          :key="ctx.apiKeyRowKey(row, index)"
          :row="row"
        />
      </div>

      <div v-if="ctx.canToggleApiKeyRows.value" class="flex items-center justify-between gap-3 rounded-lg border border-dashed border-gray-200 bg-white px-3 py-2 text-xs text-gray-500 dark:border-dark-700 dark:bg-dark-800 dark:text-gray-400">
        <span class="min-w-0 truncate">
          {{ ctx.apiKeyRowsExpanded.value ? t('admin.riskControl.apiKeyRowsExpanded', { count: ctx.apiKeyRows.value.length }) : t('admin.riskControl.apiKeyRowsCollapsed', { count: ctx.hiddenApiKeyRowCount.value }) }}
        </span>
        <button
          type="button"
          class="inline-flex shrink-0 items-center gap-1 rounded-md px-2 py-1 font-medium text-primary-600 transition-colors hover:bg-primary-50 hover:text-primary-700 dark:text-primary-300 dark:hover:bg-primary-900/20"
          @click="ctx.apiKeyRowsExpanded.value = !ctx.apiKeyRowsExpanded.value"
        >
          <Icon :name="ctx.apiKeyRowsExpanded.value ? 'chevronUp' : 'chevronDown'" size="xs" />
          {{ ctx.apiKeyRowsExpanded.value ? t('admin.riskControl.collapseApiKeyRows') : t('admin.riskControl.expandApiKeyRows') }}
        </button>
      </div>
    </div>

    <RiskAuditTestResult v-if="ctx.moderationTestResult.value" />
  </div>
</template>

<script setup lang="ts">
import { inject } from 'vue'
import { useI18n } from 'vue-i18n'
import { Icon } from '@sub2api/plugin-sdk'
import { RiskControlKey } from './riskControlContext'
import RiskApiKeyRow from './RiskApiKeyRow.vue'
import RiskAuditTestResult from './RiskAuditTestResult.vue'

const { t } = useI18n()
const ctx = inject(RiskControlKey)!
</script>
