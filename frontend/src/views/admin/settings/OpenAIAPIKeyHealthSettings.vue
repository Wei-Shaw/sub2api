<template>
  <section class="card" aria-labelledby="openai-health-title">
    <div class="border-b border-gray-100 px-6 py-4 dark:border-dark-700">
      <h2 id="openai-health-title" class="text-lg font-semibold text-gray-900 dark:text-white">
        {{ t('admin.settings.openaiAPIKeyHealth.title') }}
      </h2>
      <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
        {{ t('admin.settings.openaiAPIKeyHealth.description') }}
      </p>
    </div>
    <div class="space-y-5 p-6">
      <p v-if="loading" role="status">{{ t('common.loading') }}</p>
      <div v-else-if="loadFailed" class="flex flex-wrap items-center gap-3">
        <p role="alert" class="text-sm text-red-600">{{ t('admin.settings.openaiAPIKeyHealth.loadFailed') }}</p>
        <button type="button" class="btn btn-secondary btn-sm" @click="load">
          <Icon name="refresh" size="sm" class="mr-1" />
          {{ t('common.refresh') }}
        </button>
      </div>
      <template v-else>
        <div class="flex items-center justify-between gap-4">
          <label id="openai-health-enabled" class="font-medium text-gray-900 dark:text-white">
            {{ t('admin.settings.openaiAPIKeyHealth.enabled') }}
          </label>
          <Toggle v-model="form.enabled" :disabled="saving" aria-labelledby="openai-health-enabled" />
        </div>
        <div class="grid grid-cols-1 gap-4 sm:grid-cols-3">
          <div v-for="field in fields" :key="field.key" class="min-w-0">
            <label :for="`health-${field.key}`" class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">
              {{ t(`admin.settings.openaiAPIKeyHealth.${field.key}`) }}
            </label>
            <input
              :id="`health-${field.key}`"
              v-model.number="form[field.key]"
              type="number"
              min="1"
              :max="field.max"
              step="1"
              :disabled="saving"
              class="input w-full max-w-40"
            />
          </div>
        </div>
        <p class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.settings.openaiAPIKeyHealth.scopeHint') }}</p>
        <div class="flex justify-end border-t border-gray-100 pt-4 dark:border-dark-700">
          <button type="button" class="btn btn-primary btn-sm" :disabled="saving || !valid" @click="save">
            <Icon :name="saving ? 'refresh' : 'check'" size="sm" :class="['mr-1', { 'animate-spin': saving }]" />
            {{ t(saving ? 'common.saving' : 'common.save') }}
          </button>
        </div>
      </template>
    </div>
  </section>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores/app'
import Toggle from '@/components/common/Toggle.vue'
import Icon from '@/components/icons/Icon.vue'
import {
  getOpenAIAPIKeyHealthBreakerSettings,
  updateOpenAIAPIKeyHealthBreakerSettings,
  type OpenAIAPIKeyHealthBreakerSettings,
} from '@/api/admin/settings'

const { t } = useI18n()
const appStore = useAppStore()
const loading = ref(true)
const saving = ref(false)
const loadFailed = ref(false)
const form = reactive<OpenAIAPIKeyHealthBreakerSettings>({
  enabled: false, window_minutes: 2, failure_threshold: 10, cooldown_minutes: 5,
})
const fields = [
  { key: 'window_minutes', max: 60 },
  { key: 'failure_threshold', max: 10000 },
  { key: 'cooldown_minutes', max: 60 },
] as const
const valid = computed(() => fields.every(({ key, max }) =>
  Number.isInteger(form[key]) && form[key] >= 1 && form[key] <= max,
))

async function load() {
  loading.value = true
  loadFailed.value = false
  try {
    Object.assign(form, await getOpenAIAPIKeyHealthBreakerSettings())
  } catch {
    loadFailed.value = true
  } finally {
    loading.value = false
  }
}

async function save() {
  if (saving.value || !valid.value) return
  saving.value = true
  try {
    Object.assign(form, await updateOpenAIAPIKeyHealthBreakerSettings({ ...form }))
    appStore.showSuccess(t('admin.settings.openaiAPIKeyHealth.saved'))
  } catch {
    appStore.showError(t('admin.settings.openaiAPIKeyHealth.saveFailed'))
  } finally {
    saving.value = false
  }
}

onMounted(load)
</script>
