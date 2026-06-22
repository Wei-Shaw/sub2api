<template>
  <div class="border-t border-gray-200 pt-4 dark:border-dark-600">
    <div class="mb-3 flex items-center justify-between gap-3">
      <div>
        <label class="input-label mb-0">{{ t('admin.accounts.openai.customHeaders') }}</label>
        <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
          {{ t('admin.accounts.openai.customHeadersDesc') }}
        </p>
      </div>
      <button type="button" class="btn btn-secondary shrink-0 text-sm" @click="addRow">
        <Icon name="plus" size="sm" class="mr-1" :stroke-width="2" />
        {{ t('admin.accounts.openai.addCustomHeader') }}
      </button>
    </div>

    <div v-if="rows.length > 0" class="space-y-2">
      <div
        v-for="(row, index) in rows"
        :key="index"
        class="grid grid-cols-1 gap-2 sm:grid-cols-[minmax(0,1fr)_minmax(0,1fr)_2.5rem]"
      >
        <input
          :value="row.name"
          type="text"
          class="input font-mono text-sm"
          :class="rowError(index) ? 'border-red-300 focus:border-red-500 focus:ring-red-500 dark:border-red-700' : ''"
          :placeholder="t('admin.accounts.openai.customHeaderNamePlaceholder')"
          @input="updateRow(index, 'name', $event)"
        />
        <input
          :value="row.value"
          type="text"
          class="input font-mono text-sm"
          :class="rowError(index) ? 'border-red-300 focus:border-red-500 focus:ring-red-500 dark:border-red-700' : ''"
          :placeholder="t('admin.accounts.openai.customHeaderValuePlaceholder')"
          @input="updateRow(index, 'value', $event)"
        />
        <button
          type="button"
          class="flex h-10 items-center justify-center rounded-md text-red-500 transition-colors hover:bg-red-50 hover:text-red-600 dark:hover:bg-red-900/20"
          @click="removeRow(index)"
        >
          <Icon name="trash" size="sm" :stroke-width="2" />
        </button>
        <p
          v-if="rowError(index)"
          class="text-xs text-red-600 dark:text-red-400 sm:col-span-3"
        >
          {{ t(rowErrorKey(index)) }}
        </p>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import type { OpenAICustomHeader } from '@/types'
import Icon from '@/components/icons/Icon.vue'
import {
  getOpenAICustomHeaderRowError,
  type OpenAICustomHeaderRowError
} from '@/utils/openaiCustomHeaders'

const props = defineProps<{
  modelValue: OpenAICustomHeader[]
}>()

const emit = defineEmits<{
  'update:modelValue': [value: OpenAICustomHeader[]]
}>()

const { t } = useI18n()

const rows = computed(() => props.modelValue || [])

const emitRows = (value: OpenAICustomHeader[]) => {
  emit('update:modelValue', value)
}

const addRow = () => {
  emitRows([...rows.value, { name: '', value: '' }])
}

const removeRow = (index: number) => {
  emitRows(rows.value.filter((_, rowIndex) => rowIndex !== index))
}

const updateRow = (index: number, field: keyof OpenAICustomHeader, event: Event) => {
  const target = event.target as HTMLInputElement | null
  const next = rows.value.map((row, rowIndex) =>
    rowIndex === index ? { ...row, [field]: target?.value || '' } : row
  )
  emitRows(next)
}

const rowError = (index: number): OpenAICustomHeaderRowError | null =>
  getOpenAICustomHeaderRowError(rows.value[index], rows.value)

const rowErrorKey = (index: number) => {
  const error = rowError(index)
  switch (error) {
    case 'incomplete':
      return 'admin.accounts.openai.customHeaderIncomplete'
    case 'invalid':
      return 'admin.accounts.openai.customHeaderInvalid'
    case 'protected':
      return 'admin.accounts.openai.customHeaderProtected'
    case 'duplicate':
      return 'admin.accounts.openai.customHeaderDuplicate'
    default:
      return ''
  }
}
</script>
