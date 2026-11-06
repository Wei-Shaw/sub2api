<template>
  <section class="mt-4 space-y-3 border-t border-dashed border-gray-200 pt-4 dark:border-dark-700" data-testid="image-price-expressions">
    <h4 class="text-sm font-medium text-gray-700 dark:text-gray-300">
      {{ t('admin.groups.imageExpressions.title') }}
    </h4>
    <p class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.groups.imageExpressions.description') }}</p>
    <p class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.groups.imageExpressions.totalHint') }}</p>
    <p class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.groups.imageExpressions.scopeHint') }}</p>
    <p class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.groups.imageExpressions.parametersHint') }}</p>

    <div v-for="(row, index) in modelValue" :key="index" class="space-y-2 rounded-lg border border-gray-200 p-3 dark:border-dark-700" data-testid="image-expression-row">
      <label class="block">
        <span class="input-label">{{ t('admin.groups.imageExpressions.model') }}</span>
        <input
          :value="row.model"
          type="text"
          class="input font-mono"
          data-testid="image-expression-model"
          :placeholder="grokImageExpressionModel"
          spellcheck="false"
          @input="updateRow(index, 'model', ($event.target as HTMLInputElement).value)"
        />
      </label>
      <label class="block">
        <span class="input-label">{{ t('admin.groups.imageExpressions.expression') }}</span>
        <textarea
          :value="row.expression"
          class="input min-h-32 font-mono text-xs"
          data-testid="image-expression-source"
          rows="6"
          spellcheck="false"
          placeholder='tier("base", fixed(0.04)) * image_count'
          @input="updateRow(index, 'expression', ($event.target as HTMLTextAreaElement).value)"
        />
      </label>
      <button type="button" class="btn btn-secondary text-xs" data-testid="image-expression-remove" @click="removeRow(index)">
        {{ t('admin.groups.imageExpressions.remove') }}
      </button>
    </div>

    <p v-if="validationError" role="alert" class="text-xs text-red-600 dark:text-red-400">
      {{ t(`admin.groups.imageExpressions.${validationError}`) }}
    </p>
    <div class="flex flex-wrap gap-2">
      <button type="button" class="btn btn-secondary text-xs" data-testid="image-expression-add" @click="addRow">
        {{ t('admin.groups.imageExpressions.add') }}
      </button>
      <button type="button" class="btn btn-secondary text-xs" data-testid="image-expression-preset" :disabled="hasGrokPreset" @click="addGrokPreset">
        {{ t('admin.groups.imageExpressions.grokPreset') }}
      </button>
    </div>
    <p class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.groups.imageExpressions.grokPresetHint') }}</p>
    <p class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.groups.imageExpressions.validationHint') }}</p>
  </section>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import {
  grokImageExpressionModel,
  grokImageExpressionPreset,
  validateImagePriceExpressionRows,
  type ImagePriceExpressionRow
} from './imagePriceExpressions'

const props = defineProps<{ modelValue: ImagePriceExpressionRow[] }>()
const emit = defineEmits<{ 'update:modelValue': [value: ImagePriceExpressionRow[]] }>()
const { t } = useI18n()
const validationError = computed(() => validateImagePriceExpressionRows(props.modelValue))
const hasGrokPreset = computed(() => props.modelValue.some(row => row.model === grokImageExpressionModel))

function updateRow(index: number, field: keyof ImagePriceExpressionRow, value: string) {
  emit('update:modelValue', props.modelValue.map((row, i) => i === index ? { ...row, [field]: value } : row))
}

function addRow() {
  emit('update:modelValue', [...props.modelValue, { model: '', expression: '' }])
}

function removeRow(index: number) {
  emit('update:modelValue', props.modelValue.filter((_, i) => i !== index))
}

function addGrokPreset() {
  if (hasGrokPreset.value) return
  emit('update:modelValue', [...props.modelValue, { model: grokImageExpressionModel, expression: grokImageExpressionPreset }])
}
</script>
