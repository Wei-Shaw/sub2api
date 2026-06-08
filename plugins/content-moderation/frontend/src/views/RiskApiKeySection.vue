<template>
  <div class="overflow-hidden rounded-xl border border-gray-100 bg-white shadow-sm dark:border-dark-700 dark:bg-dark-800">
    <div class="flex flex-col gap-4 border-b border-gray-100 bg-gray-50 px-4 py-4 dark:border-dark-700 dark:bg-dark-800/60 lg:flex-row lg:items-center lg:justify-between">
      <div class="flex items-start gap-3">
        <span class="mt-0.5 flex h-10 w-10 flex-shrink-0 items-center justify-center rounded-lg bg-primary-50 text-primary-600 dark:bg-primary-900/30 dark:text-primary-300">
          <Icon name="key" size="md" />
        </span>
        <div>
          <label class="text-sm font-semibold text-gray-900 dark:text-white">{{ t('admin.riskControl.apiKeys') }}</label>
          <p class="mt-1 max-w-3xl text-xs leading-5 text-gray-500 dark:text-gray-400">
            {{ t('admin.riskControl.apiKeysHint', { count: ctx.configForm.api_key_count }) }}
          </p>
        </div>
      </div>
      <RiskApiKeyActions />
    </div>

    <div class="grid grid-cols-1 gap-4 p-4 xl:grid-cols-[minmax(0,1fr)_minmax(360px,440px)]">
      <RiskApiKeyInput />
      <RiskApiKeyHealthPanel />
    </div>
  </div>
</template>

<script setup lang="ts">
import { inject } from 'vue'
import { useI18n } from 'vue-i18n'
import { Icon } from '@sub2api/plugin-sdk'
import { RiskControlKey } from './riskControlContext'
import RiskApiKeyActions from './RiskApiKeyActions.vue'
import RiskApiKeyInput from './RiskApiKeyInput.vue'
import RiskApiKeyHealthPanel from './RiskApiKeyHealthPanel.vue'

const { t } = useI18n()
const ctx = inject(RiskControlKey)!
</script>
