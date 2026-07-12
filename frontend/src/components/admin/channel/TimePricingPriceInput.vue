<template>
  <label class="block">
    <span class="text-xs text-gray-400">{{ label }} <span class="font-normal">{{ unit }}</span></span>
    <input
      :value="value"
      type="number"
      step="any"
      min="0"
      class="input mt-0.5 text-sm"
      :placeholder="t('admin.channels.form.inheritDefault')"
      @input="emit('update', normalize(($event.target as HTMLInputElement).value))"
    />
  </label>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'

withDefaults(defineProps<{
  label: string
  value: number | string | null
  unit?: string
}>(), {
  unit: '$/MTok',
})

const emit = defineEmits<{
  update: [value: string | null]
}>()
const { t } = useI18n()

function normalize(value: string): string | null {
  return value === '' ? null : value
}
</script>
