<template>
  <BaseDialog
    :show="show"
    :title="t('admin.accounts.batchModelSync.title')"
    width="wide"
    :show-close-button="!running"
    :close-on-escape="!running"
    @close="emit('close')"
  >
    <div class="space-y-4">
      <p class="text-sm text-gray-600 dark:text-gray-300">{{ t('admin.accounts.batchModelSync.hint') }}</p>
      <div class="space-y-2" role="status" aria-live="polite">
        <div class="flex flex-wrap justify-between gap-2 text-sm">
          <span>{{ t('admin.accounts.batchModelSync.progress', { done: completed, total: results.length }) }}</span>
          <span>{{ t('admin.accounts.batchModelSync.summary', counts) }}</span>
        </div>
        <progress class="h-2 w-full accent-primary-600" :value="completed" :max="results.length || 1" />
      </div>
      <p v-if="results.some(row => row.status === 'unknown')" class="rounded-lg bg-amber-50 p-3 text-sm text-amber-800 dark:bg-amber-900/20 dark:text-amber-200">
        {{ t('admin.accounts.batchModelSync.unknownHint') }}
      </p>
      <div class="max-h-96 overflow-y-auto rounded-lg border border-gray-200 dark:border-dark-600">
        <div v-for="row in results" :key="row.account_id" class="space-y-1 border-b border-gray-100 px-3 py-3 last:border-b-0 dark:border-dark-700">
          <div class="flex flex-wrap items-center justify-between gap-2">
            <span class="min-w-0 break-all text-sm font-medium text-gray-900 dark:text-gray-100">{{ row.name }} <span class="text-xs text-gray-400">#{{ row.account_id }}</span></span>
            <span class="text-xs font-medium" :class="statusClass(row.status)">{{ t(`admin.accounts.batchModelSync.status.${row.status}`) }}</span>
          </div>
          <p v-if="row.status === 'success' || row.status === 'warning'" class="text-xs text-gray-500 dark:text-gray-400">
            {{ t('admin.accounts.batchModelSync.modelCounts', { total: row.model_count, added: row.added_count }) }}
            <span v-if="row.mapping_unchanged"> · {{ t('admin.accounts.batchModelSync.mappingUnchanged') }}</span>
          </p>
          <p v-for="warning in row.warnings" :key="warning.code" class="break-words text-xs text-amber-700 dark:text-amber-300">{{ warningMessage(warning) }}</p>
          <p v-if="row.error" class="break-words text-xs text-rose-600 dark:text-rose-400">{{ row.error }}</p>
        </div>
      </div>
    </div>
    <template #footer>
      <div class="flex flex-wrap justify-end gap-3">
        <button v-if="running" class="btn btn-secondary" @click="emit('stop')">{{ t('admin.accounts.batchModelSync.stop') }}</button>
        <template v-else>
          <button class="btn btn-secondary" @click="emit('close')">{{ t('common.close') }}</button>
          <button v-if="hasRetryable" class="btn btn-primary" @click="emit('select-failed')">{{ t('admin.accounts.batchModelSync.selectFailed') }}</button>
        </template>
      </div>
    </template>
  </BaseDialog>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import BaseDialog from '@/components/common/BaseDialog.vue'
import type { AccountUpstreamModelSyncResult, UpstreamModelSyncWarning } from '@/api/admin/accounts'

type Row = Omit<AccountUpstreamModelSyncResult, 'status'> & {
  status: AccountUpstreamModelSyncResult['status'] | 'pending' | 'running' | 'unknown'
}
const props = defineProps<{ show: boolean; running: boolean; results: Row[] }>()
const emit = defineEmits<{ close: []; stop: []; 'select-failed': [] }>()
const { t } = useI18n()
const completed = computed(() => props.results.filter(row => row.status !== 'pending' && row.status !== 'running').length)
const counts = computed(() => ({
  success: props.results.filter(row => row.status === 'success').length,
  warning: props.results.filter(row => row.status === 'warning').length,
  failed: props.results.filter(row => row.status === 'failed').length,
  unsupported: props.results.filter(row => row.status === 'unsupported').length
}))
const hasRetryable = computed(() => props.results.some(row => ['failed', 'unknown', 'canceled'].includes(row.status)))
const statusClass = (status: Row['status']) => {
  if (status === 'success') return 'text-emerald-600 dark:text-emerald-400'
  if (status === 'failed') return 'text-rose-600 dark:text-rose-400'
  if (status === 'warning' || status === 'unknown') return 'text-amber-600 dark:text-amber-400'
  return 'text-gray-500 dark:text-gray-400'
}
const warningMessage = (warning: UpstreamModelSyncWarning) => {
  if (warning.code === 'upstream_model_metadata_incomplete') return t('admin.accounts.syncUpstreamModelsMetadataIncomplete')
  if (warning.code === 'upstream_model_metadata_partial') return t('admin.accounts.syncUpstreamModelsMetadataPartial')
  if (warning.code === 'upstream_model_list_unavailable') return t('admin.accounts.batchModelSync.listUnavailable')
  return warning.message
}
</script>
