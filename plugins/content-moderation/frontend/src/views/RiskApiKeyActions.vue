<template>
  <div class="flex flex-wrap items-center gap-2">
    <button
      type="button"
      class="btn btn-secondary inline-flex items-center gap-2"
      :disabled="ctx.apiKeyTesting.value || ctx.inputApiKeyCount.value === 0 || ctx.configForm.clear_api_key"
      @click="ctx.testApiKeys(true)"
    >
      <Icon name="beaker" size="sm" :class="ctx.apiKeyTesting.value ? 'animate-pulse' : ''" />
      {{ ctx.apiKeyTesting.value ? t('admin.riskControl.testingApiKeys') : t('admin.riskControl.testInputApiKeys') }}
    </button>
    <button
      type="button"
      class="btn btn-secondary inline-flex items-center gap-2"
      :disabled="ctx.apiKeyTesting.value || ctx.effectiveStoredApiKeyCount.value === 0 || ctx.pendingDeletedApiKeyCount.value > 0 || ctx.configForm.clear_api_key || ctx.configForm.api_keys_mode === 'replace'"
      @click="ctx.testApiKeys(false)"
    >
      <Icon name="shield" size="sm" />
      {{ ctx.storedApiKeyTestButtonText.value }}
    </button>
    <button
      v-if="ctx.configForm.api_key_configured"
      type="button"
      class="btn btn-secondary inline-flex items-center gap-2"
      @click="ctx.toggleClearApiKey"
    >
      <Icon :name="ctx.configForm.clear_api_key ? 'x' : 'trash'" size="sm" />
      {{ ctx.configForm.clear_api_key ? t('admin.riskControl.keepApiKey') : t('admin.riskControl.clearApiKey') }}
    </button>
  </div>
</template>

<script setup lang="ts">
import { inject } from 'vue'
import { useI18n } from 'vue-i18n'
import { Icon } from '@sub2api/plugin-sdk'
import { RiskControlKey } from './riskControlContext'

const { t } = useI18n()
const ctx = inject(RiskControlKey)!
</script>
