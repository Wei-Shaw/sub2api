<template>
  <div>
    <div class="input flex min-h-[42px] flex-wrap items-center gap-1.5 py-1.5">
      <span
        v-for="(tag, idx) in modelValue"
        :key="idx"
        class="inline-flex items-center gap-1 rounded-md bg-primary-50 px-2 py-0.5 text-xs font-medium text-primary-700 dark:bg-primary-900/30 dark:text-primary-300"
      >
        <span class="font-mono">{{ tag }}</span>
        <button type="button" class="text-primary-500 hover:text-red-500" @click.stop="remove(idx)">
          <Icon name="x" size="xs" />
        </button>
      </span>
      <input
        v-model="draft"
        type="text"
        class="flex-1 min-w-[160px] border-none bg-transparent p-0 text-sm outline-none focus:ring-0"
        :placeholder="modelValue.length === 0 ? placeholder : enterToAddHint"
        @keydown.enter.prevent="commit"
        @keydown.comma.prevent="commit"
        @blur="commit"
      />
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'

const props = defineProps<{
  modelValue: string[]
  placeholder?: string
  enterToAddHint?: string
}>()

const emit = defineEmits<{
  (e: 'update:modelValue', value: string[]): void
}>()

const { t } = useI18n()
const draft = ref('')
const enterToAddHint = props.enterToAddHint ?? t('common.tagInput.enterToAdd')

function commit() {
  const value = draft.value.trim()
  if (!value) {
    draft.value = ''
    return
  }
  if (props.modelValue.includes(value)) {
    draft.value = ''
    return
  }
  emit('update:modelValue', [...props.modelValue, value])
  draft.value = ''
}

function remove(idx: number) {
  const next = props.modelValue.slice()
  next.splice(idx, 1)
  emit('update:modelValue', next)
}
</script>