<!--
  ImageUploadInput — file picker that uploads via the host SDK and exposes
  the resulting public URL as v-model.

  Behavior:
    - When `modelValue` is empty: render a file input. Selecting an image
      uploads it through `getSdk().images.upload(file)` and emits the
      returned URL via update:modelValue.
    - When `modelValue` is non-empty: render a thumbnail preview and a
      remove button (which emits '' to clear).
    - During upload: a spinner replaces the file input; the input is
      disabled to prevent double-uploads.

  Why this lives in the plugin (not the host):
    - Host shipped no comparable upload component before this change; this
      one is intentionally minimal and tied to the plugin's i18n keys. If a
      second plugin needs the same control, promote it to packages/plugin-sdk.
-->
<template>
  <div class="space-y-2">
    <div
      v-if="modelValue"
      class="overflow-hidden rounded-lg border border-gray-200 bg-gray-50 p-2 dark:border-dark-600 dark:bg-dark-800"
    >
      <img
        :src="modelValue"
        class="mx-auto max-h-32 object-contain"
        alt=""
      />
      <div class="mt-2 flex items-center justify-between gap-2 text-xs">
        <span class="truncate text-gray-500 dark:text-gray-400">{{ modelValue }}</span>
        <button
          type="button"
          class="btn-ghost-danger shrink-0 rounded px-2 py-1"
          :disabled="uploading"
          @click="onClear"
        >
          {{ removeLabel || t('payment.adminSettings.helpImageRemove') }}
        </button>
      </div>
    </div>

    <div v-else class="flex items-center gap-2">
      <label
        class="inline-flex cursor-pointer items-center gap-2 rounded-lg border border-dashed border-gray-300 bg-white px-3 py-2 text-sm text-gray-600 hover:border-primary-400 hover:bg-primary-50 dark:border-dark-600 dark:bg-dark-800 dark:text-gray-300 dark:hover:border-primary-500 dark:hover:bg-primary-900/10"
        :class="{ 'pointer-events-none opacity-60': uploading }"
      >
        <LoadingSpinner v-if="uploading" size="sm" color="primary" />
        <span>{{ uploading ? t('payment.adminSettings.helpImageUploading') : (uploadLabel || t('payment.adminSettings.helpImageUpload')) }}</span>
        <input
          type="file"
          class="hidden"
          accept="image/png,image/jpeg,image/webp,image/gif,image/svg+xml"
          :disabled="uploading"
          @change="onFileSelected"
        />
      </label>
      <span v-if="hint" class="text-xs text-gray-400 dark:text-gray-500">{{ hint }}</span>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { getSdk } from '../../api/sdk'
import { extractApiErrorMessage } from '@sub2api/plugin-sdk'
import { useAppStore } from '../../stores/host'
import LoadingSpinner from './LoadingSpinner.vue'

interface Props {
  modelValue: string
  uploadLabel?: string
  removeLabel?: string
  hint?: string
}

interface Emits {
  (e: 'update:modelValue', value: string): void
}

defineProps<Props>()
const emit = defineEmits<Emits>()

const { t } = useI18n()
const appStore = useAppStore()

const uploading = ref(false)

async function onFileSelected(event: Event): Promise<void> {
  const input = event.target as HTMLInputElement
  const file = input.files?.[0]
  // Always reset the input so picking the same file twice still fires change.
  input.value = ''
  if (!file) return

  uploading.value = true
  try {
    const result = await getSdk().images.upload(file)
    emit('update:modelValue', result.url)
  } catch (err: unknown) {
    appStore.showError(extractApiErrorMessage(err, t('common.error')))
  } finally {
    uploading.value = false
  }
}

function onClear(): void {
  emit('update:modelValue', '')
}
</script>
