<template>
  <div>
    <!-- Tags display -->
    <div class="flex min-h-[2.5rem] flex-wrap gap-1.5 rounded-sm border border-border-strong bg-surface p-2 focus-within:border-accent">
      <span
        v-for="(model, idx) in models"
        :key="idx"
        class="inline-flex items-center gap-1 rounded-sm px-2 py-0.5 font-mono text-label"
        :class="getPlatformTagClass(props.platform || '')"
      >
        {{ model }}
        <button
          :aria-label="t('common.remove')"
          :title="t('common.remove')"
          type="button"
          @click="removeModel(idx)"
          class="ml-0.5 rounded-sm p-0.5 hover:bg-accent-weak"
        >
          <Icon name="x" size="xs" />
        </button>
      </span>
      <input
        ref="inputRef"
        v-model="inputValue"
        type="text"
        class="flex-1 min-w-[120px] border-none bg-transparent text-sm outline-none font-mono text-fg placeholder:font-sans placeholder:text-fg-subtle"
        :placeholder="models.length === 0 ? placeholder : ''"
        @keydown.enter="handleEnter"
        @keydown.tab="handleTab"
        @keydown.backspace="handleBackspace"
        @paste="handlePaste"
        @blur="addModel"
      />
    </div>
    <p class="mt-1 text-xs text-fg-subtle">
      {{ t('admin.channels.form.modelInputHint', 'Press Enter to add, supports paste for batch import.') }}
    </p>
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
import { getPlatformTagClass } from './types'

const { t } = useI18n()

const props = defineProps<{
  models: string[]
  placeholder?: string
  platform?: string
}>()

const emit = defineEmits<{
  'update:models': [models: string[]]
}>()

const inputValue = ref('')
const inputRef = ref<HTMLInputElement>()

function addModel() {
  const val = inputValue.value.trim()
  if (!val) return
  if (!props.models.includes(val)) {
    emit('update:models', [...props.models, val])
  }
  inputValue.value = ''
}

function handleEnter(event: KeyboardEvent) {
  if (event.isComposing) return
  event.preventDefault()
  addModel()
}

function handleTab(event: KeyboardEvent) {
  if (event.isComposing || !inputValue.value.trim()) return
  event.preventDefault()
  addModel()
}

function removeModel(idx: number) {
  const newModels = [...props.models]
  newModels.splice(idx, 1)
  emit('update:models', newModels)
}

function handleBackspace(event: KeyboardEvent) {
  if (event.isComposing) return
  if (inputValue.value === '' && props.models.length > 0) {
    removeModel(props.models.length - 1)
  }
}

function handlePaste(e: ClipboardEvent) {
  e.preventDefault()
  const text = e.clipboardData?.getData('text') || ''
  const items = text.split(/[,\n;]+/).map(s => s.trim()).filter(Boolean)
  if (items.length === 0) return
  const unique = [...new Set([...props.models, ...items])]
  emit('update:models', unique)
  inputValue.value = ''
}
</script>
