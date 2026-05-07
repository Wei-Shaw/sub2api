<template>
  <div class="mb-3 rounded-lg border border-dashed border-gray-300 bg-gray-50 p-3 dark:border-dark-500 dark:bg-dark-800/60">
    <div class="flex flex-wrap items-center justify-between gap-2">
      <div>
        <p class="text-sm font-medium text-gray-700 dark:text-gray-200">{{ title }}</p>
        <p v-if="hint" class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ hint }}</p>
      </div>
      <button
        type="button"
        class="btn btn-secondary btn-sm"
        :disabled="!draft.trim()"
        @click="submit"
      >
        {{ buttonText }}
      </button>
    </div>
    <textarea
      v-model="draft"
      rows="4"
      class="input mt-3 w-full font-mono text-sm"
      :placeholder="placeholder"
    />
  </div>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'

const props = withDefaults(defineProps<{
  title?: string
  hint?: string
  placeholder?: string
  clearSignal?: number
}>(), {
  title: '',
  hint: '',
  placeholder: ''
})

const emit = defineEmits<{
  import: [value: string]
}>()

const { t } = useI18n()
const draft = ref('')
const buttonText = computed(() => t('admin.accounts.importMappings'))

watch(() => props.clearSignal, () => {
  draft.value = ''
})

const submit = () => {
  if (!draft.value.trim()) return
  emit('import', draft.value)
  draft.value = ''
}
</script>
