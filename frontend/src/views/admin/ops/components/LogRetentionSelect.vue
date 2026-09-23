<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import Select from '@/components/common/Select.vue'

const props = defineProps<{
  modelValue: number
  label: string
  allowForever?: boolean
}>()
const emit = defineEmits<{ 'update:modelValue': [value: number] }>()
const { t } = useI18n()
const presets = [1, 3, 7, 14, 30, 60, 90, 180, 365]
const custom = ref(false)
const options = computed(() => [
  ...(props.allowForever ? [{ value: 0, label: t('admin.ops.systemLogs.retentionForever') }] : []),
  ...presets.map(days => ({ value: days, label: t('admin.ops.systemLogs.retentionDaysOption', { days }) })),
  { value: 'custom', label: t('admin.ops.systemLogs.retentionDaysCustom') }
])
const selection = computed(() => custom.value || !(presets.includes(props.modelValue) || (props.allowForever && props.modelValue === 0)) ? 'custom' : props.modelValue)

function select(value: string | number | boolean | null | undefined) {
  custom.value = value === 'custom'
  if (typeof value === 'number') emit('update:modelValue', value)
}
</script>

<template>
  <div class="space-y-2">
    <Select :model-value="selection" :options="options" :aria-label="label" @update:model-value="select" />
    <input
      v-if="selection === 'custom'"
      :value="modelValue"
      type="number"
      min="1"
      max="3650"
      step="1"
      class="input"
      :aria-label="`${label}: ${t('admin.ops.systemLogs.retentionDaysCustom')}`"
      @input="emit('update:modelValue', ($event.target as HTMLInputElement).valueAsNumber)"
    />
  </div>
</template>
