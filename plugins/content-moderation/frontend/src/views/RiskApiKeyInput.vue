<template>
  <div class="space-y-3">
    <div class="flex flex-col gap-2 rounded-lg border border-gray-100 bg-gray-50 p-2 dark:border-dark-700 dark:bg-dark-900/30 sm:flex-row sm:items-center sm:justify-between">
      <div class="text-xs leading-5 text-gray-500 dark:text-gray-400">
        <span class="font-medium text-gray-700 dark:text-gray-200">{{ t('admin.riskControl.apiKeysWriteMode') }}</span>
        <span class="ml-2">{{ ctx.apiKeysModeHint.value }}</span>
      </div>
      <div class="inline-flex rounded-lg bg-white p-1 shadow-sm dark:bg-dark-800">
        <button
          type="button"
          class="rounded-md px-3 py-1.5 text-xs font-medium transition-colors"
          :class="ctx.configForm.api_keys_mode === 'append' ? 'bg-primary-500 text-white shadow-sm' : 'text-gray-600 hover:bg-gray-100 dark:text-gray-300 dark:hover:bg-dark-700'"
          :disabled="ctx.configForm.clear_api_key"
          @click="ctx.setAPIKeysMode('append')"
        >
          {{ t('admin.riskControl.apiKeysModeAppend') }}
        </button>
        <button
          type="button"
          class="rounded-md px-3 py-1.5 text-xs font-medium transition-colors"
          :class="ctx.configForm.api_keys_mode === 'replace' ? 'bg-amber-500 text-white shadow-sm' : 'text-gray-600 hover:bg-gray-100 dark:text-gray-300 dark:hover:bg-dark-700'"
          :disabled="ctx.configForm.clear_api_key"
          @click="ctx.setAPIKeysMode('replace')"
        >
          {{ t('admin.riskControl.apiKeysModeReplace') }}
        </button>
      </div>
    </div>
    <textarea
      v-model="ctx.configForm.api_keys_text"
      class="input min-h-44 resize-y font-mono text-sm"
      :placeholder="ctx.apiKeysPlaceholder.value"
      autocomplete="new-password"
      :disabled="ctx.configForm.clear_api_key"
    ></textarea>
    <div class="flex flex-wrap items-center gap-2 text-xs text-gray-500 dark:text-gray-400">
      <span class="inline-flex rounded-md bg-gray-100 px-2 py-1 dark:bg-dark-700">
        {{ t('admin.riskControl.inputApiKeyCount', { count: ctx.inputApiKeyCount.value }) }}
      </span>
      <span v-if="ctx.configForm.api_key_configured" class="inline-flex rounded-md bg-gray-100 px-2 py-1 dark:bg-dark-700">
        {{ t('admin.riskControl.storedApiKeyCount', { count: ctx.configForm.api_key_count }) }}
      </span>
      <span v-if="ctx.configForm.clear_api_key" class="badge-danger inline-flex rounded-md px-2 py-1">
        {{ t('admin.riskControl.apiKeyWillClear') }}
      </span>
      <span v-else-if="ctx.pendingDeletedApiKeyCount.value > 0" class="badge-warning inline-flex rounded-md px-2 py-1">
        {{ t('admin.riskControl.apiKeyPendingDeleteCount', { count: ctx.pendingDeletedApiKeyCount.value }) }}
      </span>
      <span v-if="ctx.configForm.api_keys_mode === 'replace'" class="badge-warning inline-flex rounded-md px-2 py-1">
        {{ t('admin.riskControl.apiKeysReplaceWarning') }}
      </span>
    </div>

    <RiskAuditTestInput />
  </div>
</template>

<script setup lang="ts">
import { inject } from 'vue'
import { useI18n } from 'vue-i18n'
import { RiskControlKey } from './riskControlContext'
import RiskAuditTestInput from './RiskAuditTestInput.vue'

const { t } = useI18n()
const ctx = inject(RiskControlKey)!
</script>
