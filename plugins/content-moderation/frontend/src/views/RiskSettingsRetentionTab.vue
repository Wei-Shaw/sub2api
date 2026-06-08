<template>
  <div class="grid grid-cols-1 gap-5 lg:grid-cols-2">
    <div>
      <label class="input-label">{{ t('admin.riskControl.hitRetentionDays') }}</label>
      <input v-model.number="ctx.configForm.hit_retention_days" type="number" min="1" max="3650" class="input" />
    </div>
    <div>
      <label class="input-label">{{ t('admin.riskControl.nonHitRetentionDays') }}</label>
      <input v-model.number="ctx.configForm.non_hit_retention_days" type="number" min="1" max="3" class="input" />
    </div>
    <div class="rounded-lg border border-gray-100 p-4 text-sm text-gray-500 dark:border-dark-700 dark:text-gray-400 lg:col-span-2">
      <div class="flex flex-wrap items-center gap-3">
        <Icon name="database" size="md" class="text-gray-400" />
        <span>{{ t('admin.riskControl.cleanupStats', { hit: ctx.status.value?.last_cleanup_deleted_hit ?? 0, nonHit: ctx.status.value?.last_cleanup_deleted_non_hit ?? 0 }) }}</span>
      </div>
    </div>
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
