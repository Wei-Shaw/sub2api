<template>
  <div class="space-y-6">
    <div>
      <h3 class="text-base font-semibold text-gray-900 dark:text-gray-100">
        {{ t('admin.settings.asyncMedia.title') }}
      </h3>
      <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
        {{ t('admin.settings.asyncMedia.description') }}
      </p>
    </div>

    <div v-if="loading" class="py-8 text-center text-sm text-gray-500 dark:text-gray-400">
      {{ t('common.loading') }}
    </div>

    <template v-else>
      <div class="grid grid-cols-1 gap-4 sm:grid-cols-2">
        <div>
          <label class="form-label">{{ t('admin.settings.asyncMedia.reconcileInterval') }}</label>
          <input
            v-model.number="form.reconcile_interval_seconds"
            type="number"
            min="1"
            max="3600"
            class="input"
          />
          <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
            {{ t('admin.settings.asyncMedia.reconcileIntervalHint') }}
          </p>
        </div>
        <div>
          <label class="form-label">{{ t('admin.settings.asyncMedia.failTimeout') }}</label>
          <input
            v-model.number="form.fail_timeout_seconds"
            type="number"
            min="60"
            max="86400"
            class="input"
          />
          <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
            {{ t('admin.settings.asyncMedia.failTimeoutHint') }}
          </p>
        </div>
      </div>

      <div class="flex justify-end">
        <button type="button" class="btn btn-primary" :disabled="saving" @click="onSave">
          {{ saving ? t('common.saving') : t('common.save') }}
        </button>
      </div>
    </template>
  </div>
</template>

<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores/app'
import {
  getAsyncMediaConfig,
  updateAsyncMediaConfig,
  type AsyncMediaRuntimeConfig,
} from '@/api/admin/asyncMedia'

const { t } = useI18n()
const appStore = useAppStore()

const loading = ref(false)
const saving = ref(false)

const form = reactive<AsyncMediaRuntimeConfig>({
  reconcile_interval_seconds: 30,
  fail_timeout_seconds: 1800,
})

async function load() {
  loading.value = true
  try {
    const resp = await getAsyncMediaConfig()
    Object.assign(form, resp)
  } catch (e: unknown) {
    appStore.showError(
      (e as { message?: string })?.message ?? t('admin.settings.asyncMedia.loadFailed'),
    )
  } finally {
    loading.value = false
  }
}

async function onSave() {
  saving.value = true
  try {
    const updated = await updateAsyncMediaConfig({ ...form })
    Object.assign(form, updated)
    appStore.showSuccess(t('admin.settings.asyncMedia.saveSuccess'))
  } catch (e: unknown) {
    appStore.showError(
      (e as { message?: string })?.message ?? t('admin.settings.asyncMedia.saveFailed'),
    )
  } finally {
    saving.value = false
  }
}

onMounted(load)
</script>
