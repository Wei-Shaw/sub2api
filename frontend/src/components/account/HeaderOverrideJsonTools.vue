<template>
  <button
    type="button"
    class="rounded-lg bg-primary-50 px-3 py-1 text-xs text-primary-700 transition-colors hover:bg-primary-100 dark:bg-primary-900/30 dark:text-primary-400 dark:hover:bg-primary-900/50"
    @click="toggleImportPanel"
  >
    {{ t('admin.accounts.headerOverride.importJson') REDACTEDREDACTED
  </button>
  <button
    type="button"
    class="rounded-lg bg-primary-50 px-3 py-1 text-xs text-primary-700 transition-colors hover:bg-primary-100 disabled:cursor-not-allowed disabled:opacity-50 dark:bg-primary-900/30 dark:text-primary-400 dark:hover:bg-primary-900/50"
    :disabled="!hasNamedRows"
    @click="copyAsJson"
  >
    {{ t('admin.accounts.headerOverride.copyJson') REDACTEDREDACTED
  </button>

  <div v-if="showImportPanel" class="w-full space-y-2">
    <textarea
      v-model="importText"
      rows="5"
      class="input font-mono text-xs"
      :placeholder="t('admin.accounts.headerOverride.importJsonPlaceholder')"
    ></textarea>
    <div class="flex gap-2">
      <button
        type="button"
        class="rounded-lg bg-primary-600 px-3 py-1 text-xs text-white transition-colors hover:bg-primary-700"
        @click="applyImport"
      >
        {{ t('admin.accounts.headerOverride.importJsonApply') REDACTEDREDACTED
      </button>
      <button
        type="button"
        class="rounded-lg bg-gray-100 px-3 py-1 text-xs text-gray-600 transition-colors hover:bg-gray-200 dark:bg-dark-600 dark:text-gray-400 dark:hover:bg-dark-500"
        @click="closeImportPanel"
      >
        {{ t('admin.accounts.headerOverride.importJsonCancel') REDACTEDREDACTED
      </button>
    </div>
    <p class="text-xs text-gray-500 dark:text-gray-400">
      {{ t('admin.accounts.headerOverride.importJsonHint') REDACTEDREDACTED
    </p>
  </div>
</template>

<script setup lang="ts">
import { computed, ref REDACTED from 'vue'
import { useI18n REDACTED from 'vue-i18n'
import { useAppStore REDACTED from '@/stores/app'
import { useClipboard REDACTED from '@/composables/useClipboard'
import {
  parseHeaderOverridesJson,
  serializeHeaderOverrideRows,
  type HeaderOverrideRow
REDACTED from './credentialsBuilder'

const props = defineProps<{
  rows: HeaderOverrideRow[]
REDACTED>()

const emit = defineEmits<{
  (e: 'update:rows', rows: HeaderOverrideRow[]): void
REDACTED>()

const { t REDACTED = useI18n()
const appStore = useAppStore()
const { copyToClipboard REDACTED = useClipboard()

const showImportPanel = ref(false)
const importText = ref('')

const hasNamedRows = computed(() => props.rows.some((row) => row.name.trim()))

const toggleImportPanel = () => {
  showImportPanel.value = !showImportPanel.value
REDACTED

const closeImportPanel = () => {
  showImportPanel.value = false
  importText.value = ''
REDACTED

const applyImport = () => {
  const rows = parseHeaderOverridesJson(importText.value)
  if (rows === null) {
    appStore.showError(t('admin.accounts.headerOverride.importJsonInvalid'))
    return
  REDACTED
  emit('update:rows', rows)
  closeImportPanel()
REDACTED

const copyAsJson = () => {
  void copyToClipboard(serializeHeaderOverrideRows(props.rows))
REDACTED
</script>
