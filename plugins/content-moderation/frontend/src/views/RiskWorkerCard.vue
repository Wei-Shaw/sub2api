<template>
  <div v-if="ctx.showWorkerRuntimeCard.value" class="card">
    <div class="flex flex-col gap-4 border-b border-gray-100 px-6 py-4 dark:border-dark-700 lg:flex-row lg:items-center lg:justify-between">
      <div>
        <h2 class="text-lg font-semibold text-gray-900 dark:text-white">{{ t('admin.riskControl.workerStatus') }}</h2>
        <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">{{ t('admin.riskControl.workerStatusHint') }}</p>
      </div>
      <div class="flex flex-wrap items-center gap-2 text-sm text-gray-500 dark:text-gray-400">
        <span>{{ t('admin.riskControl.autoRefresh') }}</span>
        <span v-if="ctx.status.value?.last_cleanup_at">
          {{ t('admin.riskControl.lastCleanup', { time: ctx.formatDateTime(ctx.status.value.last_cleanup_at) }) }}
        </span>
      </div>
    </div>

    <div class="grid grid-cols-1 gap-6 p-6 xl:grid-cols-[minmax(0,360px)_1fr]">
      <div class="space-y-4">
        <div class="rounded-lg border border-gray-100 p-4 dark:border-dark-700">
          <div class="flex items-center justify-between gap-3">
            <div>
              <p class="text-sm font-medium text-gray-900 dark:text-white">{{ t('admin.riskControl.queueUsage') }}</p>
              <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
                {{ ctx.formatNumber(ctx.status.value?.queue_length ?? 0) }} / {{ ctx.formatNumber(ctx.status.value?.queue_size ?? ctx.configForm.queue_size) }}
              </p>
            </div>
            <span class="text-sm font-semibold text-gray-900 dark:text-white">{{ ctx.queueUsagePercent.value }}</span>
          </div>
          <div class="mt-4 h-2 overflow-hidden rounded-full bg-gray-100 dark:bg-dark-700">
            <div class="h-full rounded-full bg-primary-500 transition-all duration-300" :style="ctx.queueUsageStyle.value"></div>
          </div>
        </div>

        <div class="grid grid-cols-2 gap-3">
          <div class="rounded-lg bg-gray-50 p-4 dark:bg-dark-700/50">
            <p class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.riskControl.activeWorkers') }}</p>
            <p class="mt-2 text-2xl font-semibold text-gray-900 dark:text-white">{{ ctx.status.value?.active_workers ?? 0 }}</p>
          </div>
          <div class="surface-success rounded-lg p-4">
            <p class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.riskControl.idleWorkers') }}</p>
            <p class="text-semantic-success mt-2 text-2xl font-semibold">{{ ctx.status.value?.idle_workers ?? ctx.configForm.worker_count }}</p>
          </div>
          <div class="rounded-lg bg-gray-50 p-4 dark:bg-dark-700/50">
            <p class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.riskControl.processed') }}</p>
            <p class="mt-2 text-2xl font-semibold text-gray-900 dark:text-white">{{ ctx.formatNumber(ctx.status.value?.processed ?? 0) }}</p>
          </div>
          <div class="rounded-lg bg-gray-50 p-4 dark:bg-dark-700/50">
            <p class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.riskControl.droppedErrors') }}</p>
            <p class="mt-2 text-2xl font-semibold text-gray-900 dark:text-white">{{ ctx.formatNumber((ctx.status.value?.dropped ?? 0) + (ctx.status.value?.errors ?? 0)) }}</p>
          </div>
        </div>
      </div>

      <div>
        <div class="mb-3 flex items-center justify-between gap-3">
          <div>
            <p class="text-sm font-medium text-gray-900 dark:text-white">{{ t('admin.riskControl.workerPool') }}</p>
            <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
              {{ t('admin.riskControl.workerPoolMeta', { active: ctx.status.value?.active_workers ?? 0, idle: ctx.status.value?.idle_workers ?? ctx.configForm.worker_count, total: ctx.status.value?.worker_count ?? ctx.configForm.worker_count }) }}
            </p>
          </div>
          <span class="inline-flex items-center rounded-full bg-gray-100 px-2.5 py-1 text-xs font-medium text-gray-600 dark:bg-dark-700 dark:text-gray-300">
            {{ ctx.modeLabel(ctx.status.value?.mode ?? ctx.configForm.mode) }}
          </span>
        </div>
        <div class="grid grid-cols-2 gap-2 sm:grid-cols-4 md:grid-cols-6 xl:grid-cols-8 2xl:grid-cols-10">
          <div
            v-for="worker in ctx.workerSlots.value"
            :key="worker.id"
            class="flex h-12 items-center justify-between rounded-lg border px-3 transition-colors"
            :class="ctx.workerSlotClass(worker.state)"
            :title="worker.label"
          >
            <span class="text-sm font-semibold">#{{ worker.id }}</span>
            <span class="h-2.5 w-2.5 rounded-full" :class="ctx.workerDotClass(worker.state)"></span>
          </div>
        </div>
      </div>
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
