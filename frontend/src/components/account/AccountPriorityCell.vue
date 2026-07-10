<template>
  <div class="relative inline-flex w-24 items-center">
    <input
      v-model="draft"
      type="number"
      min="1"
      step="1"
      class="h-8 w-full rounded-md border border-gray-300 bg-white py-1 pl-2 pr-7 text-right font-mono text-sm text-gray-700 transition-colors focus:border-primary-500 focus:outline-none focus:ring-1 focus:ring-primary-500 disabled:cursor-wait disabled:bg-gray-100 disabled:text-gray-500 dark:border-dark-600 dark:bg-dark-800 dark:text-gray-300 dark:disabled:bg-dark-700"
      :aria-label="label"
      :title="label"
      :disabled="saving"
      @keydown.enter.prevent="submit"
      @keydown.esc.prevent="cancel"
      @blur="submit"
    />
    <Icon
      v-if="saving"
      name="refresh"
      size="xs"
      class="pointer-events-none absolute right-2 animate-spin text-gray-400"
      data-test="priority-saving"
    />
  </div>
</template>

<script setup lang="ts">
import { ref, watch } from 'vue'
import Icon from '@/components/icons/Icon.vue'

const props = withDefaults(defineProps<{
  value: number
  saving?: boolean
  label?: string
}>(), {
  saving: false,
  label: 'Priority'
})

const emit = defineEmits<{
  save: [value: number]
}>()

const draft = ref(String(props.value))
const submittedValue = ref<number | null>(null)

const resetDraft = () => {
  draft.value = String(props.value)
}

const submit = () => {
  if (props.saving) return

  const nextValue = Number(draft.value)
  if (!Number.isInteger(nextValue) || nextValue < 1) {
    resetDraft()
    return
  }
  if (nextValue === props.value) {
    resetDraft()
    return
  }
  if (submittedValue.value === nextValue) return

  submittedValue.value = nextValue
  emit('save', nextValue)
}

const cancel = (event: KeyboardEvent) => {
  submittedValue.value = null
  resetDraft()
  const input = event.currentTarget as HTMLInputElement | null
  input?.blur()
}

watch(() => props.value, resetDraft)
watch(() => props.saving, (saving) => {
  if (!saving) {
    submittedValue.value = null
    resetDraft()
  }
})
</script>
