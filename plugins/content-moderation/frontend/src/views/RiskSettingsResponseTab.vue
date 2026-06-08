<template>
  <div class="space-y-5">
    <div class="grid grid-cols-1 gap-5 lg:grid-cols-2">
      <div>
        <label class="input-label">{{ t('admin.riskControl.blockStatus') }}</label>
        <input v-model.number="ctx.configForm.block_status" type="number" min="400" max="599" class="input" />
      </div>
      <div>
        <label class="input-label">{{ t('admin.riskControl.blockMessage') }}</label>
        <input v-model.trim="ctx.configForm.block_message" type="text" class="input" />
      </div>
      <div class="flex items-center justify-between rounded-lg border border-gray-100 p-4 dark:border-dark-700">
        <div>
          <p class="text-sm font-medium text-gray-900 dark:text-white">{{ t('admin.riskControl.emailOnHit') }}</p>
          <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.riskControl.emailOnHitHint') }}</p>
        </div>
        <Toggle v-model="ctx.configForm.email_on_hit" />
      </div>
      <div class="flex items-center justify-between rounded-lg border border-gray-100 p-4 dark:border-dark-700">
        <div>
          <p class="text-sm font-medium text-gray-900 dark:text-white">{{ t('admin.riskControl.autoBan') }}</p>
          <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.riskControl.autoBanHint') }}</p>
        </div>
        <Toggle v-model="ctx.configForm.auto_ban_enabled" />
      </div>
      <div>
        <label class="input-label">{{ t('admin.riskControl.banThreshold') }}</label>
        <input v-model.number="ctx.configForm.ban_threshold" type="number" min="1" max="1000" class="input" />
      </div>
      <div>
        <label class="input-label">{{ t('admin.riskControl.violationWindowHours') }}</label>
        <input v-model.number="ctx.configForm.violation_window_hours" type="number" min="1" max="8760" class="input" />
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { inject } from 'vue'
import { useI18n } from 'vue-i18n'
import { Toggle } from '@sub2api/plugin-sdk'
import { RiskControlKey } from './riskControlContext'

const { t } = useI18n()
const ctx = inject(RiskControlKey)!
</script>
