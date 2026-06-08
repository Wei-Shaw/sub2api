<template>
  <div class="space-y-5">
    <div class="grid grid-cols-1 gap-5 lg:grid-cols-2">
      <div class="flex items-center justify-between rounded-lg border border-gray-100 p-4 dark:border-dark-700">
        <div>
          <p class="text-sm font-medium text-gray-900 dark:text-white">{{ t('admin.riskControl.enabled') }}</p>
          <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.riskControl.enabledHint') }}</p>
        </div>
        <Toggle v-model="ctx.configForm.enabled" />
      </div>
      <div>
        <label class="input-label">{{ t('admin.riskControl.mode') }}</label>
        <Select v-model="ctx.configForm.mode" :options="ctx.modeOptions.value" />
        <p class="mt-2 text-xs leading-5 text-gray-500 dark:text-gray-400">{{ ctx.modeDescription(ctx.configForm.mode) }}</p>
      </div>
      <div>
        <label class="input-label">{{ t('admin.riskControl.baseUrl') }}</label>
        <input v-model.trim="ctx.configForm.base_url" type="url" class="input" placeholder="https://api.openai.com" />
      </div>
      <div>
        <label class="input-label">{{ t('admin.riskControl.model') }}</label>
        <input v-model.trim="ctx.configForm.model" type="text" class="input" placeholder="omni-moderation-latest" />
      </div>
      <div>
        <label class="input-label">{{ t('admin.riskControl.timeoutMs') }}</label>
        <input v-model.number="ctx.configForm.timeout_ms" type="number" min="500" max="30000" class="input" />
      </div>
      <div>
        <label class="input-label">{{ t('admin.riskControl.retryCount') }}</label>
        <input v-model.number="ctx.configForm.retry_count" type="number" min="0" max="5" class="input" />
      </div>
      <div>
        <label class="input-label">{{ t('admin.riskControl.sampleRate') }}</label>
        <div class="relative">
          <input v-model.number="ctx.configForm.sample_rate" type="number" min="0" max="100" step="1" class="input pr-8" />
          <span class="pointer-events-none absolute right-3 top-1/2 -translate-y-1/2 text-gray-400">%</span>
        </div>
      </div>
    </div>

    <RiskApiKeySection />
  </div>
</template>

<script setup lang="ts">
import { inject } from 'vue'
import { useI18n } from 'vue-i18n'
import { Select, Toggle } from '@sub2api/plugin-sdk'
import { RiskControlKey } from './riskControlContext'
import RiskApiKeySection from './RiskApiKeySection.vue'

const { t } = useI18n()
const ctx = inject(RiskControlKey)!
</script>
