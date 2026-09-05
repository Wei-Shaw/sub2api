<template>
  <div v-if="rows.length > 0" class="space-y-2">
    <div
      v-for="(row, index) in rows"
      :key="getHeaderOverrideRowKey(row)"
      class="flex items-center gap-2"
    >
      <input
        v-model="row.name"
        type="text"
        class="input flex-1"
        :placeholder="t('admin.accounts.headerOverride.namePlaceholder')"
      />
      <input
        v-model="row.value"
        type="text"
        class="input flex-1"
        :placeholder="t('admin.accounts.headerOverride.valuePlaceholder')"
      />
      <button
        type="button"
        class="rounded-lg p-2 text-red-500 transition-colors hover:bg-red-50 hover:text-red-600 dark:hover:bg-red-900/20"
        @click="removeRow(index)"
      >
        <svg class="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
          <path
            stroke-linecap="round"
            stroke-linejoin="round"
            stroke-width="2"
            d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16"
          />
        </svg>
      </button>
    </div>
  </div>

  <button
    type="button"
    class="w-full rounded-lg border-2 border-dashed border-gray-300 px-4 py-2 text-gray-600 transition-colors hover:border-gray-400 hover:text-gray-700 dark:border-dark-500 dark:text-gray-400 dark:hover:border-dark-400 dark:hover:text-gray-300"
    @click="addRow"
  >
    <svg class="mr-1 inline h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
      <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 4v16m8-8H4" />
    </svg>
    {{ t('admin.accounts.headerOverride.addRow') }}
  </button>

  <div class="flex flex-wrap gap-2">
    <HeaderOverrideJsonTools :rows="rows" @update:rows="emit('update:rows', $event)" />
  </div>

  <p class="text-xs text-gray-500 dark:text-gray-400">
    {{ t('admin.accounts.headerOverride.emptyValueHint') }}
  </p>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { createStableObjectKeyResolver } from '@/utils/stableObjectKey'
import HeaderOverrideJsonTools from './HeaderOverrideJsonTools.vue'
import type { HeaderOverrideRow } from './credentialsBuilder'

const props = defineProps<{
  rows: HeaderOverrideRow[]
}>()

const emit = defineEmits<{
  (e: 'update:rows', rows: HeaderOverrideRow[]): void
}>()

const { t } = useI18n()

const getHeaderOverrideRowKey = createStableObjectKeyResolver<HeaderOverrideRow>(
  'header-override-row'
)

const addRow = () => {
  emit('update:rows', [...props.rows, { name: '', value: '' }])
}

const removeRow = (index: number) => {
  emit('update:rows', props.rows.filter((_, i) => i !== index))
}
</script>
