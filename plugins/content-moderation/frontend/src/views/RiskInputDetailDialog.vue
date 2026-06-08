<template>
  <BaseDialog
    :show="ctx.inputDetailRow.value !== null"
    :title="t('admin.riskControl.inputDetailTitle')"
    width="wide"
    @close="ctx.closeInputDetail"
  >
    <div v-if="ctx.inputDetailRow.value" class="space-y-5">
      <div class="grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-4">
        <div class="rounded-lg border border-gray-100 bg-gray-50 p-4 dark:border-dark-700 dark:bg-dark-800/70">
          <p class="text-xs font-medium text-gray-500 dark:text-gray-400">{{ t('admin.riskControl.table.time') }}</p>
          <p class="mt-1 truncate text-sm font-semibold text-gray-900 dark:text-white">{{ ctx.formatDateTime(ctx.inputDetailRow.value.created_at) }}</p>
        </div>
        <div class="rounded-lg border border-gray-100 bg-gray-50 p-4 dark:border-dark-700 dark:bg-dark-800/70">
          <p class="text-xs font-medium text-gray-500 dark:text-gray-400">{{ t('admin.riskControl.table.user') }}</p>
          <p class="mt-1 truncate text-sm font-semibold text-gray-900 dark:text-white">{{ ctx.inputDetailRow.value.user_email || '-' }}</p>
        </div>
        <div class="rounded-lg border border-gray-100 bg-gray-50 p-4 dark:border-dark-700 dark:bg-dark-800/70">
          <p class="text-xs font-medium text-gray-500 dark:text-gray-400">{{ t('admin.riskControl.table.result') }}</p>
          <span class="mt-1 inline-flex rounded-md px-2 py-1 text-xs font-medium" :class="ctx.resultBadgeClass(ctx.inputDetailRow.value)">
            {{ ctx.resultLabel(ctx.inputDetailRow.value) }}
          </span>
        </div>
        <div class="rounded-lg border border-gray-100 bg-gray-50 p-4 dark:border-dark-700 dark:bg-dark-800/70">
          <p class="text-xs font-medium text-gray-500 dark:text-gray-400">{{ t('admin.riskControl.table.highest') }}</p>
          <p class="mt-1 truncate text-sm font-semibold text-gray-900 dark:text-white">
            {{ ctx.inputDetailRow.value.highest_category || '-' }} / {{ ctx.percent(ctx.inputDetailRow.value.highest_score) }}
          </p>
        </div>
      </div>

      <div class="rounded-xl border border-gray-100 bg-white p-4 shadow-sm dark:border-dark-700 dark:bg-dark-800">
        <div class="flex flex-wrap items-center justify-between gap-3">
          <div>
            <p class="text-sm font-semibold text-gray-900 dark:text-white">{{ t('admin.riskControl.inputDetailContent') }}</p>
            <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
              {{ ctx.inputDetailRow.value.endpoint || '-' }} · {{ ctx.inputDetailRow.value.provider || '-' }} / {{ ctx.inputDetailRow.value.model || '-' }}
            </p>
          </div>
          <span v-if="ctx.inputDetailRow.value.group_name" class="inline-flex rounded-md bg-sky-50 px-2.5 py-1 text-xs font-medium text-sky-700 dark:bg-sky-900/20 dark:text-sky-300">
            {{ ctx.inputDetailRow.value.group_name }}
          </span>
        </div>
        <pre class="mt-4 max-h-[420px] overflow-auto whitespace-pre-wrap break-words rounded-lg bg-gray-950 p-4 text-sm leading-6 text-gray-100 shadow-inner dark:bg-black/50">{{ ctx.inputDetailText.value }}</pre>
      </div>
    </div>

    <template #footer>
      <div class="flex justify-end">
        <button type="button" class="btn btn-secondary" @click="ctx.closeInputDetail">{{ t('common.close') }}</button>
      </div>
    </template>
  </BaseDialog>
</template>

<script setup lang="ts">
import { inject } from 'vue'
import { useI18n } from 'vue-i18n'
import { BaseDialog } from '@sub2api/plugin-sdk'
import { RiskControlKey } from './riskControlContext'

const { t } = useI18n()
const ctx = inject(RiskControlKey)!
</script>
