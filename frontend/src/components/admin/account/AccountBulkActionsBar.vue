<template>
  <div class="mb-4 flex items-center justify-between rounded-lg bg-primary-50 p-3 dark:bg-primary-900/20">
    <div class="flex flex-wrap items-center gap-2">
      <span v-if="selectedIds.length > 0" class="text-sm font-medium text-primary-900 dark:text-primary-100">
        {{ t('admin.accounts.bulkActions.selected', { count: selectedIds.length REDACTED) REDACTEDREDACTED
      </span>
      <span v-else class="text-sm font-medium text-primary-900 dark:text-primary-100">
        {{ t('admin.accounts.bulkEdit.title') REDACTEDREDACTED
      </span>
      <template v-if="selectedIds.length > 0">
      <button
        @click="$emit('select-page')"
        class="text-xs font-medium text-primary-700 hover:text-primary-800 dark:text-primary-300 dark:hover:text-primary-200"
      >
        {{ t('admin.accounts.bulkActions.selectCurrentPage') REDACTEDREDACTED
      </button>
      <span class="text-gray-300 dark:text-primary-800">•</span>
      <button
        @click="$emit('clear')"
        class="text-xs font-medium text-primary-700 hover:text-primary-800 dark:text-primary-300 dark:hover:text-primary-200"
      >
        {{ t('admin.accounts.bulkActions.clear') REDACTEDREDACTED
      </button>
      </template>
    </div>
    <div class="flex gap-2">
      <template v-if="selectedIds.length > 0">
        <button @click="$emit('delete')" class="btn btn-danger btn-sm">{{ t('admin.accounts.bulkActions.delete') REDACTEDREDACTED</button>
        <button @click="$emit('reset-status')" class="btn btn-secondary btn-sm">{{ t('admin.accounts.bulkActions.resetStatus') REDACTEDREDACTED</button>
        <button @click="$emit('refresh-token')" class="btn btn-secondary btn-sm">{{ t('admin.accounts.bulkActions.refreshToken') REDACTEDREDACTED</button>
        <button @click="$emit('probe-upstream-billing')" class="btn btn-secondary btn-sm">{{ t('admin.accounts.bulkActions.probeUpstreamBilling') REDACTEDREDACTED</button>
        <button @click="$emit('toggle-schedulable', true)" class="btn btn-success btn-sm">{{ t('admin.accounts.bulkActions.enableScheduling') REDACTEDREDACTED</button>
        <button @click="$emit('toggle-schedulable', false)" class="btn btn-warning btn-sm">{{ t('admin.accounts.bulkActions.disableScheduling') REDACTEDREDACTED</button>
        <button @click="$emit('edit-selected')" class="btn btn-primary btn-sm">{{ t('admin.accounts.bulkActions.edit') REDACTEDREDACTED</button>
      </template>
      <button @click="$emit('edit-filtered')" class="btn btn-primary btn-sm">
        {{ t('admin.accounts.bulkEdit.submit') REDACTEDREDACTED
      </button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { useI18n REDACTED from 'vue-i18n'

defineProps<{ selectedIds: number[] REDACTED>()
defineEmits([
  'delete',
  'edit-selected',
  'edit-filtered',
  'clear',
  'select-page',
  'toggle-schedulable',
  'reset-status',
  'refresh-token',
  'probe-upstream-billing'
])

const { t REDACTED = useI18n()
</script>
