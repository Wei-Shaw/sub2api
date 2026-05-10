<template>
  <div class="relative flex min-h-screen flex-col overflow-x-hidden bg-gray-50 dark:bg-dark-950">
    <PublicPageHeader active-path="/monitoring" />

    <main class="mx-auto w-full max-w-7xl flex-1 px-4 py-6 sm:px-6 sm:py-8">
      <div class="space-y-6">
        <div class="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
          <div>
            <h1 class="text-xl font-bold text-gray-900 dark:text-white sm:text-2xl">{{ t('admin.monitoring.title') }}</h1>
            <p class="mt-1 text-sm text-gray-500 dark:text-dark-400">{{ t('admin.monitoring.description') }}</p>
          </div>
          <div class="flex items-center gap-3">
            <span class="text-xs text-gray-400 dark:text-dark-500">{{ t('admin.monitoring.last24h') }}</span>
            <button @click="refresh" :disabled="loading"
              class="inline-flex items-center gap-2 rounded-lg border border-gray-300 bg-white px-4 py-2 text-sm font-medium text-gray-700 transition-colors hover:bg-gray-50 disabled:opacity-50 dark:border-dark-600 dark:bg-dark-800 dark:text-dark-200 dark:hover:bg-dark-700">
              <svg class="h-4 w-4" :class="{ 'animate-spin': loading }" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="1.5">
                <path stroke-linecap="round" stroke-linejoin="round" d="M16.023 9.348h4.992v-.001M2.985 19.644v-4.992m0 0h4.992m-4.993 0l3.181 3.183a8.25 8.25 0 0013.803-3.7M4.031 9.865a8.25 8.25 0 0113.803-3.7l3.181 3.182m0-4.991v4.99" />
              </svg>
              {{ t('common.refresh') }}
            </button>
          </div>
        </div>

        <!-- Summary Cards -->
        <div class="grid grid-cols-2 gap-3 sm:grid-cols-5 sm:gap-4">
          <div class="rounded-xl border border-gray-200 bg-white p-3 shadow-sm dark:border-dark-700 dark:bg-dark-900 sm:p-5">
            <p class="text-xs text-gray-500 dark:text-dark-400 sm:text-sm">{{ t('admin.monitoring.todayRequests') }}</p>
            <p class="mt-1 text-lg font-bold text-gray-900 dark:text-white sm:text-2xl">{{ summary?.total_requests_today?.toLocaleString() ?? '-' }}</p>
          </div>
          <div class="rounded-xl border border-gray-200 bg-white p-3 shadow-sm dark:border-dark-700 dark:bg-dark-900 sm:p-5">
            <p class="text-xs text-green-600 dark:text-green-400 sm:text-sm">{{ t('admin.monitoring.success') }}</p>
            <p class="mt-1 text-lg font-bold text-green-600 dark:text-green-400 sm:text-2xl">{{ summary?.success_count_today?.toLocaleString() ?? '-' }}</p>
          </div>
          <div class="rounded-xl border border-gray-200 bg-white p-3 shadow-sm dark:border-dark-700 dark:bg-dark-900 sm:p-5">
            <p class="text-xs text-red-500 dark:text-red-400 sm:text-sm">{{ t('admin.monitoring.errors') }}</p>
            <p class="mt-1 text-lg font-bold text-red-600 dark:text-red-400 sm:text-2xl">{{ summary?.error_count_today?.toLocaleString() ?? '-' }}</p>
          </div>
          <div class="rounded-xl border border-gray-200 bg-white p-3 shadow-sm dark:border-dark-700 dark:bg-dark-900 sm:p-5">
            <p class="text-xs text-gray-500 dark:text-dark-400 sm:text-sm">{{ t('admin.monitoring.avgLatency') }}</p>
            <p class="mt-1 text-lg font-bold text-gray-900 dark:text-white sm:text-2xl">{{ summary ? Math.round(summary.avg_latency_ms_today) + 'ms' : '-' }}</p>
          </div>
          <div class="rounded-xl border border-gray-200 bg-white p-3 shadow-sm dark:border-dark-700 dark:bg-dark-900 sm:p-5">
            <p class="text-xs text-gray-500 dark:text-dark-400 sm:text-sm">{{ t('admin.monitoring.totalGroups') }}</p>
            <p class="mt-1 text-lg font-bold text-gray-900 dark:text-white sm:text-2xl">{{ summary?.groups?.length ?? 0 }}</p>
          </div>
        </div>

        <!-- 24h Hourly Success Rate Bar -->
        <div class="rounded-xl border border-gray-200 bg-white p-4 shadow-sm dark:border-dark-700 dark:bg-dark-900 sm:p-6">
          <div class="mb-4 flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between">
            <p class="text-sm font-medium text-gray-700 dark:text-dark-200">{{ t('admin.monitoring.hourlySuccessRate') }}</p>
            <div class="flex flex-wrap items-center gap-x-3 gap-y-1 text-xs text-gray-400 dark:text-dark-500 sm:gap-4">
              <span class="flex items-center gap-1.5"><span class="inline-block h-2.5 w-2.5 rounded-sm bg-green-500"></span> ≥95%</span>
              <span class="flex items-center gap-1.5"><span class="inline-block h-2.5 w-2.5 rounded-sm bg-amber-500"></span> ≥80%</span>
              <span class="flex items-center gap-1.5"><span class="inline-block h-2.5 w-2.5 rounded-sm bg-red-500"></span> &lt;80%</span>
              <span class="flex items-center gap-1.5"><span class="inline-block h-2.5 w-2.5 rounded-sm bg-gray-200 dark:bg-dark-700"></span> {{ t('admin.monitoring.noData') }}</span>
            </div>
          </div>
          <div class="flex gap-[3px]" style="height: 40px;">
            <div v-for="(h, i) in hourlyData" :key="i"
              class="relative flex-1 rounded transition-all cursor-pointer group hover:opacity-80"
              :class="hourColor(h.rate)"
              :title="hourTooltip(i, h)">
              <div class="pointer-events-none absolute bottom-full left-1/2 z-10 mb-2 -translate-x-1/2 whitespace-nowrap rounded-lg bg-gray-900 px-3 py-1.5 text-xs text-white opacity-0 shadow-lg transition-opacity group-hover:opacity-100 dark:bg-dark-600">
                {{ hourTooltip(i, h) }}
                <div class="absolute top-full left-1/2 -translate-x-1/2 border-4 border-transparent border-t-gray-900 dark:border-t-dark-600"></div>
              </div>
            </div>
          </div>
          <div class="mt-2 flex justify-between text-[10px] tabular-nums text-gray-400 dark:text-dark-500">
            <span>{{ hourLabel(0) }}</span>
            <span>{{ hourLabel(6) }}</span>
            <span>{{ hourLabel(12) }}</span>
            <span>{{ hourLabel(18) }}</span>
            <span>{{ hourLabel(23) }}</span>
          </div>
        </div>

        <!-- Group × Model Matrix -->
        <div v-for="group in groupedModels" :key="group.groupId" class="rounded-xl border border-gray-200 bg-white shadow-sm dark:border-dark-700 dark:bg-dark-900 overflow-hidden">
          <div class="flex items-center gap-3 border-b border-gray-100 px-4 py-3 dark:border-dark-700 sm:px-6 sm:py-4">
            <span class="text-base font-semibold text-gray-900 dark:text-white sm:text-lg">{{ group.groupName }}</span>
            <span class="rounded-full bg-gray-100 px-2.5 py-0.5 text-xs font-medium text-gray-600 dark:bg-dark-800 dark:text-dark-300">
              {{ group.models.length }} {{ group.models.length === 1 ? 'model' : 'models' }}
            </span>
          </div>
          <div>
            <div class="hidden sm:block overflow-x-auto">
              <table class="w-full" style="table-layout: fixed;">
                <colgroup>
                  <col style="width: 22%;">
                  <col style="width: 8%;">
                  <col style="width: 7%;">
                  <col style="width: 7%;">
                  <col style="width: 14%;">
                  <col style="width: 22%;">
                  <col style="width: 7%;">
                  <col style="width: 7%;">
                  <col style="width: 6%;">
                </colgroup>
                <thead>
                  <tr class="border-b border-gray-100 bg-gray-50 dark:border-dark-700 dark:bg-dark-800">
                    <th class="px-6 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-dark-400">{{ t('admin.monitoring.model') }}</th>
                    <th class="px-4 py-3 text-center text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-dark-400">{{ t('admin.monitoring.requests') }}</th>
                    <th class="px-3 py-3 text-center text-xs font-medium uppercase tracking-wider text-green-600 dark:text-green-400">{{ t('admin.monitoring.success') }}</th>
                    <th class="px-3 py-3 text-center text-xs font-medium uppercase tracking-wider text-red-500 dark:text-red-400">{{ t('admin.monitoring.errors') }}</th>
                    <th class="px-4 py-3 text-center text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-dark-400">{{ t('admin.monitoring.successRate') }}</th>
                    <th class="px-2 py-3 text-center text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-dark-400">{{ t('admin.monitoring.hourlySuccessRate') }}</th>
                    <th class="px-4 py-3 text-right text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-dark-400">AVG</th>
                    <th class="px-4 py-3 text-right text-xs font-medium uppercase tracking-wider text-amber-500 dark:text-amber-400">P95</th>
                    <th class="px-4 py-3 text-right text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-dark-400">TTFT</th>
                  </tr>
                </thead>
                <tbody>
                  <tr v-for="m in group.models" :key="m.model" class="border-b border-gray-50 dark:border-dark-800">
                    <td class="px-6 py-3 text-sm font-medium text-gray-900 dark:text-white font-mono truncate" :title="m.model">{{ m.model }}</td>
                    <td class="px-4 py-3 text-center text-sm text-gray-700 dark:text-dark-200 tabular-nums">{{ m.request_count.toLocaleString() }}</td>
                    <td class="px-3 py-3 text-center">
                      <span class="text-sm font-semibold tabular-nums" :class="m.success_count > 0 ? 'text-green-600 dark:text-green-400' : 'text-gray-400'">{{ m.success_count }}</span>
                    </td>
                    <td class="px-3 py-3 text-center">
                      <span class="text-sm font-semibold tabular-nums" :class="m.error_count > 0 ? 'text-red-600 dark:text-red-400' : 'text-sm text-gray-400'">{{ m.error_count }}</span>
                    </td>
                    <td class="px-4 py-3 text-center">
                      <div class="flex items-center justify-center gap-2">
                        <div class="h-1.5 w-16 overflow-hidden rounded-full bg-gray-200 dark:bg-dark-700 flex-shrink-0">
                          <div class="h-full rounded-full" :class="successRateColor(m.success_count, m.request_count)" :style="{ width: successRateWidth(m.success_count, m.request_count) }"></div>
                        </div>
                        <span class="text-sm tabular-nums whitespace-nowrap" :class="successRateTextColor(m.success_count, m.request_count)">{{ successRate(m.success_count, m.request_count) }}</span>
                      </div>
                    </td>
                    <td class="px-2 py-3">
                      <div class="flex gap-[1px]" style="height: 16px;">
                        <div v-for="(h, hi) in getModelHourly(m.group_id, m.model)" :key="hi"
                          class="relative flex-1 rounded-[1px] transition-colors cursor-pointer group/cell"
                          :class="hourColorMini(h.rate)"
                          :title="modelHourTooltip(m.model, hi, h)">
                          <div class="pointer-events-none absolute bottom-full left-1/2 z-20 mb-1 -translate-x-1/2 whitespace-nowrap rounded bg-gray-900 px-2 py-1 text-[10px] text-white opacity-0 shadow-lg group-hover/cell:opacity-100 dark:bg-dark-600">
                            {{ modelHourTooltip(m.model, hi, h) }}
                          </div>
                        </div>
                      </div>
                    </td>
                    <td class="px-4 py-3 text-right text-sm text-gray-700 dark:text-dark-200 tabular-nums">{{ formatMs(m.avg_latency_ms) }}</td>
                    <td class="px-4 py-3 text-right text-sm tabular-nums" :class="m.p95_latency_ms > 10000 ? 'text-amber-600 dark:text-amber-400 font-semibold' : 'text-gray-700 dark:text-dark-200'">{{ formatMs(m.p95_latency_ms) }}</td>
                    <td class="px-4 py-3 text-right text-sm text-gray-700 dark:text-dark-200 tabular-nums">{{ formatMs(m.avg_ttft) }}</td>
                  </tr>
                </tbody>
              </table>
            </div>
            <div class="sm:hidden divide-y divide-gray-100 dark:divide-dark-700">
              <div v-for="m in group.models" :key="'m-'+m.model" class="px-4 py-3 space-y-2">
                <div class="flex items-center justify-between">
                  <p class="text-sm font-medium text-gray-900 dark:text-white font-mono break-all">{{ m.model }}</p>
                  <span class="ml-2 flex-shrink-0 text-sm tabular-nums font-semibold" :class="successRateTextColor(m.success_count, m.request_count)">{{ successRate(m.success_count, m.request_count) }}</span>
                </div>
                <div class="flex items-center gap-3 text-xs text-gray-500 dark:text-dark-400">
                  <span>{{ m.request_count.toLocaleString() }} {{ t('admin.monitoring.requests') }}</span>
                  <span class="text-green-600 dark:text-green-400">{{ m.success_count }} {{ t('admin.monitoring.success') }}</span>
                  <span :class="m.error_count > 0 ? 'text-red-500' : ''">{{ m.error_count }} {{ t('admin.monitoring.errors') }}</span>
                </div>
                <div class="flex gap-[1px]" style="height: 16px;">
                  <div v-for="(h, hi) in getModelHourly(m.group_id, m.model)" :key="hi"
                    class="relative flex-1 rounded-[1px] transition-colors"
                    :class="hourColorMini(h.rate)"
                    :title="modelHourTooltip(m.model, hi, h)">
                  </div>
                </div>
                <div class="flex items-center gap-3 text-xs">
                  <span class="text-gray-500 dark:text-dark-400">AVG {{ formatMs(m.avg_latency_ms) }}</span>
                  <span :class="m.p95_latency_ms > 10000 ? 'text-amber-600 dark:text-amber-400 font-medium' : 'text-gray-500 dark:text-dark-400'">P95 {{ formatMs(m.p95_latency_ms) }}</span>
                  <span class="text-gray-500 dark:text-dark-400">TTFT {{ formatMs(m.avg_ttft) }}</span>
                </div>
              </div>
            </div>
          </div>
        </div>

        <div v-if="summary && (!groupModels?.group_models || groupModels.group_models.length === 0)" class="rounded-xl border border-gray-200 bg-white py-16 text-center shadow-sm dark:border-dark-700 dark:bg-dark-900">
          <p class="text-sm text-gray-500 dark:text-dark-400">{{ t('admin.monitoring.noData') }}</p>
        </div>

        <!-- Model Latency Overview -->
        <div v-if="modelLatency?.model_latencies?.length" class="rounded-xl border border-gray-200 bg-white shadow-sm dark:border-dark-700 dark:bg-dark-900 overflow-hidden">
          <div class="flex items-center gap-3 border-b border-gray-100 px-4 py-3 dark:border-dark-700 sm:px-6 sm:py-4">
            <h2 class="text-base font-semibold text-gray-900 dark:text-white sm:text-lg">{{ t('admin.monitoring.modelLatency') }}</h2>
            <span class="text-xs text-gray-400 dark:text-dark-500">{{ t('admin.monitoring.modelLatencyHint') }}</span>
          </div>
          <div>
            <div class="hidden sm:block overflow-x-auto">
              <table class="w-full">
                <thead>
                  <tr class="border-b border-gray-100 bg-gray-50 dark:border-dark-700 dark:bg-dark-800">
                    <th class="px-6 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-dark-400">{{ t('admin.monitoring.model') }}</th>
                    <th class="px-6 py-3 text-center text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-dark-400">{{ t('admin.monitoring.requests') }}</th>
                    <th class="px-6 py-3 text-center text-xs font-medium uppercase tracking-wider text-green-600 dark:text-green-400">{{ t('admin.monitoring.success') }}</th>
                    <th class="px-6 py-3 text-center text-xs font-medium uppercase tracking-wider text-red-500 dark:text-red-400">{{ t('admin.monitoring.errors') }}</th>
                    <th class="px-6 py-3 text-center text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-dark-400">{{ t('admin.monitoring.successRate') }}</th>
                    <th class="px-6 py-3 text-right text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-dark-400">AVG</th>
                    <th class="px-6 py-3 text-right text-xs font-medium uppercase tracking-wider text-amber-500 dark:text-amber-400">P95</th>
                    <th class="px-6 py-3 text-right text-xs font-medium uppercase tracking-wider text-red-500 dark:text-red-400">P99</th>
                    <th class="px-6 py-3 text-right text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-dark-400">TTFT</th>
                  </tr>
                </thead>
                <tbody>
                  <tr v-for="m in modelLatency.model_latencies" :key="m.model" class="border-b border-gray-50 dark:border-dark-800">
                    <td class="px-6 py-3 text-sm font-medium text-gray-900 dark:text-white font-mono">{{ m.model }}</td>
                    <td class="px-6 py-3 text-center text-sm text-gray-700 dark:text-dark-200">{{ m.request_count.toLocaleString() }}</td>
                    <td class="px-6 py-3 text-center">
                      <span class="text-sm font-semibold" :class="m.success_count > 0 ? 'text-green-600 dark:text-green-400' : 'text-gray-400'">{{ m.success_count }}</span>
                    </td>
                    <td class="px-6 py-3 text-center">
                      <span :class="m.error_count > 0 ? 'text-sm font-semibold text-red-600 dark:text-red-400' : 'text-sm text-gray-400'">{{ m.error_count }}</span>
                    </td>
                    <td class="px-6 py-3 text-center">
                      <div class="flex items-center justify-center gap-2">
                        <div class="h-1.5 w-16 overflow-hidden rounded-full bg-gray-200 dark:bg-dark-700">
                          <div class="h-full rounded-full" :class="successRateColor(m.success_count, m.request_count)" :style="{ width: successRateWidth(m.success_count, m.request_count) }"></div>
                        </div>
                        <span class="text-sm" :class="successRateTextColor(m.success_count, m.request_count)">{{ successRate(m.success_count, m.request_count) }}</span>
                      </div>
                    </td>
                    <td class="px-6 py-3 text-right text-sm text-gray-700 dark:text-dark-200">{{ formatMs(m.avg_latency_ms) }}</td>
                    <td class="px-6 py-3 text-right text-sm" :class="m.p95_latency_ms > 10000 ? 'text-amber-600 dark:text-amber-400 font-semibold' : 'text-gray-700 dark:text-dark-200'">{{ formatMs(m.p95_latency_ms) }}</td>
                    <td class="px-6 py-3 text-right text-sm" :class="m.p99_latency_ms > 30000 ? 'text-red-600 dark:text-red-400 font-semibold' : 'text-gray-700 dark:text-dark-200'">{{ formatMs(m.p99_latency_ms) }}</td>
                    <td class="px-6 py-3 text-right text-sm text-gray-700 dark:text-dark-200">{{ formatMs(m.avg_first_token_ms) }}</td>
                  </tr>
                </tbody>
              </table>
            </div>
            <div class="sm:hidden divide-y divide-gray-100 dark:divide-dark-700">
              <div v-for="m in modelLatency.model_latencies" :key="'ml-'+m.model" class="px-4 py-3 space-y-2">
                <div class="flex items-center justify-between">
                  <p class="text-sm font-medium text-gray-900 dark:text-white font-mono break-all">{{ m.model }}</p>
                  <span class="ml-2 flex-shrink-0 text-sm tabular-nums font-semibold" :class="successRateTextColor(m.success_count, m.request_count)">{{ successRate(m.success_count, m.request_count) }}</span>
                </div>
                <div class="flex items-center gap-3 text-xs text-gray-500 dark:text-dark-400">
                  <span>{{ m.request_count.toLocaleString() }} {{ t('admin.monitoring.requests') }}</span>
                  <span class="text-green-600 dark:text-green-400">{{ m.success_count }} {{ t('admin.monitoring.success') }}</span>
                  <span :class="m.error_count > 0 ? 'text-red-500' : ''">{{ m.error_count }} {{ t('admin.monitoring.errors') }}</span>
                </div>
                <div class="flex flex-wrap items-center gap-x-3 gap-y-1 text-xs">
                  <span class="text-gray-500 dark:text-dark-400">AVG {{ formatMs(m.avg_latency_ms) }}</span>
                  <span :class="m.p95_latency_ms > 10000 ? 'text-amber-600 dark:text-amber-400 font-medium' : 'text-gray-500 dark:text-dark-400'">P95 {{ formatMs(m.p95_latency_ms) }}</span>
                  <span :class="m.p99_latency_ms > 30000 ? 'text-red-600 dark:text-red-400 font-medium' : 'text-gray-500 dark:text-dark-400'">P99 {{ formatMs(m.p99_latency_ms) }}</span>
                  <span class="text-gray-500 dark:text-dark-400">TTFT {{ formatMs(m.avg_first_token_ms) }}</span>
                </div>
              </div>
            </div>
          </div>
        </div>
      </div>
    </main>

    <PublicPageFooter />
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import PublicPageHeader from '@/components/common/PublicPageHeader.vue'
import PublicPageFooter from '@/components/common/PublicPageFooter.vue'
import { monitoringAPI, type MonitoringSummary, type MonitoringGroupModels, type MonitoringModelLatency, type GroupModelStats } from '@/api/admin/monitoring'

const { t } = useI18n()

const summary = ref<MonitoringSummary | null>(null)
const groupModels = ref<MonitoringGroupModels | null>(null)
const modelLatency = ref<MonitoringModelLatency | null>(null)
const loading = ref(false)

interface GroupedModels {
  groupId: number
  groupName: string
  models: GroupModelStats[]
}

const groupedModels = computed<GroupedModels[]>(() => {
  if (!groupModels.value?.group_models) return []
  const map = new Map<number, GroupedModels>()
  for (const m of groupModels.value.group_models) {
    let g = map.get(m.group_id)
    if (!g) {
      g = { groupId: m.group_id, groupName: m.group_name, models: [] }
      map.set(m.group_id, g)
    }
    g.models.push(m)
  }
  return Array.from(map.values())
})

async function refresh() {
  loading.value = true
  try {
    const [summaryRes, gmRes, mlRes] = await Promise.all([
      monitoringAPI.getSummary().catch(() => null),
      monitoringAPI.getGroupModels().catch(() => null),
      monitoringAPI.getModelLatency().catch(() => null),
    ])
    summary.value = summaryRes
    groupModels.value = gmRes
    modelLatency.value = mlRes
  } finally {
    loading.value = false
  }
}

function formatMs(ms: number): string {
  if (!ms || ms === 0) return '-'
  if (ms < 1000) return Math.round(ms) + 'ms'
  return (ms / 1000).toFixed(1) + 's'
}

function successRate(success: number, total: number): string {
  if (total === 0) return '-'
  return (success / total * 100).toFixed(1) + '%'
}

function successRateWidth(success: number, total: number): string {
  if (total === 0) return '0%'
  return Math.min(success / total * 100, 100) + '%'
}

function successRateColor(success: number, total: number): string {
  if (total === 0) return 'bg-gray-300'
  const rate = success / total
  if (rate >= 0.95) return 'bg-green-500'
  if (rate >= 0.8) return 'bg-amber-500'
  return 'bg-red-500'
}

function successRateTextColor(success: number, total: number): string {
  if (total === 0) return 'text-gray-400'
  const rate = success / total
  if (rate >= 0.95) return 'text-green-600 dark:text-green-400'
  if (rate >= 0.8) return 'text-amber-600 dark:text-amber-400'
  return 'text-red-600 dark:text-red-400'
}

interface HourBucket {
  total: number
  success: number
  rate: number | null
}

const hourlyData = computed<HourBucket[]>(() => {
  const buckets: HourBucket[] = []
  const now = new Date()
  for (let i = 23; i >= 0; i--) {
    const hourStart = new Date(now.getTime() - (i + 1) * 3600000)
    const hourEnd = new Date(now.getTime() - i * 3600000)
    let total = 0
    let success = 0
    if (summary.value?.hourly_stats) {
      for (const h of summary.value.hourly_stats) {
        const d = new Date(h.hour)
        if (d >= hourStart && d < hourEnd) {
          total = h.total
          success = h.success
          break
        }
      }
    }
    buckets.push({ total, success, rate: total > 0 ? success / total : null })
  }
  return buckets
})

function getModelHourly(groupId: number, model: string): HourBucket[] {
  const now = new Date()
  const stats = groupModels.value?.model_hourly_stats ?? []
  const buckets: HourBucket[] = []
  for (let i = 23; i >= 0; i--) {
    const hourStart = new Date(now.getTime() - (i + 1) * 3600000)
    const hourEnd = new Date(now.getTime() - i * 3600000)
    let total = 0
    let success = 0
    for (const h of stats) {
      if (h.group_id !== groupId || h.model !== model) continue
      const d = new Date(h.hour)
      if (d >= hourStart && d < hourEnd) {
        total = h.total
        success = h.success
        break
      }
    }
    buckets.push({ total, success, rate: total > 0 ? success / total : null })
  }
  return buckets
}

function hourColor(rate: number | null): string {
  if (rate === null) return 'bg-gray-200 dark:bg-dark-700'
  if (rate >= 0.95) return 'bg-green-500'
  if (rate >= 0.8) return 'bg-amber-500'
  return 'bg-red-500'
}

function hourColorMini(rate: number | null): string {
  if (rate === null) return 'bg-gray-200 dark:bg-dark-700'
  if (rate >= 0.95) return 'bg-green-400'
  if (rate >= 0.8) return 'bg-amber-400'
  return 'bg-red-400'
}

function hourLabel(index: number): string {
  const now = new Date()
  const hourStart = new Date(now.getTime() - (24 - index) * 3600000)
  return hourStart.getHours().toString().padStart(2, '0') + ':00'
}

function hourTooltip(index: number, h: HourBucket): string {
  const now = new Date()
  const hourStart = new Date(now.getTime() - (24 - index) * 3600000)
  const hStr = hourStart.getHours().toString().padStart(2, '0') + ':00'
  if (h.rate === null) return `${hStr} - ${t('admin.monitoring.noData')}`
  return `${hStr} | ${h.total} ${t('admin.monitoring.requests')} | ${(h.rate * 100).toFixed(1)}% ${t('admin.monitoring.successRate')}`
}

function modelHourTooltip(model: string, index: number, h: HourBucket): string {
  const now = new Date()
  const hourStart = new Date(now.getTime() - (24 - index) * 3600000)
  const hStr = hourStart.getHours().toString().padStart(2, '0') + ':00'
  if (h.rate === null) return `${model} ${hStr} - ${t('admin.monitoring.noData')}`
  return `${model} ${hStr} | ${h.total} req | ${(h.rate * 100).toFixed(1)}%`
}

onMounted(refresh)
</script>
