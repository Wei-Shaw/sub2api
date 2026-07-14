<template>
  <div class="relative inline-flex h-8 w-28 items-center">
    <span
      v-if="!editing"
      class="inline-flex h-8 w-full cursor-text select-none items-center justify-end px-2 font-mono text-sm text-gray-700 dark:text-gray-300"
      :class="saving ? 'cursor-wait pr-7 text-gray-500 dark:text-gray-400' : ''"
      data-test="priority-value"
      @dblclick.prevent="startEditing"
    >
      {{ displayedValue }}
    </span>
    <input
      v-else
      ref="inputRef"
      v-model="draft"
      type="number"
      min="1"
      step="1"
      class="h-8 w-full rounded-md border border-gray-300 bg-white px-2 py-1 text-right font-mono text-sm text-gray-700 transition-colors focus:border-primary-500 focus:outline-none focus:ring-1 focus:ring-primary-500 dark:border-dark-600 dark:bg-dark-800 dark:text-gray-300"
      :aria-label="label"
      :title="label"
      @keydown.enter.prevent="finishEditing"
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
import { computed, nextTick, ref, watch } from 'vue'
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

const inputRef = ref<HTMLInputElement | null>(null)
const editing = ref(false)
const draft = ref(String(props.value))
const submittedValue = ref<number | null>(null)
const displayedValue = computed(() => submittedValue.value ?? props.value)

const resetDraft = () => {
  draft.value = String(props.value)
}

const startEditing = async () => {
  if (props.saving) return

  resetDraft()
  editing.value = true
  await nextTick()
  inputRef.value?.focus()
  inputRef.value?.select()
}

const submit = () => {
  if (!editing.value) return
  editing.value = false

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

const finishEditing = () => {
  submit()
}

const cancel = () => {
  editing.value = false
  submittedValue.value = null
  resetDraft()
}

watch(() => props.value, () => {
  if (!editing.value) resetDraft()
})
watch(() => props.saving, (saving) => {
  if (!saving) {
    submittedValue.value = null
    resetDraft()
  }
})
</script>
