<template>
  <label class="block">
    <span class="text-xs font-semibold text-gray-600 dark:text-dark-300">{{ label }}</span>
    <div class="relative mt-1.5">
      <span class="absolute left-3 top-1/2 -translate-y-1/2 text-sm font-semibold text-gray-400">
        {{ currency === 'CNY' ? '¥' : '$' }}
      </span>
      <input
        :value="modelValue ?? ''"
        type="number"
        min="0"
        step="any"
        :required="required"
        :placeholder="placeholder"
        class="input pl-8 font-mono"
        @input="handleInput"
      />
    </div>
  </label>
</template>

<script setup lang="ts">
import type { DisplayPriceCurrency } from '@/api/modelPrices'

withDefaults(
  defineProps<{
    modelValue: number | null
    label: string
    currency: DisplayPriceCurrency
    placeholder?: string
    required?: boolean
  }>(),
  {
    placeholder: '',
    required: false
  }
)

const emit = defineEmits<{ 'update:modelValue': [value: number | null] }>()

function handleInput(event: Event): void {
  const raw = (event.target as HTMLInputElement).value
  emit('update:modelValue', raw === '' ? null : Number(raw))
}
</script>
