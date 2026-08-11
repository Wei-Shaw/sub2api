<!-- 按 SchemaRow 约束编辑一份 JSON 值，用于 array 的多个默认元素。 -->
<template>
  <div class="schema-value-editor">
    <input
      v-if="schema.type === 'string'"
      :value="stringValue"
      type="text"
      class="input h-8 text-xs"
      :placeholder="t('admin.modelIntros.fields.arrayDefaultPlaceholder')"
      @input="updateString(($event.target as HTMLInputElement).value)"
    />
    <input
      v-else-if="schema.type === 'number'"
      :value="numberValue"
      type="number"
      class="input h-8 text-xs"
      @input="updateNumber(($event.target as HTMLInputElement).value)"
    />
    <label
      v-else-if="schema.type === 'boolean'"
      class="inline-flex h-8 items-center gap-2 rounded-xl border border-gray-200 px-3 dark:border-dark-600"
    >
      <input
        :checked="modelValue === true"
        type="checkbox"
        class="h-4 w-4"
        @change="emit('update:modelValue', ($event.target as HTMLInputElement).checked)"
      />
      <span class="text-xs text-gray-500">{{ modelValue === true ? 'true' : 'false' }}</span>
    </label>

    <div
      v-else-if="schema.type === 'object'"
      class="space-y-2 rounded border border-gray-200 bg-gray-50/60 p-2 dark:border-dark-700 dark:bg-dark-800/40"
    >
      <div v-for="child in schema.children" :key="child.uid" class="grid gap-1 sm:grid-cols-[10rem_1fr] sm:items-center">
        <span class="truncate font-mono text-[11px] text-gray-500">{{ child.key }}</span>
        <SchemaValueEditor
          :schema="child"
          :model-value="objectValue[child.key]"
          @update:model-value="updateObjectChild(child.key, $event)"
        />
      </div>
    </div>

    <div
      v-else-if="schema.type === 'array'"
      class="space-y-2 rounded border border-gray-200 bg-gray-50/60 p-2 dark:border-dark-700 dark:bg-dark-800/40"
    >
      <div v-for="(value, index) in arrayValue" :key="index" class="flex items-start gap-2">
        <span class="mt-2 w-8 shrink-0 font-mono text-[11px] text-gray-400">[{{ index }}]</span>
        <SchemaValueEditor
          v-if="schema.items"
          class="min-w-0 flex-1"
          :schema="schema.items"
          :model-value="value"
          @update:model-value="updateArrayItem(index, $event)"
        />
        <button
          type="button"
          class="btn btn-ghost btn-xs shrink-0 text-red-500"
          :title="t('common.remove')"
          @click="removeArrayItem(index)"
        >
          <Icon name="trash" size="xs" />
        </button>
      </div>
      <button
        type="button"
        class="btn btn-secondary btn-xs"
        :disabled="!canAddNestedItem"
        @click="addArrayItem"
      >
        <Icon name="plus" size="xs" />
        {{ t('admin.modelIntros.fields.addArrayItem') }}
      </button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
import type { SchemaRow } from './paramSchemaRow'
import { defaultValueForSchemaRow } from './paramSchemaRow'

defineOptions({ name: 'SchemaValueEditor' })

const props = defineProps<{ schema: SchemaRow; modelValue: unknown }>()
const emit = defineEmits<{ (e: 'update:modelValue', value: unknown): void }>()
const { t } = useI18n()

const stringValue = computed(() => props.modelValue == null ? '' : String(props.modelValue))
const numberValue = computed(() => typeof props.modelValue === 'number' ? String(props.modelValue) : '')
const objectValue = computed<Record<string, unknown>>(() =>
  props.modelValue && typeof props.modelValue === 'object' && !Array.isArray(props.modelValue)
    ? props.modelValue as Record<string, unknown>
    : {}
)
const arrayValue = computed<unknown[]>(() => Array.isArray(props.modelValue) ? props.modelValue : [])
const canAddNestedItem = computed(() =>
  props.schema.maxItems <= 0 || arrayValue.value.length < props.schema.maxItems
)

function updateString(value: string) {
  emit('update:modelValue', value)
}

function updateNumber(value: string) {
  if (value.trim() === '') {
    emit('update:modelValue', 0)
    return
  }
  const parsed = Number(value)
  emit('update:modelValue', Number.isFinite(parsed) ? parsed : 0)
}

function updateObjectChild(key: string, value: unknown) {
  emit('update:modelValue', { ...objectValue.value, [key]: value })
}

function updateArrayItem(index: number, value: unknown) {
  const next = [...arrayValue.value]
  next[index] = value
  emit('update:modelValue', next)
}

function removeArrayItem(index: number) {
  const next = [...arrayValue.value]
  next.splice(index, 1)
  emit('update:modelValue', next)
}

function addArrayItem() {
  if (!props.schema.items || !canAddNestedItem.value) return
  emit('update:modelValue', [...arrayValue.value, defaultValueForSchemaRow(props.schema.items)])
}
</script>

<style scoped>
.schema-value-editor {
  width: 100%;
}
</style>
