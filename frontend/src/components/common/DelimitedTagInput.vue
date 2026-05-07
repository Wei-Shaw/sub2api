<template>
  <div>
    <div class="flex min-h-[2.5rem] flex-wrap gap-1.5 rounded-lg border border-gray-200 bg-white p-2 dark:border-dark-600 dark:bg-dark-800">
      <span
        v-for="(item, index) in modelValue"
        :key="`${item}-${index}`"
        :class="[
          'inline-flex items-center gap-1 rounded-md px-2 py-0.5 text-sm',
          tagClass || 'bg-gray-100 text-gray-700 dark:bg-gray-900/30 dark:text-gray-300'
        ]"
      >
        {{ item }}
        <button
          type="button"
          class="rounded-full p-0.5 hover:bg-black/5 dark:hover:bg-white/10"
          @click="removeItem(index)"
        >
          <Icon name="x" size="xs" />
        </button>
      </span>
      <input
        v-model="inputValue"
        type="text"
        class="min-w-[120px] flex-1 border-none bg-transparent text-sm outline-none placeholder:text-gray-400 dark:text-white"
        :placeholder="modelValue.length === 0 ? placeholder : ''"
        @keydown.enter.prevent="commitInput"
        @keydown.tab.prevent="commitInput"
        @keydown.delete="handleBackspace"
        @paste="handlePaste"
        @blur="commitInput"
      />
    </div>
    <p v-if="hint" class="mt-1 text-xs text-gray-400">
      {{ hint }}
    </p>
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import Icon from '@/components/icons/Icon.vue'
import { mergeDelimitedValues } from '@/utils/accountFormBulk'

const props = defineProps<{
  modelValue: string[]
  placeholder?: string
  hint?: string
  tagClass?: string
}>()

const emit = defineEmits<{
  'update:modelValue': [value: string[]]
}>()

const inputValue = ref('')

const commitInput = () => {
  const next = mergeDelimitedValues(props.modelValue, inputValue.value)
  if (next.length !== props.modelValue.length) {
    emit('update:modelValue', next)
  }
  inputValue.value = ''
}

const removeItem = (index: number) => {
  const next = [...props.modelValue]
  next.splice(index, 1)
  emit('update:modelValue', next)
}

const handleBackspace = () => {
  if (!inputValue.value && props.modelValue.length > 0) {
    removeItem(props.modelValue.length - 1)
  }
}

const handlePaste = (event: ClipboardEvent) => {
  event.preventDefault()
  const text = event.clipboardData?.getData('text') || ''
  const next = mergeDelimitedValues(props.modelValue, text)
  if (next.length !== props.modelValue.length) {
    emit('update:modelValue', next)
  }
  inputValue.value = ''
}
</script>
