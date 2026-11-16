<template>
  <div
    ref="root"
    role="radiogroup"
    :aria-label="t('admin.groups.openaiFast.policy')"
    class="grid min-w-0 grid-cols-3 gap-1 rounded-md border border-gray-200 bg-gray-50 p-1 dark:border-dark-400 dark:bg-dark-800"
    @keydown="handleKeydown"
  >
    <button
      v-for="option in options"
      :key="option.value"
      type="button"
      role="radio"
      :data-tier="option.value"
      :aria-checked="modelValue === option.value"
      :tabindex="modelValue === option.value ? 0 : -1"
      :disabled="disabled"
      :title="option.title"
      :class="[
        'min-h-9 min-w-0 break-words rounded px-2 py-2 text-center text-xs font-medium leading-tight transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary-500 disabled:cursor-not-allowed disabled:opacity-50',
        modelValue === option.value
          ? 'bg-white text-primary-700 shadow-sm ring-1 ring-gray-200 dark:bg-dark-600 dark:text-primary-300 dark:ring-dark-400'
          : 'text-gray-600 hover:bg-gray-100 dark:text-gray-400 dark:hover:bg-dark-700',
      ]"
      @click="emit('update:modelValue', option.value)"
    >
      {{ option.label }}
    </button>
  </div>
</template>

<script setup lang="ts">
import { computed, nextTick, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import type { OpenAIServiceTierPolicy } from '@/types'

const props = defineProps<{
  modelValue: OpenAIServiceTierPolicy
  disabled?: boolean
}>()
const emit = defineEmits<{ 'update:modelValue': [value: OpenAIServiceTierPolicy] }>()
const { t } = useI18n()
const root = ref<HTMLElement>()
const options = computed(() => [
  { value: 'client' as const, label: t('admin.groups.openaiFast.client'), title: t('admin.groups.openaiFast.client') },
  { value: 'priority' as const, label: t('admin.groups.openaiFast.forceFastOption'), title: t('admin.groups.openaiFast.force') },
  { value: 'ultrafast' as const, label: t('admin.groups.openaiFast.forceUltrafast'), title: t('admin.groups.openaiFast.forceUltrafast') },
])

async function handleKeydown(event: KeyboardEvent) {
  if (props.disabled) return
  const index = options.value.findIndex(option => option.value === props.modelValue)
  let next = index
  if (event.key === 'ArrowRight' || event.key === 'ArrowDown') next = (index + 1) % options.value.length
  else if (event.key === 'ArrowLeft' || event.key === 'ArrowUp') next = (index + options.value.length - 1) % options.value.length
  else if (event.key === 'Home') next = 0
  else if (event.key === 'End') next = options.value.length - 1
  else return
  event.preventDefault()
  const value = options.value[next].value
  emit('update:modelValue', value)
  await nextTick()
  root.value?.querySelector<HTMLButtonElement>(`[data-tier="${value}"]`)?.focus()
}
</script>
