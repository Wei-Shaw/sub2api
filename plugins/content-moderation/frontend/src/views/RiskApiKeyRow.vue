<template>
  <div
    class="rounded-lg border bg-white p-2.5 shadow-sm dark:bg-dark-800"
    :class="ctx.isStoredApiKeyPendingDelete(row) ? 'border-amber-200 opacity-70 dark:border-amber-800/60' : 'border-gray-100 dark:border-dark-700'"
  >
    <div class="flex items-start justify-between gap-2">
      <div class="min-w-0">
        <div class="flex min-w-0 flex-wrap items-center gap-2">
          <span class="truncate font-mono text-sm font-semibold text-gray-900 dark:text-white">{{ row.masked || '-' }}</span>
          <span
            class="inline-flex rounded-md px-1.5 py-0.5 text-[11px] font-medium"
            :class="row.configured ? 'bg-primary-50 text-primary-700 dark:bg-primary-900/30 dark:text-primary-300' : 'bg-purple-50 text-purple-700 dark:bg-purple-900/30 dark:text-purple-300'"
          >
            {{ ctx.isStoredApiKeyPendingDelete(row) ? t('admin.riskControl.apiKeyPendingDelete') : row.configured ? t('admin.riskControl.apiKeyConfigured') : t('admin.riskControl.apiKeyTemporary') }}
          </span>
        </div>
        <p class="mt-1 text-xs leading-5 text-gray-500 dark:text-gray-400">{{ ctx.apiKeyStatusMeta(row) }}</p>
      </div>
      <div class="flex flex-shrink-0 items-center gap-1.5">
        <span class="inline-flex items-center gap-1.5 whitespace-nowrap rounded-full px-2 py-0.5 text-xs font-medium" :class="ctx.apiKeyStatusBadgeClass(row.status)">
          <span class="h-1.5 w-1.5 rounded-full" :class="ctx.apiKeyStatusDotClass(row.status)"></span>
          {{ ctx.apiKeyStatusLabel(row.status) }}
        </span>
        <button
          v-if="row.configured && !ctx.configForm.clear_api_key"
          type="button"
          class="inline-flex h-7 w-7 items-center justify-center rounded-md text-gray-400 transition-colors hover:bg-gray-100 hover:text-gray-700 dark:hover:bg-dark-700 dark:hover:text-gray-200"
          :title="ctx.isStoredApiKeyPendingDelete(row) ? t('admin.riskControl.undoDeleteApiKey') : t('admin.riskControl.deleteApiKey')"
          @click="ctx.toggleDeleteStoredApiKey(row)"
        >
          <Icon :name="ctx.isStoredApiKeyPendingDelete(row) ? 'refresh' : 'trash'" size="xs" />
        </button>
      </div>
    </div>
    <p v-if="row.last_error" class="badge-warning mt-1.5 rounded-md px-2 py-1.5 text-xs leading-5">
      {{ row.last_error }}
    </p>
  </div>
</template>

<script setup lang="ts">
import { inject } from 'vue'
import { useI18n } from 'vue-i18n'
import { Icon } from '@sub2api/plugin-sdk'
import type { ContentModerationAPIKeyStatus } from './types'
import { RiskControlKey } from './riskControlContext'

defineProps<{ row: ContentModerationAPIKeyStatus }>()

const { t } = useI18n()
const ctx = inject(RiskControlKey)!
</script>
