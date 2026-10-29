<template>
  <div class="flex flex-wrap gap-2">
    <button
      v-for="preset in GROK_BASE_URL_PRESETS"
      :key="preset.url"
      type="button"
      data-testid="grok-base-url-preset"
      class="rounded-lg bg-gray-100 px-3 py-1 text-xs text-gray-700 transition-colors hover:bg-primary-50 hover:text-primary-700 dark:bg-dark-600 dark:text-gray-300 dark:hover:bg-primary-900/30 dark:hover:text-primary-400"
      @click="emit('select', preset.url)"
    >
      {{ presetLabel(preset) }} ({{ displayUrl(preset.url) }})
    </button>
  </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { GROK_BASE_URL_PRESETS, type GrokBaseUrlPreset } from './credentialsBuilder'

// Grok 快捷端点：点击把预设地址填入调用方的输入框。
// 仅是快速填充，不限制可填值——输入框仍接受任意第三方转发地址。
const emit = defineEmits<{
  (e: 'select', url: string): void
}>()

const { t } = useI18n()

const presetLabel = (preset: GrokBaseUrlPreset) =>
  preset.label ?? t(`admin.accounts.grokCustomBaseUrl.presets.${preset.labelKey}`)

const displayUrl = (url: string) => url.replace(/^https?:\/\//i, '')
</script>
