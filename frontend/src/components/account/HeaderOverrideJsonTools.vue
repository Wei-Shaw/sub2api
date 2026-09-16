<template>
  <button
    type="button"
    class="rounded-lg bg-primary-50 px-3 py-1 text-xs text-primary-700 transition-colors hover:bg-primary-100 dark:bg-primary-900/30 dark:text-primary-400 dark:hover:bg-primary-900/50"
    @click="toggleImportPanel"
  >
    {{ t('admin.accounts.headerOverride.importJson') }}
  </button>
  <button
    type="button"
    class="rounded-lg bg-primary-50 px-3 py-1 text-xs text-primary-700 transition-colors hover:bg-primary-100 disabled:cursor-not-allowed disabled:opacity-50 dark:bg-primary-900/30 dark:text-primary-400 dark:hover:bg-primary-900/50"
    :disabled="!hasNamedRows"
    @click="copyAsJson"
  >
    {{ t('admin.accounts.headerOverride.copyJson') }}
  </button>

  <div v-if="showImportPanel" ref="importPanelRef" class="w-full space-y-2">
    <textarea
      ref="importTextareaRef"
      v-model="importText"
      rows="5"
      class="input font-mono text-xs"
      :placeholder="IMPORT_JSON_PLACEHOLDER"
    ></textarea>
    <div class="flex gap-2">
      <button
        type="button"
        class="rounded-lg bg-primary-600 px-3 py-1 text-xs text-white transition-colors hover:bg-primary-700"
        @click="applyImport"
      >
        {{ t('admin.accounts.headerOverride.importJsonApply') }}
      </button>
      <button
        type="button"
        class="rounded-lg bg-gray-100 px-3 py-1 text-xs text-gray-600 transition-colors hover:bg-gray-200 dark:bg-dark-600 dark:text-gray-400 dark:hover:bg-dark-500"
        @click="closeImportPanel"
      >
        {{ t('admin.accounts.headerOverride.importJsonCancel') }}
      </button>
    </div>
    <p class="text-xs text-gray-500 dark:text-gray-400">
      {{ t('admin.accounts.headerOverride.importJsonHint') }}
    </p>
  </div>
</template>

<script setup lang="ts">
import { computed, nextTick, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores/app'
import { useClipboard } from '@/composables/useClipboard'
import {
  parseHeaderOverridesJson,
  serializeHeaderOverrideRows,
  type HeaderOverrideRow
} from './credentialsBuilder'

const props = defineProps<{
  rows: HeaderOverrideRow[]
}>()

const emit = defineEmits<{
  (e: 'update:rows', rows: HeaderOverrideRow[]): void
}>()

const { t } = useI18n()
const appStore = useAppStore()
const { copyToClipboard } = useClipboard()

// JSON 示例语言中立，且花括号会被 vue-i18n 消息编译器当作插值占位符解析
// 导致渲染时抛 SyntaxError，因此不走 i18n。
const IMPORT_JSON_PLACEHOLDER = '{"user-agent": "my-client/1.0", "x-relay-token": "..."}'

const showImportPanel = ref(false)
const importText = ref('')
const importPanelRef = ref<HTMLElement | null>(null)
const importTextareaRef = ref<HTMLTextAreaElement | null>(null)

const hasNamedRows = computed(() => props.rows.some((row) => row.name.trim()))

// 面板在弹窗滚动容器（.modal-body）内向下展开，可能落在当前视野之外，
// 打开后必须主动滚入视野，否则看起来像点击无效。
const toggleImportPanel = async () => {
  showImportPanel.value = !showImportPanel.value
  if (!showImportPanel.value) return
  await nextTick()
  importPanelRef.value?.scrollIntoView({ block: 'nearest', behavior: 'smooth' })
  importTextareaRef.value?.focus({ preventScroll: true })
}

const closeImportPanel = () => {
  showImportPanel.value = false
  importText.value = ''
}

const applyImport = () => {
  const rows = parseHeaderOverridesJson(importText.value)
  if (rows === null) {
    appStore.showError(t('admin.accounts.headerOverride.importJsonInvalid'))
    return
  }
  emit('update:rows', rows)
  closeImportPanel()
}

const copyAsJson = () => {
  void copyToClipboard(serializeHeaderOverrideRows(props.rows))
}
</script>
