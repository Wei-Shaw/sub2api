<template>
  <BaseDialog
    :show="show"
    :title="t('admin.accounts.batchTestResults.title')"
    width="wide"
    @close="handleClose"
  >
    <div v-if="result" class="space-y-4">
      <div v-if="running" class="space-y-2">
        <div class="flex items-center justify-between text-sm text-gray-600 dark:text-gray-300">
          <span>{{ t('admin.accounts.bulkActions.batchTesting') }}</span>
          <span>{{ completed }} / {{ result.total }}</span>
        </div>
        <div class="h-2 overflow-hidden rounded-full bg-gray-100 dark:bg-dark-700">
          <div class="h-full bg-primary-500 transition-all" :style="{ width: `${progressPercent}%` }"></div>
        </div>
      </div>

      <div class="grid grid-cols-2 gap-3 md:grid-cols-4">
        <div class="rounded-lg border border-gray-200 bg-gray-50 p-3 dark:border-dark-600 dark:bg-dark-700">
          <div class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.accounts.batchTestResults.total') }}</div>
          <div class="text-xl font-semibold text-gray-900 dark:text-gray-100">{{ result.total }}</div>
        </div>
        <div class="rounded-lg border border-green-200 bg-green-50 p-3 dark:border-green-800 dark:bg-green-900/20">
          <div class="text-xs text-green-700 dark:text-green-300">{{ t('admin.accounts.batchTestResults.success') }}</div>
          <div class="text-xl font-semibold text-green-700 dark:text-green-300">{{ result.success }}</div>
        </div>
        <div class="rounded-lg border border-red-200 bg-red-50 p-3 dark:border-red-800 dark:bg-red-900/20">
          <div class="text-xs text-red-700 dark:text-red-300">{{ t('admin.accounts.batchTestResults.failed') }}</div>
          <div class="text-xl font-semibold text-red-700 dark:text-red-300">{{ result.failed }}</div>
        </div>
        <div class="rounded-lg border border-amber-200 bg-amber-50 p-3 dark:border-amber-800 dark:bg-amber-900/20">
          <div class="text-xs text-amber-700 dark:text-amber-300">{{ t('admin.accounts.batchTestResults.unauthorized') }}</div>
          <div class="text-xl font-semibold text-amber-700 dark:text-amber-300">{{ result.unauthorized }}</div>
        </div>
      </div>

      <div class="max-h-[460px] overflow-auto rounded-lg border border-gray-200 dark:border-dark-600">
        <table class="min-w-full divide-y divide-gray-200 text-sm dark:divide-dark-600">
          <thead class="bg-gray-50 dark:bg-dark-700">
            <tr>
              <th class="px-3 py-2 text-left font-medium text-gray-600 dark:text-gray-300">
                {{ t('admin.accounts.batchTestResults.account') }}
              </th>
              <th class="px-3 py-2 text-left font-medium text-gray-600 dark:text-gray-300">
                {{ t('admin.accounts.batchTestResults.status') }}
              </th>
              <th class="px-3 py-2 text-left font-medium text-gray-600 dark:text-gray-300">
                {{ t('admin.accounts.batchTestResults.latency') }}
              </th>
              <th class="px-3 py-2 text-left font-medium text-gray-600 dark:text-gray-300">
                {{ t('admin.accounts.batchTestResults.message') }}
              </th>
            </tr>
          </thead>
          <tbody class="divide-y divide-gray-100 bg-white dark:divide-dark-600 dark:bg-dark-800">
            <tr v-for="item in result.results" :key="item.account_id">
              <td class="px-3 py-2 text-gray-900 dark:text-gray-100">
                <div class="font-medium">{{ accountName(item.account_id) }}</div>
                <div class="text-xs text-gray-500">#{{ item.account_id }}</div>
              </td>
              <td class="px-3 py-2">
                <span
                  :class="[
                    'inline-flex rounded-full px-2 py-0.5 text-xs font-medium',
                    item.success
                      ? 'bg-green-100 text-green-700 dark:bg-green-900/30 dark:text-green-300'
                      : 'bg-red-100 text-red-700 dark:bg-red-900/30 dark:text-red-300'
                  ]"
                >
                  {{ item.success ? t('admin.accounts.batchTestResults.ok') : t('admin.accounts.batchTestResults.error') }}
                </span>
              </td>
              <td class="px-3 py-2 text-gray-600 dark:text-gray-300">
                {{ item.latency_ms ? `${item.latency_ms}ms` : '-' }}
              </td>
              <td class="max-w-md px-3 py-2 text-gray-600 dark:text-gray-300">
                <div class="line-clamp-3 whitespace-pre-wrap break-words">
                  {{ item.error_message || item.response_text || '-' }}
                </div>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>

    <template #footer>
      <div class="flex justify-end gap-2">
        <button v-if="running" class="btn btn-secondary" @click="emit('cancel')">
          {{ t('common.cancel') }}
        </button>
        <button class="btn btn-primary" :disabled="running" @click="emit('close')">
          {{ t('common.close') }}
        </button>
      </div>
    </template>
  </BaseDialog>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import BaseDialog from '@/components/common/BaseDialog.vue'
import type { Account, BatchAccountTestResponse } from '@/types'

const props = defineProps<{
  show: boolean
  result: BatchAccountTestResponse | null
  accounts: Account[]
  running?: boolean
  completed?: number
}>()

const emit = defineEmits<{
  (e: 'close'): void
  (e: 'cancel'): void
}>()

const { t } = useI18n()

const accountMap = computed(() => new Map(props.accounts.map((account) => [account.id, account.name])))
const accountName = (accountID: number) => accountMap.value.get(accountID) || t('admin.accounts.batchTestResults.unknownAccount')
const completed = computed(() => props.completed ?? props.result?.results.length ?? 0)
const progressPercent = computed(() => {
  const total = props.result?.total ?? 0
  if (total <= 0) return 0
  return Math.min(100, Math.round((completed.value / total) * 100))
})

const handleClose = () => {
  if (props.running) {
    emit('cancel')
    return
  }
  emit('close')
}
</script>
