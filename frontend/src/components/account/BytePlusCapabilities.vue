<template>
  <div class="space-y-2 border-t border-gray-200 pt-4 dark:border-dark-600" data-testid="byteplus-capabilities">
    <label class="input-label">{{ t('admin.accounts.openai.endpointCapabilities') }}</label>
    <div class="grid grid-cols-1 gap-2 sm:grid-cols-2">
      <label
        v-for="option in options"
        :key="option.value"
        class="flex cursor-pointer items-center gap-2 rounded-lg border border-gray-200 px-3 py-2 text-sm dark:border-dark-600"
      >
        <input
          type="checkbox"
          :data-testid="`byteplus-capability-${option.value}`"
          :checked="modelValue.includes(option.value)"
          :disabled="modelValue.length === 1 && modelValue.includes(option.value)"
          class="rounded border-gray-300 text-primary-600 focus:ring-primary-500 dark:border-dark-500"
          @change="toggle(option.value)"
        />
        <span>{{ t(option.label) }}</span>
      </label>
    </div>
    <p class="input-hint">{{ t('admin.accounts.byteplus.regionHint') }}</p>
  </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import type { OpenAIEndpointCapability } from '@/types'

const props = defineProps<{ modelValue: OpenAIEndpointCapability[] }>()
const emit = defineEmits<{ 'update:modelValue': [value: OpenAIEndpointCapability[]] }>()
const { t } = useI18n()
const options = [
  { value: 'chat_completions', label: 'admin.accounts.byteplus.textCapability' },
  { value: 'seedance', label: 'admin.accounts.byteplus.videoCapability' }
] as const

function toggle(capability: OpenAIEndpointCapability) {
  const selected = props.modelValue.includes(capability)
  if (selected && props.modelValue.length === 1) return
  emit('update:modelValue', selected
    ? props.modelValue.filter(value => value !== capability)
    : [...props.modelValue, capability])
}
</script>
