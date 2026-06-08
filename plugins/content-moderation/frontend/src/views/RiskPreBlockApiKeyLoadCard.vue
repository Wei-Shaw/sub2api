<template>
  <div data-test="pre-block-api-key-load-card" class="card">
    <div class="flex flex-col gap-4 border-b border-gray-100 px-6 py-4 dark:border-dark-700 lg:flex-row lg:items-center lg:justify-between">
      <div>
        <h2 class="text-lg font-semibold text-gray-900 dark:text-white">{{ t('admin.riskControl.preBlockAPIKeyLoad') }}</h2>
        <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
          {{ t('admin.riskControl.preBlockAPIKeyLoadHint') }}
        </p>
      </div>
      <span class="inline-flex w-fit items-center rounded-full bg-gray-100 px-2.5 py-1 text-xs font-medium text-gray-600 dark:bg-dark-700 dark:text-gray-300">
        {{ ctx.preBlockAPIKeyLoadSummaryText.value }}
      </span>
    </div>

    <div class="p-6">
      <div
        v-if="ctx.preBlockAPIKeyLoads.value.length > 0"
        data-test="pre-block-api-key-load-list"
        class="max-h-[280px] space-y-3 overflow-y-auto pr-1"
      >
        <div
          v-for="item in ctx.preBlockAPIKeyLoads.value"
          :key="item.key_hash || item.index"
          class="rounded-lg bg-gray-50 p-3 dark:bg-dark-700/50"
        >
          <div class="flex min-w-0 flex-col gap-2 sm:flex-row sm:items-center sm:justify-between">
            <div class="min-w-0">
              <div class="flex min-w-0 items-center gap-2">
                <span class="font-mono text-sm font-semibold text-gray-900 dark:text-white">#{{ item.index + 1 }}</span>
                <span class="truncate font-mono text-sm text-gray-700 dark:text-gray-200">{{ item.masked || '-' }}</span>
                <span class="h-2 w-2 flex-shrink-0 rounded-full" :class="ctx.apiKeyStatusDotClass(item.status)"></span>
              </div>
              <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
                {{ t('admin.riskControl.preBlockAPIKeyTotals', { total: ctx.formatNumber(item.total), success: ctx.formatNumber(item.success), errors: ctx.formatNumber(item.errors) }) }}
              </p>
            </div>
            <div class="grid grid-cols-4 gap-2 text-right text-xs text-gray-500 dark:text-gray-400 sm:min-w-[280px]">
              <div>
                <p>{{ t('admin.riskControl.preBlockKeyActiveShort') }}</p>
                <p class="mt-1 text-sm font-semibold text-sky-700 dark:text-sky-300">{{ ctx.formatNumber(item.active) }}</p>
              </div>
              <div>
                <p>{{ t('admin.riskControl.preBlockKeyTotalShort') }}</p>
                <p class="mt-1 text-sm font-semibold text-gray-900 dark:text-white">{{ ctx.formatNumber(item.total) }}</p>
              </div>
              <div>
                <p>{{ t('admin.riskControl.preBlockKeyAvgShort') }}</p>
                <p class="mt-1 text-sm font-semibold text-gray-900 dark:text-white">{{ ctx.formatNumber(item.avg_latency_ms) }} ms</p>
              </div>
              <div>
                <p>{{ t('admin.riskControl.preBlockKeyLastShort') }}</p>
                <p class="mt-1 text-sm font-semibold text-gray-900 dark:text-white">{{ ctx.formatNumber(item.last_latency_ms) }} ms</p>
              </div>
            </div>
          </div>
          <div class="mt-3 h-1.5 overflow-hidden rounded-full bg-white dark:bg-dark-900">
            <div class="h-full rounded-full bg-sky-500" :style="{ width: ctx.preBlockAPIKeyLoadWidth(item.total) }"></div>
          </div>
        </div>
      </div>
      <p v-else class="rounded-lg bg-gray-50 p-4 text-sm text-gray-500 dark:bg-dark-700/50 dark:text-gray-400">
        {{ t('admin.riskControl.preBlockAPIKeyLoadEmpty') }}
      </p>
    </div>
  </div>
</template>

<script setup lang="ts">
import { inject } from 'vue'
import { useI18n } from 'vue-i18n'
import { RiskControlKey } from './riskControlContext'

const { t } = useI18n()
const ctx = inject(RiskControlKey)!
</script>
