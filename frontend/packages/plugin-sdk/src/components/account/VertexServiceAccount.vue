<template>
  <div class="space-y-4">
    <div>
      <label class="input-label">Service Account JSON</label>
      <input ref="fileInputRef" type="file" accept="application/json,.json" class="hidden" @change="handleFileSelect" />
      <div
        :class="[
          'rounded-lg border-2 border-dashed px-4 py-5 transition-colors',
          dragActive
            ? 'border-sky-500 bg-sky-50 dark:border-sky-500 dark:bg-sky-900/20'
            : 'border-gray-300 bg-gray-50 hover:border-sky-400 hover:bg-sky-50/60 dark:border-dark-500 dark:bg-dark-700/40 dark:hover:border-sky-600 dark:hover:bg-sky-900/10'
        ]"
        @dragenter.prevent="dragActive = true"
        @dragover.prevent="dragActive = true"
        @dragleave.prevent="dragActive = false"
        @drop.prevent="handleDrop"
      >
        <div class="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
          <div class="min-w-0">
            <div class="flex items-center gap-2 text-sm font-medium text-gray-900 dark:text-white">
              <Icon name="upload" size="sm" />
              <span>{{ clientEmail ? t('admin.accounts.vertexSaJsonLoaded') : t('admin.accounts.vertexSaJsonDrop') }}</span>
            </div>
            <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
              {{ clientEmail ? t('admin.accounts.vertexSaJsonKeyHidden') : t('admin.accounts.vertexSaJsonDropHint') }}
            </p>
          </div>
          <button type="button" class="btn btn-secondary shrink-0" @click="fileInputRef?.click()">
            <Icon name="upload" size="sm" />
            {{ t('admin.accounts.vertexSaJsonSelectBtn') }}
          </button>
        </div>
        <div
          v-if="clientEmail"
          class="mt-3 rounded-md border border-sky-200 bg-white px-3 py-2 text-xs text-sky-900 dark:border-sky-800/50 dark:bg-dark-800 dark:text-sky-200"
        >
          <div class="truncate">Project ID: <span class="font-mono">{{ projectId }}</span></div>
          <div class="truncate">Client Email: <span class="font-mono">{{ clientEmail }}</span></div>
        </div>
      </div>
      <p class="input-hint">{{ t('admin.accounts.vertexSaJsonUploadHint') }}</p>
    </div>

    <div class="grid grid-cols-1 gap-4 sm:grid-cols-2">
      <div>
        <label class="input-label">Project ID</label>
        <input :value="projectId" type="text" class="input font-mono" readonly :placeholder="t('admin.accounts.vertexProjectIdPlaceholder')" />
      </div>
      <div>
        <label class="input-label">Location</label>
        <select :value="location" required class="input font-mono" @change="$emit('update:location', ($event.target as HTMLSelectElement).value)">
          <optgroup v-for="group in VERTEX_LOCATION_OPTIONS" :key="group.label" :label="group.label">
            <option v-for="opt in group.options" :key="opt.value" :value="opt.value">{{ opt.label }}</option>
          </optgroup>
        </select>
        <p class="input-hint">{{ t('admin.accounts.vertexLocationHint') }}</p>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '../Icon.vue'
import { VERTEX_LOCATION_OPTIONS } from '../../constants/account'

const { t } = useI18n()

defineProps<{
  serviceAccountJson: string
  projectId: string
  clientEmail: string
  location: string
}>()

const emit = defineEmits<{
  'update:serviceAccountJson': [value: string]
  'update:projectId': [value: string]
  'update:clientEmail': [value: string]
  'update:location': [value: string]
  'parseError': [message: string]
}>()

const fileInputRef = ref<HTMLInputElement | null>(null)
const dragActive = ref(false)

const applyJson = (value: string) => {
  const raw = value.trim()
  if (!raw) { emit('update:projectId', ''); emit('update:clientEmail', ''); return }
  try {
    const parsed = JSON.parse(raw) as Record<string, unknown>
    const pid = typeof parsed.project_id === 'string' ? parsed.project_id.trim() : ''
    const email = typeof parsed.client_email === 'string' ? parsed.client_email.trim() : ''
    const key = typeof parsed.private_key === 'string' ? parsed.private_key.trim() : ''
    if (!pid || !email || !key) { emit('parseError', t('admin.accounts.vertexSaJsonMissingFields')); return }
    emit('update:projectId', pid)
    emit('update:clientEmail', email)
    emit('update:serviceAccountJson', JSON.stringify(parsed))
  } catch { emit('parseError', t('admin.accounts.vertexSaJsonInvalid')) }
}

const handleFileSelect = async (event: Event) => {
  const input = event.target as HTMLInputElement
  const file = input.files?.[0]
  if (!file) return
  try { applyJson(await file.text()) } finally { input.value = '' }
}

const handleDrop = async (event: DragEvent) => {
  dragActive.value = false
  const file = event.dataTransfer?.files?.[0]
  if (file) applyJson(await file.text())
}
</script>
