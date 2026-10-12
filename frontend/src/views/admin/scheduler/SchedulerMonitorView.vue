<template>
  <AppLayout>
    <div class="w-full min-w-0 space-y-6 pb-12">
      <header class="flex flex-col gap-4 rounded-3xl bg-white p-6 shadow-sm ring-1 ring-gray-900/5 dark:bg-dark-800 dark:ring-dark-700">
        <div class="flex flex-wrap items-center justify-between gap-4 border-b border-gray-100 pb-4 dark:border-dark-700">
          <div class="min-w-0">
            <h1 class="flex items-center gap-2 text-xl font-black text-gray-900 dark:text-white">
              <span class="inline-flex h-8 w-8 items-center justify-center rounded-xl bg-blue-50 text-blue-500 dark:bg-blue-900/30 dark:text-blue-400">
                <Icon name="chartBar" size="sm" />
              </span>
              {{ t('admin.ops.scheduler.title') }}
            </h1>
            <div class="mt-1 flex flex-wrap items-center gap-3 text-xs text-gray-500 dark:text-gray-400">
              <span class="flex items-center gap-1.5">
                <span class="h-2 w-2 rounded-full" :class="loading ? 'bg-gray-400' : 'bg-green-500'" />
                {{ loading ? t('admin.ops.loadingText') : t('admin.ops.ready') }}
              </span>
              <span>·</span>
              <span>{{ t('admin.ops.scheduler.description') }}</span>
              <template v-if="lastUpdated">
                <span>·</span>
                <span>{{ t('admin.ops.scheduler.updatedAt', { time: formatDateTime(lastUpdated) }) }}</span>
              </template>
            </div>
          </div>

          <div class="flex flex-wrap items-center gap-3">
            <label class="flex h-8 items-center gap-2 rounded-lg bg-gray-100 px-3 text-xs font-semibold text-gray-700 dark:bg-dark-700 dark:text-gray-300">
              <input v-model="autoRefresh" type="checkbox" class="h-4 w-4 rounded border-gray-300 text-primary-600 focus:ring-primary-500 dark:border-dark-600 dark:bg-dark-800" />
              {{ t('admin.ops.scheduler.autoRefresh') }}
            </label>
            <button
              type="button"
              class="btn btn-secondary btn-sm h-8"
              :disabled="loading"
              :title="t('admin.ops.scheduler.refresh')"
              @click="loadData"
            >
              <Icon name="refresh" size="sm" :class="{ 'animate-spin': loading }" />
              <span>{{ t('admin.ops.scheduler.refresh') }}</span>
            </button>
          </div>
        </div>
      </header>

      <div v-if="errorMessage" class="rounded-2xl bg-red-50 p-4 text-sm text-red-600 dark:bg-red-900/20 dark:text-red-400">
        {{ errorMessage }}
      </div>

      <div v-if="loading && !hasLoaded" class="grid grid-cols-1 gap-4 sm:grid-cols-2 xl:grid-cols-4">
        <div v-for="index in 4" :key="index" class="h-28 animate-pulse rounded-2xl bg-gray-100 dark:bg-dark-800" />
      </div>

      <template v-else>
        <section class="grid grid-cols-1 gap-4 sm:grid-cols-2 xl:grid-cols-4">
          <article v-for="card in metricCards" :key="card.label" class="rounded-2xl bg-gray-50 p-4 dark:bg-dark-900">
            <div class="flex items-start justify-between gap-3">
              <span class="text-xs font-bold uppercase tracking-wider text-gray-400">{{ card.label }}</span>
              <span :class="['inline-flex h-8 w-8 items-center justify-center rounded-xl', card.iconClass]">
                <Icon :name="card.icon" size="sm" />
              </span>
            </div>
            <div class="mt-2 text-3xl font-black tabular-nums text-gray-900 dark:text-white">{{ card.value }}</div>
            <p class="mt-1 text-xs leading-5 text-gray-500 dark:text-gray-400">{{ card.hint }}</p>
          </article>
        </section>

        <p class="px-1 text-xs leading-5 text-gray-500 dark:text-gray-400">
          {{ t('admin.ops.scheduler.cooldownHint') }}
        </p>

        <section class="grid grid-cols-1 gap-6 xl:grid-cols-5">
          <div class="rounded-3xl bg-white p-6 shadow-sm ring-1 ring-gray-900/5 dark:bg-dark-800 dark:ring-dark-700 xl:col-span-3">
            <div class="border-b border-gray-100 pb-4 dark:border-dark-700">
              <h2 class="text-base font-bold text-gray-900 dark:text-white">{{ t('admin.ops.scheduler.accountStatus') }}</h2>
              <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.ops.scheduler.accountStatusHint') }}</p>
            </div>

            <div v-if="accountRows.length === 0" class="pt-8 text-center text-sm text-gray-500 dark:text-gray-400">
              {{ t('admin.ops.scheduler.noData') }}
            </div>
            <div v-else class="mt-4 overflow-x-auto rounded-2xl bg-gray-50 dark:bg-dark-900/40">
              <table class="min-w-full text-left text-xs">
                <thead class="bg-gray-100 text-[11px] uppercase tracking-wide text-gray-500 dark:bg-dark-900 dark:text-gray-400">
                  <tr>
                    <th class="px-5 py-3 font-semibold sm:px-6">{{ t('admin.ops.scheduler.account') }}</th>
                    <th class="px-3 py-3 font-semibold">{{ t('admin.ops.scheduler.group') }}</th>
                    <th class="px-3 py-3 font-semibold">{{ t('admin.ops.scheduler.status') }}</th>
                    <th class="px-5 py-3 text-right font-semibold sm:px-6">{{ t('admin.ops.scheduler.concurrency') }}</th>
                  </tr>
                </thead>
                <tbody class="divide-y divide-gray-100 dark:divide-dark-700">
                  <tr v-for="row in accountRows" :key="row.id" class="text-gray-700 dark:text-gray-300">
                    <td class="max-w-[240px] px-5 py-3 sm:px-6">
                      <div class="truncate font-semibold text-gray-900 dark:text-white" :title="row.name">{{ row.name }}</div>
                      <div class="mt-0.5 font-mono text-[11px] text-gray-400">#{{ row.id }}</div>
                    </td>
                    <td class="max-w-[140px] truncate px-3 py-3 text-gray-500 dark:text-gray-400" :title="row.group">{{ row.group || '-' }}</td>
                    <td class="px-3 py-3">
                      <span :class="['inline-flex items-center rounded-md px-2 py-1 text-[11px] font-semibold', statusClass(row.status)]">
                        {{ statusLabel(row.status) }}
                      </span>
                    </td>
                    <td class="px-5 py-3 text-right sm:px-6">
                      <div class="font-mono font-semibold text-gray-900 dark:text-white">{{ row.current }}/{{ row.max }}</div>
                      <div class="mt-1 h-1.5 w-24 overflow-hidden rounded-full bg-gray-100 dark:bg-dark-700 sm:ml-auto">
                        <div class="h-full rounded-full transition-all" :class="loadBarClass(row.load)" :style="{ width: `${Math.min(100, row.load)}%` }" />
                      </div>
                    </td>
                  </tr>
                </tbody>
              </table>
            </div>
          </div>

          <div class="rounded-3xl bg-white p-6 shadow-sm ring-1 ring-gray-900/5 dark:bg-dark-800 dark:ring-dark-700 xl:col-span-2">
            <div class="border-b border-gray-100 pb-4 dark:border-dark-700">
              <h2 class="text-base font-bold text-gray-900 dark:text-white">{{ t('admin.ops.scheduler.cooldownList') }}</h2>
              <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.ops.scheduler.cooldownListHint') }}</p>
            </div>

            <div v-if="cooldownRows.length === 0" class="pt-8 text-center text-sm text-gray-500 dark:text-gray-400">
              {{ t('admin.ops.scheduler.empty') }}
            </div>
            <div v-else class="mt-4 divide-y divide-gray-200 overflow-hidden rounded-2xl bg-gray-50 dark:divide-dark-700 dark:bg-dark-900/40">
              <div v-for="row in cooldownRows" :key="`${row.account_id}-${row.model}`" class="p-4">
                <div class="flex items-start justify-between gap-3">
                  <div class="min-w-0">
                    <div class="truncate text-sm font-semibold text-gray-900 dark:text-white" :title="row.accountName">{{ row.accountName }}</div>
                    <div class="mt-1 break-all font-mono text-xs text-gray-500 dark:text-gray-400">{{ row.model === '*' ? (locale.startsWith('zh') ? '整个账号' : 'Entire account') : row.model }}</div>
                    <div class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ recoveryReasonLabel(row, locale) }}</div>
                  </div>
                  <span class="shrink-0 rounded-md bg-amber-100 px-2 py-1 text-[11px] font-semibold text-amber-700 dark:bg-amber-900/30 dark:text-amber-300">
                    {{ recoveryStateLabel(row, locale) }}
                  </span>
                </div>
                <div class="mt-3 flex flex-wrap gap-x-4 gap-y-1 text-[11px] text-gray-500 dark:text-gray-400">
                  <span v-if="!row.state || row.state === 'cooldown'">{{ t('admin.ops.scheduler.remaining') }} <strong class="font-mono text-amber-700 dark:text-amber-300">{{ formatRemaining(row.remaining_seconds) }}</strong></span>
                  <span v-else>{{ locale.startsWith('zh') ? '验证进度' : 'Recovery progress' }} {{ row.recovery_successes || 0 }}/{{ row.recovery_required || 2 }} · {{ locale.startsWith('zh') ? '进行中' : 'In flight' }} {{ row.probes_in_flight || 0 }}</span>
                  <span v-if="row.probe_started_at">{{ locale.startsWith('zh') ? '试探开始' : 'Probe started' }} {{ new Date(row.probe_started_at).toLocaleString() }}</span>
                  <span>{{ t('admin.ops.scheduler.streak') }} <strong class="font-mono text-gray-700 dark:text-gray-200">{{ row.failure_streak }}</strong></span>
                </div>
              </div>
            </div>
          </div>
        </section>

        <section class="rounded-3xl bg-white p-6 shadow-sm ring-1 ring-gray-900/5 dark:bg-dark-800 dark:ring-dark-700">
          <div class="border-b border-gray-100 pb-4 dark:border-dark-700">
            <h2 class="text-base font-bold text-gray-900 dark:text-white">{{ t('admin.ops.scheduler.recentErrors') }}</h2>
          </div>
          <div v-if="recentErrors.length === 0" class="pt-8 text-center text-sm text-gray-500 dark:text-gray-400">
            {{ t('admin.ops.scheduler.noErrors') }}
          </div>
          <div v-else class="mt-4 overflow-x-auto rounded-2xl bg-gray-50 dark:bg-dark-900/40">
            <table class="min-w-full text-left text-xs">
              <thead class="bg-gray-100 text-[11px] uppercase tracking-wide text-gray-500 dark:bg-dark-900 dark:text-gray-400">
                <tr>
                  <th class="px-5 py-3 font-semibold sm:px-6">{{ t('admin.ops.scheduler.lastFailure') }}</th>
                  <th class="px-3 py-3 font-semibold">{{ t('admin.ops.scheduler.account') }}</th>
                  <th class="px-3 py-3 font-semibold">{{ t('admin.ops.scheduler.model') }}</th>
                  <th class="px-3 py-3 font-semibold">{{ t('admin.ops.scheduler.upstreamStatus') }}</th>
                  <th class="max-w-[420px] px-5 py-3 font-semibold sm:px-6">{{ t('admin.ops.scheduler.reason') }}</th>
                </tr>
              </thead>
              <tbody class="divide-y divide-gray-100 dark:divide-dark-700">
                <tr v-for="row in recentErrors" :key="row.id" class="text-gray-700 dark:text-gray-300">
                  <td class="whitespace-nowrap px-5 py-3 text-gray-500 dark:text-gray-400 sm:px-6">{{ formatDateTime(row.created_at) }}</td>
                  <td class="max-w-[180px] truncate px-3 py-3 font-medium text-gray-900 dark:text-white" :title="row.account_name">{{ row.account_name || '-' }}</td>
                  <td class="max-w-[180px] truncate px-3 py-3 font-mono text-[11px]" :title="row.model">{{ row.model || '-' }}</td>
                  <td class="px-3 py-3">
                    <span class="rounded-md bg-red-100 px-2 py-1 font-mono font-semibold text-red-700 dark:bg-red-900/30 dark:text-red-300">{{ row.status_code || '-' }}</span>
                  </td>
                  <td class="max-w-[420px] truncate px-5 py-3 text-gray-500 dark:text-gray-400 sm:px-6" :title="row.message">{{ row.message || '-' }}</td>
                </tr>
              </tbody>
            </table>
          </div>
        </section>
      </template>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import Icon from '@/components/icons/Icon.vue'
import {
  opsAPI,
  type OpsAccountAvailabilityStatsResponse,
  type OpsConcurrencyStatsResponse,
  type OpsDashboardOverview,
  type OpsErrorLog,
  type OpsModelCooldown,
  type OpsSchedulerSnapshot
} from '@/api/admin/ops'
import { formatDateTime as formatDateTimeValue } from '@/utils/format'
import { affectedAccountCount, recoveryReasonLabel, recoveryStateLabel } from './schedulerRecovery'

type AccountStatus = 'normal' | 'rate_limited' | 'overloaded' | 'error' | 'disabled'

interface AccountRow {
  id: number
  name: string
  group: string
  current: number
  max: number
  load: number
  status: AccountStatus
}

interface CooldownRow extends OpsModelCooldown {
  accountName: string
}

const { t, locale } = useI18n()
const loading = ref(false)
const hasLoaded = ref(false)
const errorMessage = ref('')
const autoRefresh = ref(true)
const lastUpdated = ref<string>('')
const scheduler = ref<OpsSchedulerSnapshot | null>(null)
const availability = ref<OpsAccountAvailabilityStatsResponse | null>(null)
const concurrency = ref<OpsConcurrencyStatsResponse | null>(null)
const overview = ref<OpsDashboardOverview | null>(null)
const recentErrors = ref<OpsErrorLog[]>([])
let refreshTimer: ReturnType<typeof setInterval> | undefined

const metricCards = computed(() => {
  const metrics = scheduler.value?.metrics
  return [
    {
      label: t('admin.ops.scheduler.switchAttempts'),
      value: formatNumber(metrics?.account_switch_total),
      hint: t('admin.ops.accountSwitches'),
      icon: 'swap' as const,
      iconClass: 'bg-blue-50 text-blue-600 dark:bg-blue-900/30 dark:text-blue-400'
    },
    {
      label: locale.value.startsWith('zh') ? '受影响账号' : 'Affected accounts',
      value: formatNumber(affectedAccountCount(scheduler.value?.model_cooldowns ?? [])),
      hint: locale.value.startsWith('zh') ? '冷却或恢复验证中的账号' : 'Accounts cooling down or verifying recovery',
      icon: 'clock' as const,
      iconClass: 'bg-amber-50 text-amber-600 dark:bg-amber-900/30 dark:text-amber-400'
    },
    {
      label: t('admin.ops.scheduler.requestErrors'),
      value: formatNumber(overview.value?.error_count_total),
      hint: t('admin.ops.errors'),
      icon: 'exclamationTriangle' as const,
      iconClass: 'bg-red-50 text-red-600 dark:bg-red-900/30 dark:text-red-400'
    },
    {
      label: t('admin.ops.scheduler.ttftP95'),
      value: formatMilliseconds(overview.value?.ttft?.p95_ms),
      hint: t('admin.ops.ttftLabel'),
      icon: 'bolt' as const,
      iconClass: 'bg-primary-50 text-primary-600 dark:bg-primary-900/30 dark:text-primary-400'
    }
  ]
})

const accountRows = computed<AccountRow[]>(() => {
  const availabilityMap = availability.value?.account ?? {}
  const concurrencyMap = concurrency.value?.account ?? {}
  const ids = new Set([...Object.keys(availabilityMap), ...Object.keys(concurrencyMap)])

  return Array.from(ids).map((key) => {
    const id = Number(key)
    const available = availabilityMap[key]
    const active = concurrencyMap[key]
    const current = safeNumber(active?.current_in_use)
    const max = safeNumber(active?.max_capacity)
    const load = safeNumber(active?.load_percentage)

    let status: AccountStatus = 'normal'
    if (available?.has_error) status = 'error'
    else if (available?.is_rate_limited) status = 'rate_limited'
    else if (available?.is_overloaded) status = 'overloaded'
    else if (available && !available.is_available) status = 'disabled'

    return {
      id,
      name: active?.account_name || available?.account_name || `#${id}`,
      group: active?.group_name || available?.group_name || '',
      current,
      max,
      load,
      status
    }
  }).sort((a, b) => statusWeight(b.status) - statusWeight(a.status) || b.load - a.load)
})

const cooldownRows = computed<CooldownRow[]>(() => {
  const names = new Map(accountRows.value.map((row) => [row.id, row.name]))
  return (scheduler.value?.model_cooldowns ?? []).map((row) => ({
    ...row,
    accountName: names.get(row.account_id) || `#${row.account_id}`
  }))
})

function safeNumber(value: unknown): number {
  return typeof value === 'number' && Number.isFinite(value) ? value : 0
}

function formatNumber(value: unknown): string {
  const number = safeNumber(value)
  return new Intl.NumberFormat().format(Math.round(number))
}

function formatMilliseconds(value: number | null | undefined): string {
  if (typeof value !== 'number' || !Number.isFinite(value)) return '-'
  if (value >= 1000) return `${(value / 1000).toFixed(2)}s`
  return `${Math.round(value)}ms`
}

function formatDateTime(value: string): string {
  return value ? formatDateTimeValue(value) : '-'
}

function formatRemaining(seconds: number): string {
  if (!Number.isFinite(seconds) || seconds <= 0) return '0s'
  if (seconds < 60) return `${Math.ceil(seconds)}s`
  const minutes = Math.floor(seconds / 60)
  const remainder = Math.ceil(seconds % 60)
  if (minutes < 60) return `${minutes}m${remainder ? ` ${remainder}s` : ''}`
  return `${Math.floor(minutes / 60)}h${minutes % 60 ? ` ${minutes % 60}m` : ''}`
}

function statusWeight(status: AccountStatus): number {
  return { error: 5, rate_limited: 4, overloaded: 3, disabled: 2, normal: 1 }[status]
}

function statusLabel(status: AccountStatus): string {
  return t(`admin.ops.scheduler.${status === 'rate_limited' ? 'rateLimited' : status}`)
}

function statusClass(status: AccountStatus): string {
  return {
    normal: 'bg-green-100 text-green-700 dark:bg-green-900/30 dark:text-green-300',
    rate_limited: 'bg-amber-100 text-amber-700 dark:bg-amber-900/30 dark:text-amber-300',
    overloaded: 'bg-orange-100 text-orange-700 dark:bg-orange-900/30 dark:text-orange-300',
    error: 'bg-red-100 text-red-700 dark:bg-red-900/30 dark:text-red-300',
    disabled: 'bg-gray-100 text-gray-600 dark:bg-dark-700 dark:text-gray-300'
  }[status]
}

function loadBarClass(load: number): string {
  if (load >= 90) return 'bg-red-500'
  if (load >= 70) return 'bg-orange-500'
  if (load >= 50) return 'bg-amber-400'
  return 'bg-green-500'
}

async function loadData(): Promise<void> {
  if (loading.value) return
  loading.value = true
  errorMessage.value = ''
  try {
    const [schedulerData, availabilityData, concurrencyData, overviewData, errorsData] = await Promise.all([
      opsAPI.getSchedulerSnapshot(),
      opsAPI.getAccountAvailabilityStats('openai'),
      opsAPI.getConcurrencyStats('openai'),
      opsAPI.getDashboardOverview({ time_range: '1h', platform: 'openai', mode: 'raw' }),
      opsAPI.listUpstreamErrors({ time_range: '1h', platform: 'openai', page: 1, page_size: 8, view: 'errors' })
    ])
    scheduler.value = schedulerData
    availability.value = availabilityData
    concurrency.value = concurrencyData
    overview.value = overviewData
    recentErrors.value = errorsData.items ?? []
    lastUpdated.value = schedulerData.generated_at || new Date().toISOString()
    hasLoaded.value = true
  } catch (error: unknown) {
    console.error('[SchedulerMonitorView] Failed to load data', error)
    errorMessage.value = t('admin.ops.scheduler.loadFailed')
  } finally {
    loading.value = false
  }
}

function stopAutoRefresh(): void {
  if (refreshTimer) {
    clearInterval(refreshTimer)
    refreshTimer = undefined
  }
}

function startAutoRefresh(): void {
  stopAutoRefresh()
  if (autoRefresh.value) refreshTimer = setInterval(() => void loadData(), 60_000)
}

watch(autoRefresh, startAutoRefresh)

onMounted(async () => {
  await loadData()
  startAutoRefresh()
})

onUnmounted(stopAutoRefresh)
</script>
