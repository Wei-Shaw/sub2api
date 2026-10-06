<template>
  <div class="space-y-4">
    <p class="input-hint">{{ t('admin.accounts.muse.setupNote') }}</p>
    <div class="space-y-3 rounded-lg bg-gray-50 p-4 text-sm dark:bg-dark-700" data-testid="muse-export-guide">
      <p class="font-medium">{{ t('admin.accounts.muse.exportTitle') }}</p>
      <p class="input-hint">{{ t('admin.accounts.muse.exportDesktop') }}</p>
      <a href="/muse-session-exporter.zip" download class="btn btn-secondary btn-sm inline-flex">
        {{ t('admin.accounts.muse.downloadExporter') }}
      </a>
      <ol class="list-decimal space-y-2 pl-5 text-gray-600 dark:text-gray-300">
        <li>{{ t('admin.accounts.muse.exportUnzip') }}</li>
        <li>{{ t('admin.accounts.muse.exportInstall') }} <code class="text-xs">chrome://extensions</code> / <code class="text-xs">edge://extensions</code></li>
        <li>{{ t('admin.accounts.muse.exportLogin') }} <a href="https://muse.ai/" target="_blank" rel="noopener noreferrer" class="text-primary-600 underline dark:text-primary-400">muse.ai</a></li>
        <li>{{ t('admin.accounts.muse.exportDownload') }}</li>
        <li>{{ t('admin.accounts.muse.exportImport') }}</li>
      </ol>
      <p class="input-hint">{{ t('admin.accounts.muse.exportPrivate') }}</p>
    </div>
    <div>
      <label class="input-label" for="muse-owner">{{ t('admin.accounts.muse.owner') }}</label>
      <input id="muse-owner" :value="ownerId" type="number" min="1" step="1" required class="input"
        @input="emit('update:ownerId', Number(($event.target as HTMLInputElement).value))" />
      <p class="input-hint">{{ t('admin.accounts.muse.ownerNote') }}</p>
    </div>
    <div>
      <div class="mb-2 flex flex-wrap items-center justify-between gap-2">
        <label class="input-label mb-0" for="muse-session">{{ t('admin.accounts.muse.session') }}</label>
        <label class="btn btn-secondary btn-sm cursor-pointer">
          {{ t('admin.accounts.muse.importFile') }}
          <input type="file" accept=".json,application/json" class="sr-only" :aria-label="t('admin.accounts.muse.importFile')" @change="importFile" />
        </label>
      </div>
      <p v-if="fileError" role="alert" class="mb-2 text-sm text-red-600 dark:text-red-400">{{ fileError }}</p>
      <textarea id="muse-session" :value="sessionJson" class="input font-mono" rows="5" autocomplete="off"
        :placeholder="replacement ? t('admin.accounts.muse.keepSession') : t('admin.accounts.muse.sessionPlaceholder')"
        @input="emit('update:sessionJson', ($event.target as HTMLTextAreaElement).value)" />
      <p class="input-hint">{{ t('admin.accounts.muse.afterSave') }}</p>
    </div>
  </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { ref } from 'vue'
defineProps<{ ownerId: number; sessionJson: string; replacement?: boolean }>()
const emit = defineEmits<{ 'update:ownerId': [value: number]; 'update:sessionJson': [value: string] }>()
const { t } = useI18n()
const fileError = ref('')
async function importFile(event: Event) {
  const input = event.target as HTMLInputElement
  const file = input.files?.[0]
  fileError.value = ''
  if (!file) return
  try {
    if (file.size > 64 * 1024) throw new Error('too large')
    const raw = await file.text()
    const value: unknown = JSON.parse(raw)
    if (!value || typeof value !== 'object' || Array.isArray(value) || Object.keys(value).length === 0) throw new Error('invalid JSON')
    emit('update:sessionJson', raw)
  } catch {
    fileError.value = t('admin.accounts.muse.importFileError')
  } finally {
    input.value = ''
  }
}
</script>
