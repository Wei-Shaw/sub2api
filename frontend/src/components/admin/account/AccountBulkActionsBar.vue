<template>
  <div class="mb-4 flex items-center justify-between rounded-lg bg-primary-50 p-3 dark:bg-primary-900/20">
    <div class="flex flex-wrap items-center gap-2">
      <span v-if="selectedIds.length > 0" class="text-sm font-medium text-primary-900 dark:text-primary-100">
        {{ t('admin.accounts.bulkActions.selected', { count: selectedIds.length }) }}
      </span>
      <span v-else class="text-sm font-medium text-primary-900 dark:text-primary-100">
        {{ t('admin.accounts.bulkEdit.title') }}
      </span>
      <template v-if="selectedIds.length > 0">
        <button
          class="text-xs font-medium text-primary-700 hover:text-primary-800 dark:text-primary-300 dark:hover:text-primary-200"
          @click="$emit('select-page')"
        >
          {{ t('admin.accounts.bulkActions.selectCurrentPage') }}
        </button>
        <span class="text-gray-300 dark:text-primary-800">•</span>
        <button
          class="text-xs font-medium text-primary-700 hover:text-primary-800 dark:text-primary-300 dark:hover:text-primary-200"
          @click="$emit('clear')"
        >
          {{ t('admin.accounts.bulkActions.clear') }}
        </button>
      </template>
    </div>

    <div class="flex flex-wrap gap-2">
      <template v-if="selectedIds.length > 0">
        <button class="btn btn-secondary btn-sm" @click="$emit('batch-test')">
          {{ t('admin.accounts.bulkActions.batchTest') }}
        </button>
        <button class="btn btn-secondary btn-sm" @click="$emit('join-health-pool')">
          {{ t('admin.accounts.bulkActions.joinHealthPool') }}
        </button>
        <button class="btn btn-secondary btn-sm" @click="$emit('leave-health-pool')">
          {{ t('admin.accounts.bulkActions.leaveHealthPool') }}
        </button>
        <button class="btn btn-secondary btn-sm" @click="$emit('protect-health')">
          {{ t('admin.accounts.bulkActions.protectHealth') }}
        </button>
        <button class="btn btn-danger btn-sm" @click="$emit('delete')">
          {{ t('admin.accounts.bulkActions.delete') }}
        </button>
        <button class="btn btn-secondary btn-sm" @click="$emit('reset-status')">
          {{ t('admin.accounts.bulkActions.resetStatus') }}
        </button>
        <button class="btn btn-secondary btn-sm" @click="$emit('refresh-token')">
          {{ t('admin.accounts.bulkActions.refreshToken') }}
        </button>
        <button class="btn btn-success btn-sm" @click="$emit('toggle-schedulable', true)">
          {{ t('admin.accounts.bulkActions.enableScheduling') }}
        </button>
        <button class="btn btn-warning btn-sm" @click="$emit('toggle-schedulable', false)">
          {{ t('admin.accounts.bulkActions.disableScheduling') }}
        </button>
        <button class="btn btn-primary btn-sm" @click="$emit('edit-selected')">
          {{ t('admin.accounts.bulkActions.edit') }}
        </button>
      </template>
      <button class="btn btn-primary btn-sm" @click="$emit('edit-filtered')">
        {{ t('admin.accounts.bulkEdit.submit') }}
      </button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'

defineProps<{
  selectedIds: number[]
}>()

defineEmits<{
  (e: 'delete'): void
  (e: 'edit-selected'): void
  (e: 'edit-filtered'): void
  (e: 'clear'): void
  (e: 'select-page'): void
  (e: 'toggle-schedulable', value: boolean): void
  (e: 'reset-status'): void
  (e: 'refresh-token'): void
  (e: 'batch-test'): void
  (e: 'join-health-pool'): void
  (e: 'leave-health-pool'): void
  (e: 'protect-health'): void
}>()

const { t } = useI18n()
</script>
