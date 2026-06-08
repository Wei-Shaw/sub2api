<template>
  <div class="grid grid-cols-1 gap-5 lg:grid-cols-2">
    <div>
      <label class="input-label">{{ t('admin.riskControl.workerCount') }}</label>
      <input v-model.number="ctx.configForm.worker_count" type="number" min="1" max="32" class="input" />
    </div>
    <div>
      <label class="input-label">{{ t('admin.riskControl.queueSize') }}</label>
      <input v-model.number="ctx.configForm.queue_size" type="number" min="100" max="100000" class="input" />
    </div>
    <div class="flex items-center justify-between rounded-lg border border-gray-100 p-4 dark:border-dark-700 lg:col-span-2">
      <div>
        <p class="text-sm font-medium text-gray-900 dark:text-white">{{ t('admin.riskControl.recordNonHits') }}</p>
        <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.riskControl.recordNonHitsHint') }}</p>
      </div>
      <Toggle v-model="ctx.configForm.record_non_hits" />
    </div>
    <div class="space-y-4 rounded-lg border border-gray-100 p-4 dark:border-dark-700 lg:col-span-2">
      <div class="flex items-center justify-between gap-4">
        <div>
          <p class="text-sm font-medium text-gray-900 dark:text-white">{{ t('admin.riskControl.preHashCheck') }}</p>
          <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.riskControl.preHashCheckHint') }}</p>
        </div>
        <Toggle v-model="ctx.configForm.pre_hash_check_enabled" />
      </div>
      <div class="rounded-lg bg-gray-50 p-3 dark:bg-dark-900/30">
        <div class="flex flex-col gap-3 xl:flex-row xl:items-center xl:justify-between">
          <div>
            <p class="text-sm font-medium text-gray-900 dark:text-white">
              {{ t('admin.riskControl.flaggedHashCount', { count: ctx.formatNumber(ctx.status.value?.flagged_hash_count ?? 0) }) }}
            </p>
            <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.riskControl.flaggedHashHint') }}</p>
          </div>
          <button
            type="button"
            class="text-semantic-danger btn btn-secondary inline-flex items-center justify-center gap-2"
            :disabled="ctx.hashActionLoading.value || (ctx.status.value?.flagged_hash_count ?? 0) === 0"
            @click="ctx.clearFlaggedHashes"
          >
            <Icon name="trash" size="sm" :class="ctx.hashActionLoading.value ? 'animate-pulse' : ''" />
            {{ t('admin.riskControl.clearFlaggedHashes') }}
          </button>
        </div>
        <div class="mt-3 flex flex-col gap-2 sm:flex-row">
          <input
            v-model.trim="ctx.flaggedHashInput.value"
            type="text"
            class="input font-mono text-sm"
            :placeholder="t('admin.riskControl.flaggedHashPlaceholder')"
          />
          <button
            type="button"
            class="btn btn-secondary inline-flex items-center justify-center gap-2"
            :disabled="ctx.hashActionLoading.value || !ctx.isFlaggedHashInputValid.value"
            @click="ctx.deleteFlaggedHash"
          >
            <Icon name="trash" size="sm" />
            {{ t('admin.riskControl.deleteFlaggedHash') }}
          </button>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { inject } from 'vue'
import { useI18n } from 'vue-i18n'
import { Icon, Toggle } from '@sub2api/plugin-sdk'
import { RiskControlKey } from './riskControlContext'

const { t } = useI18n()
const ctx = inject(RiskControlKey)!
</script>
