<template>
  <div class="space-y-5">
    <div class="flex flex-col gap-3 lg:flex-row lg:items-center lg:justify-between">
      <div>
        <h3 class="text-base font-semibold text-gray-900 dark:text-white">{{ t('admin.riskControl.riskThresholds') }}</h3>
        <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">{{ t('admin.riskControl.riskThresholdsHint') }}</p>
      </div>
      <button
        type="button"
        class="btn btn-secondary inline-flex items-center justify-center gap-2"
        @click="ctx.resetRiskThresholds"
      >
        <Icon name="refresh" size="sm" />
        {{ t('admin.riskControl.riskThresholdReset') }}
      </button>
    </div>

    <div class="grid grid-cols-1 gap-3 md:grid-cols-2 xl:grid-cols-3">
      <div
        v-for="row in ctx.riskThresholdRows.value"
        :key="row.category"
        class="rounded-lg border border-gray-100 bg-gray-50 p-4 dark:border-dark-700 dark:bg-dark-900/30"
      >
        <div class="flex items-start justify-between gap-3">
          <div class="min-w-0">
            <label class="block truncate text-sm font-semibold text-gray-900 dark:text-white" :for="`risk-threshold-${row.category}`">
              {{ row.category }}
            </label>
            <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
              {{ t('admin.riskControl.riskThresholdDefault', { value: ctx.formatThresholdPercent(row.defaultValue) }) }}
            </p>
          </div>
          <span class="inline-flex shrink-0 rounded-md bg-white px-2 py-1 font-mono text-xs font-medium text-gray-600 shadow-sm dark:bg-dark-800 dark:text-gray-300">
            {{ ctx.formatThresholdPercent(row.value) }}
          </span>
        </div>
        <div class="mt-3">
          <label class="sr-only" :for="`risk-threshold-${row.category}`">
            {{ t('admin.riskControl.riskThresholdPercent') }}
          </label>
          <div class="relative">
            <input
              :id="`risk-threshold-${row.category}`"
              v-model.number="ctx.configForm.thresholds[row.category]"
              :data-test="`risk-threshold-${row.category}`"
              type="number"
              min="0"
              max="100"
              step="0.1"
              class="input pr-8 font-mono"
            />
            <span class="pointer-events-none absolute right-3 top-1/2 -translate-y-1/2 text-gray-400">%</span>
          </div>
        </div>
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
