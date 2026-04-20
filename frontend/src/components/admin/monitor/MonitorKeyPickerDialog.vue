<template>
  <BaseDialog
    :show="show"
    :title="t('admin.channelMonitor.form.selectKeyTitle')"
    width="normal"
    @close="$emit('close')"
  >
    <div class="space-y-3">
      <p class="text-xs text-gray-500 dark:text-gray-400">
        {{ t('admin.channelMonitor.form.selectKeyHint') }}
      </p>
      <div v-if="loading" class="py-6 text-center text-sm text-gray-500">
        {{ t('common.loading') }}
      </div>
      <div v-else-if="keys.length === 0" class="py-6 text-center text-sm text-gray-500">
        {{ t('admin.channelMonitor.form.noActiveKey') }}
      </div>
      <div v-else class="max-h-72 space-y-1 overflow-auto">
        <button
          v-for="k in keys"
          :key="k.id"
          type="button"
          @click="$emit('pick', k)"
          class="block w-full rounded-lg border border-gray-200 px-3 py-2 text-left text-sm transition-colors hover:bg-gray-50 dark:border-dark-600 dark:hover:bg-dark-700"
        >
          <div class="font-medium text-gray-900 dark:text-white">{{ k.name }}</div>
          <div class="font-mono text-xs text-gray-500 dark:text-gray-400">{{ maskKey(k.key) }}</div>
        </button>
      </div>
    </div>
    <template #footer>
      <div class="flex justify-end">
        <button @click="$emit('close')" class="btn btn-secondary">
          {{ t('common.cancel') }}
        </button>
      </div>
    </template>
  </BaseDialog>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import type { ApiKey } from '@/types'
import BaseDialog from '@/components/common/BaseDialog.vue'

defineProps<{
  show: boolean
  loading: boolean
  keys: ApiKey[]
}>()

defineEmits<{
  (e: 'close'): void
  (e: 'pick', key: ApiKey): void
}>()

const { t } = useI18n()

function maskKey(key: string): string {
  if (!key) return ''
  if (key.length <= 12) return `${key.slice(0, 4)}***`
  return `${key.slice(0, 6)}...${key.slice(-4)}`
}
</script>
