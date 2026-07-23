<template>
  <div class="mb-4 flex items-center justify-between rounded-lg bg-primary-50 p-3 dark:bg-primary-900/20">
    <div class="flex flex-wrap items-center gap-2">
      <span v-if="selectedIds.length > 0" class="text-sm font-medium text-primary-900 dark:text-primary-100">
        {{ t('admin.accounts.bulkActions.selected', { count: selectedIds.length }) }}
      </span>
      <span v-else class="text-sm font-medium text-primary-900 dark:text-primary-100">
        {{ t('admin.accounts.bulkEdit.title') }}
      </span>
      <button
        type="button"
        data-test="select-page"
        class="text-xs font-medium text-primary-700 hover:text-primary-800 dark:text-primary-300 dark:hover:text-primary-200"
        @click="$emit('select-page')"
      >
        {{ t('admin.accounts.bulkActions.selectCurrentPage') }}
      </button>
      <span class="text-gray-300 dark:text-primary-800">•</span>
      <button
        type="button"
        data-test="select-filtered"
        class="text-xs font-medium text-primary-700 hover:text-primary-800 disabled:cursor-not-allowed disabled:opacity-60 dark:text-primary-300 dark:hover:text-primary-200"
        :disabled="selectingFiltered"
        @click="$emit('select-filtered')"
      >
        {{ selectingFiltered ? t('admin.accounts.bulkActions.selectingFiltered') : t('admin.accounts.bulkActions.selectFiltered') }}
      </button>
      <template v-if="selectedIds.length > 0">
        <span class="text-gray-300 dark:text-primary-800">•</span>
        <button
          type="button"
          data-test="clear-selection"
          class="text-xs font-medium text-primary-700 hover:text-primary-800 dark:text-primary-300 dark:hover:text-primary-200"
          @click="$emit('clear')"
        >
          {{ t('admin.accounts.bulkActions.clear') }}
        </button>
      </template>
    </div>
    <div class="flex flex-wrap gap-2">
      <template v-if="selectedIds.length > 0">
        <button type="button" class="btn btn-danger btn-sm" @click="$emit('delete')">{{ t('admin.accounts.bulkActions.delete') }}</button>
        <button type="button" class="btn btn-secondary btn-sm" @click="$emit('reset-status')">{{ t('admin.accounts.bulkActions.resetStatus') }}</button>
        <button type="button" class="btn btn-secondary btn-sm" @click="$emit('refresh-token')">{{ t('admin.accounts.bulkActions.refreshToken') }}</button>
        <button type="button" class="btn btn-secondary btn-sm" @click="$emit('probe-upstream-billing')">{{ t('admin.accounts.bulkActions.probeUpstreamBilling') }}</button>
        <button type="button" class="btn btn-success btn-sm" @click="$emit('toggle-schedulable', true)">{{ t('admin.accounts.bulkActions.enableScheduling') }}</button>
        <button type="button" class="btn btn-warning btn-sm" @click="$emit('toggle-schedulable', false)">{{ t('admin.accounts.bulkActions.disableScheduling') }}</button>
        <button type="button" class="btn btn-primary btn-sm" @click="$emit('edit-selected')">{{ t('admin.accounts.bulkActions.edit') }}</button>
      </template>
      <button type="button" class="btn btn-primary btn-sm" @click="$emit('edit-filtered')">
        {{ t('admin.accounts.bulkEdit.submit') }}
      </button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'

withDefaults(defineProps<{ selectedIds: number[]; selectingFiltered?: boolean }>(), {
  selectingFiltered: false
})
defineEmits([
  'delete',
  'edit-selected',
  'edit-filtered',
  'clear',
  'select-page',
  'select-filtered',
  'toggle-schedulable',
  'reset-status',
  'refresh-token',
  'probe-upstream-billing'
])

const { t } = useI18n()
</script>
