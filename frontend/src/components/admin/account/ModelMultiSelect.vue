<template>
  <div ref="container" class="relative">
    <button
      :id="id"
      type="button"
      class="select-trigger w-full"
      :aria-label="ariaLabel"
      :aria-expanded="open"
      :aria-controls="id ? `${id}-options` : undefined"
      @click="open = !open"
    >
      <span class="select-value truncate text-left">{{ summary }}</span>
      <Icon name="chevronDown" size="md" :class="['transition-transform duration-200', open && 'rotate-180']" />
    </button>

    <div
      v-if="open"
      :id="id ? `${id}-options` : undefined"
      class="absolute z-20 mt-1 max-h-64 w-full overflow-auto rounded-lg border border-gray-200 bg-white p-2 shadow-lg dark:border-dark-600 dark:bg-dark-800"
      role="group"
      :aria-label="ariaLabel"
      @click.stop
    >
      <div class="mb-2 flex items-center justify-between gap-2 border-b border-gray-100 pb-2 text-xs dark:border-dark-700">
        <button type="button" class="text-primary-600 hover:underline dark:text-primary-400" @click="selectAll">
          {{ allLabel }}
        </button>
        <button type="button" class="text-gray-500 hover:underline dark:text-gray-400" @click="clearAll">
          {{ clearLabel }}
        </button>
      </div>
      <label
        v-for="option in options"
        :key="String(option.value)"
        class="flex cursor-pointer items-center gap-2 rounded px-2 py-1.5 text-sm text-gray-700 hover:bg-gray-50 dark:text-gray-300 dark:hover:bg-dark-700"
      >
        <input
          type="checkbox"
          :checked="selected.has(String(option.value))"
          :disabled="option.disabled"
          @change="toggle(String(option.value))"
        />
        <span class="truncate">{{ option.label }}</span>
      </label>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { Icon } from '@/components/icons'
import type { SelectOption } from '@/components/common/Select.vue'

const { t } = useI18n()

const props = withDefaults(defineProps<{
  modelValue: string[]
  options: SelectOption[]
  id?: string
  ariaLabel?: string
  placeholder?: string
  allLabel?: string
  clearLabel?: string
}>(), {
  placeholder: '',
  id: undefined,
  ariaLabel: undefined,
  allLabel: '',
  clearLabel: ''
})

const emit = defineEmits<{
  (e: 'update:modelValue', value: string[]): void
}>()

const open = ref(false)
const container = ref<HTMLElement | null>(null)
const selected = computed(() => new Set(props.modelValue.map(String)))
const availableValues = computed(() => props.options.map(option => String(option.value)))
const summary = computed(() => {
  if (selected.value.size === 0) return props.placeholder || t('common.selectOption')
  if (selected.value.size === availableValues.value.length && availableValues.value.length > 0) {
    return props.allLabel || t('admin.scheduledTests.allFailedModels')
  }
  return t('admin.scheduledTests.selectedModels', { count: selected.value.size })
})

const selectAll = () => emit('update:modelValue', [...availableValues.value])
const clearAll = () => emit('update:modelValue', [])
const toggle = (modelID: string) => {
  const next = new Set(selected.value)
  if (next.has(modelID)) next.delete(modelID)
  else next.add(modelID)
  emit('update:modelValue', availableValues.value.filter(value => next.has(value)))
}

const onDocumentClick = (event: MouseEvent) => {
  if (container.value && !container.value.contains(event.target as Node)) open.value = false
}

onMounted(() => document.addEventListener('click', onDocumentClick))
onUnmounted(() => document.removeEventListener('click', onDocumentClick))
</script>
