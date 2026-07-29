<template>
  <section aria-labelledby="prompt-runtime-title" class="border-b border-gray-200 py-6 dark:border-dark-700/60">
    <div class="flex flex-wrap items-start justify-between gap-3">
      <div>
        <h2 id="prompt-runtime-title" class="text-base font-semibold text-gray-950 dark:text-white">
          {{ t('admin.promptAudit.runtime.title') }}
        </h2>
        <p class="mt-1 text-sm text-gray-500 dark:text-dark-300">
          {{ t('admin.promptAudit.runtime.description') }}
        </p>
      </div>
      <button type="button" class="btn btn-secondary btn-sm" :disabled="loading" @click="$emit('refresh')">
        {{ t('admin.promptAudit.actions.refresh') }}
      </button>
    </div>

    <div v-if="error" role="alert" class="mt-5 rounded-lg bg-red-50 px-4 py-3 text-sm text-red-700 dark:bg-red-950/30 dark:text-red-300">
      {{ error }}
    </div>
    <div v-else-if="loading && !runtime" class="mt-5 grid grid-cols-2 gap-3 md:grid-cols-3 xl:grid-cols-6" aria-busy="true">
      <div v-for="index in 6" :key="index" class="h-16 animate-pulse rounded-xl bg-gray-100 dark:bg-dark-800" />
    </div>
    <template v-else-if="runtime">
      <dl class="mt-5 grid grid-cols-2 gap-3 md:grid-cols-3 xl:grid-cols-6">
        <div v-for="item in statusItems" :key="item.label" class="rounded-xl border border-gray-100 bg-gray-50/80 px-3 py-3 dark:border-dark-700/60 dark:bg-dark-900/40">
          <dt class="text-xs text-gray-500 dark:text-dark-400">{{ item.label }}</dt>
          <dd class="mt-1.5 flex items-center gap-2 text-sm font-semibold text-gray-900 dark:text-white">
            <span v-if="item.dot" class="h-2 w-2 shrink-0 rounded-full" :class="item.dot" />
            <span class="min-w-0 truncate">{{ item.value }}</span>
          </dd>
        </div>
      </dl>

      <div class="mt-4 grid gap-3 lg:grid-cols-[minmax(0,1.4fr)_minmax(220px,0.6fr)]">
        <div class="rounded-xl border border-gray-100 px-4 py-3 dark:border-dark-700/60 dark:bg-dark-900/20">
          <h3 class="text-sm font-medium text-gray-900 dark:text-white">{{ t('admin.promptAudit.runtime.guardMetrics') }}</h3>
          <div class="mt-3 grid grid-cols-2 gap-2 sm:grid-cols-4">
            <div v-for="metric in guardMetricItems" :key="metric.label" class="rounded-lg bg-gray-50 px-2.5 py-2 dark:bg-dark-900/60">
              <p class="text-[11px] text-gray-500 dark:text-dark-400">{{ metric.label }}</p>
              <p class="mt-0.5 text-sm font-semibold tabular-nums text-gray-900 dark:text-white">{{ metric.value }}</p>
            </div>
          </div>
          <p class="mt-3 text-xs leading-5 text-gray-500 dark:text-dark-400">
            {{ t('admin.promptAudit.runtime.queueBreakdown', {
              queued: runtime.queue.queued,
              processing: runtime.queue.processing,
              retry: runtime.queue.retry,
              done: runtime.queue.done,
              failed: runtime.queue.failed,
            }) }}
            <span class="mx-1.5 text-gray-300 dark:text-dark-600">·</span>
            {{ t('admin.promptAudit.runtime.deliveryTotals', { enqueued: runtime.enqueued_total, dropped: runtime.dropped_total, processed: runtime.processed_total, failed: runtime.failed_total }) }}
          </p>
        </div>
        <div class="rounded-xl border border-gray-100 px-4 py-3 dark:border-dark-700/60 dark:bg-dark-900/20">
          <h3 class="text-sm font-medium text-gray-900 dark:text-white">{{ t('admin.promptAudit.runtime.latest') }}</h3>
          <p class="mt-2 text-sm text-gray-600 dark:text-dark-300">
            {{ runtime.last_processed_at ? formatDate(runtime.last_processed_at) : t('admin.promptAudit.common.never') }}
          </p>
          <p v-if="runtime.last_error_code" class="mt-1 break-words text-sm text-red-600 dark:text-red-300">
            {{ runtime.last_error_code }}<span v-if="runtime.last_error_message"> · {{ runtime.last_error_message }}</span>
          </p>
          <div v-if="Object.keys(runtime.endpoints).length" class="mt-3 flex flex-wrap gap-2">
            <span v-for="(probe, id) in runtime.endpoints" :key="id" class="rounded-md px-2 py-1 text-xs" :class="probe.ok ? 'bg-emerald-50 text-emerald-700 dark:bg-emerald-950/40 dark:text-emerald-300' : 'bg-red-50 text-red-700 dark:bg-red-950/40 dark:text-red-300'">
              {{ id }} · {{ probe.status }} · {{ probe.latency_ms }} ms
            </span>
          </div>
        </div>
      </div>
    </template>
  </section>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import type { PromptAuditRuntime } from '../types'

const props = defineProps<{ runtime: PromptAuditRuntime | null; loading: boolean; error: string }>()
defineEmits<{ (event: 'refresh'): void }>()
const { t, locale } = useI18n()

const statusItems = computed(() => {
  const runtime = props.runtime
  if (!runtime) return []
  return [
    { label: t('admin.promptAudit.runtime.process'), value: t(`admin.promptAudit.status.${runtime.process_status}`), dot: statusDot(runtime.process_status) },
    { label: t('admin.promptAudit.runtime.mode'), value: t(`admin.promptAudit.mode.${runtime.effective_mode}`) },
    { label: t('admin.promptAudit.runtime.version'), value: `${runtime.active_config_version} / ${runtime.expected_config_version}` },
    { label: t('admin.promptAudit.runtime.workers'), value: `${runtime.worker_active} / ${runtime.worker_total}` },
    { label: t('admin.promptAudit.runtime.queue'), value: `${runtime.queue.active} / ${runtime.queue_capacity}` },
    { label: t('admin.promptAudit.runtime.dependencies'), value: `DB ${runtime.database_status} · Redis ${runtime.redis_status}` },
  ]
})

const guardMetricItems = computed(() => {
  const metrics = props.runtime?.guard_metrics
  if (!metrics) return []
  return [
    { label: t('admin.promptAudit.metrics.total'), value: metrics.total },
    { label: t('admin.promptAudit.metrics.allowed'), value: metrics.allowed },
    { label: t('admin.promptAudit.metrics.flagged'), value: metrics.flagged },
    { label: t('admin.promptAudit.metrics.blocked'), value: metrics.blocked },
    { label: t('admin.promptAudit.metrics.unavailable'), value: metrics.unavailable },
    { label: t('admin.promptAudit.metrics.timeouts'), value: metrics.timeouts },
    { label: t('admin.promptAudit.metrics.failovers'), value: metrics.failovers },
    { label: 'P95', value: metrics.latency_p95_ms != null ? `${metrics.latency_p95_ms} ms` : '—' },
  ]
})

function formatDate(value: string): string {
  return new Intl.DateTimeFormat(locale.value, { dateStyle: 'medium', timeStyle: 'medium' }).format(new Date(value))
}

function statusDot(status: string): string {
  if (status === 'running') return 'bg-emerald-500'
  if (status === 'disabled') return 'bg-gray-400'
  if (status === 'degraded') return 'bg-amber-500'
  return 'bg-red-500'
}
</script>
