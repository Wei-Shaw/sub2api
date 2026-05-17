<template>
  <AppLayout>
    <section class="mx-auto w-full max-w-[1500px] space-y-6 px-4 sm:px-6 lg:px-8">
      <div
        data-testid="channel-status-header-grid"
        class="grid gap-6"
        :class="showQuickNav ? 'xl:grid-cols-[minmax(0,1fr)_180px]' : ''"
      >
        <div class="flex flex-col gap-4 md:flex-row md:items-end md:justify-between">
          <div>
            <p class="text-xs font-semibold uppercase tracking-[0.24em] text-slate-400">
              DevRouter Status
            </p>
            <div class="mt-2 flex flex-wrap items-center gap-3">
              <h1 class="text-2xl font-bold tracking-tight text-slate-950 dark:text-white md:text-3xl">
                {{ t('channelStatus.title') }}
              </h1>
              <span
                v-if="featureEnabled"
                class="inline-flex items-center text-sm font-medium"
                :class="overallInlineClass"
              >
                <span class="mr-2 h-1.5 w-1.5 rounded-full" :class="overallDotClass"></span>
                {{ globalStatusMessage }}
              </span>
            </div>
          </div>

          <div v-if="featureEnabled" class="flex items-center gap-3">
            <button
              type="button"
              class="inline-flex h-9 w-9 items-center justify-center rounded-lg border border-slate-200 bg-white text-slate-500 shadow-sm transition hover:-translate-y-0.5 hover:border-slate-300 hover:text-slate-950 dark:border-white/10 dark:bg-white/5 dark:text-slate-300 dark:hover:bg-white/10 disabled:opacity-50"
              :disabled="loading"
              :title="t('common.refresh')"
              @click="manualReload"
            >
              <Icon name="refresh" size="md" :class="loading ? 'animate-spin' : ''" />
            </button>
            <AutoRefreshButton
              :enabled="autoRefresh.enabled.value"
              :interval-seconds="autoRefresh.intervalSeconds.value"
              :countdown="autoRefresh.countdown.value"
              :intervals="autoRefresh.intervals"
              @update:enabled="autoRefresh.setEnabled"
              @update:interval="autoRefresh.setInterval"
            />
          </div>
        </div>
      </div>

      <div
        v-if="!featureEnabled"
        class="rounded-xl border border-dashed border-slate-200 bg-white/80 p-10 text-center shadow-sm dark:border-white/10 dark:bg-white/[0.03]"
      >
        <h2 class="text-base font-semibold text-slate-950 dark:text-white">
          {{ t('channelStatus.disabled.title') }}
        </h2>
        <p class="mt-2 text-sm text-slate-500 dark:text-slate-400">
          {{ t('channelStatus.disabled.description') }}
        </p>
      </div>

      <div v-else-if="!loading && items.length === 0" class="rounded-xl border border-dashed border-slate-200 bg-white/80 p-10 text-center shadow-sm dark:border-white/10 dark:bg-white/[0.03]">
        <h2 class="text-base font-semibold text-slate-950 dark:text-white">
          {{ t('channelStatus.empty.title') }}
        </h2>
        <p class="mt-2 text-sm text-slate-500 dark:text-slate-400">
          {{ t('channelStatus.empty.description') }}
        </p>
      </div>

      <div
        v-else
        data-testid="channel-status-content-grid"
        class="grid gap-6"
        :class="showQuickNav ? 'xl:grid-cols-[minmax(0,1fr)_180px]' : ''"
      >
        <div class="channel-monitor-flat-list space-y-10">
          <section
            v-for="item in sortedItems"
            :id="channelAnchorId(item)"
            :key="item.id"
            class="scroll-mt-24"
          >
            <div class="channel-monitor-section-header flex flex-col gap-3 border-b border-slate-300/70 bg-slate-100/70 px-3 py-2 dark:border-white/10 dark:bg-white/[0.04] sm:flex-row sm:items-center sm:justify-between">
              <div class="flex min-w-0 items-center gap-3">
                <span class="h-1.5 w-1.5 rounded-full" :class="channelStatusClass(item).dot"></span>
                <div class="min-w-0">
                  <h2 class="truncate text-sm font-bold text-slate-900 dark:text-white">
                    {{ item.name }}
                  </h2>
                  <p class="mt-0.5 truncate text-[11px] text-slate-500 dark:text-slate-400">
                    {{ providerLabel(item.provider) }}
                    <span v-if="item.group_name" class="mx-1 text-slate-300">/</span>
                    <span v-if="item.group_name">{{ item.group_name }}</span>
                  </p>
                </div>
              </div>

              <span
                class="inline-flex items-center rounded-full border px-2.5 py-1 text-xs font-semibold"
                :class="channelStatusClass(item).chip"
              >
                <span class="mr-2 h-1.5 w-1.5 rounded-full" :class="channelStatusClass(item).dot"></span>
                {{ channelSummary(item).label }}
              </span>
            </div>

            <div class="divide-y divide-slate-100 bg-white dark:divide-white/10 dark:bg-transparent">
              <div
                v-if="detailLoading[item.id] && !detailCache[item.id]"
                class="px-3 py-4 text-sm text-slate-500 dark:text-slate-400"
              >
                Loading...
              </div>

              <div
                v-for="model in detailCache[item.id]?.models || fallbackModels(item)"
                :key="model.model"
                class="channel-monitor-model-row grid items-center gap-3 px-3 py-3 transition hover:bg-slate-50/80 dark:hover:bg-white/[0.03] lg:grid-cols-[minmax(120px,160px)_minmax(360px,1fr)_minmax(260px,300px)]"
              >
                <div class="min-w-0">
                  <div class="flex items-center gap-2">
                    <span class="h-1.5 w-1.5 rounded-full" :class="modelDotClass(model.latest_status)"></span>
                    <span class="truncate font-mono text-sm font-semibold text-slate-800 dark:text-slate-100">
                      {{ model.model }}
                    </span>
                  </div>
                </div>

                <MonitorTimeline
                  class="mt-0 pt-0"
                  :buckets="model.timeline || []"
                  :countdown-seconds="countdown"
                />

                <div class="grid min-w-[260px] grid-cols-2 items-center gap-4 text-sm">
                  <span class="inline-flex min-w-0 items-baseline justify-end gap-1.5 whitespace-nowrap">
                    <span class="shrink-0 text-xs text-slate-400 dark:text-slate-500">
                      {{ t('channelStatus.latestLatency') }}
                    </span>
                    <span class="font-mono text-slate-600 dark:text-slate-300">
                      {{ formatLatencyMs(model.latest_latency_ms) }}
                    </span>
                  </span>
                  <span class="inline-flex min-w-0 items-baseline justify-end gap-1.5 whitespace-nowrap">
                    <span class="shrink-0 text-xs text-slate-400 dark:text-slate-500">
                      {{ t('channelStatus.columns.availability7d') }}
                    </span>
                    <span class="font-mono text-slate-600 dark:text-slate-300">
                      {{ formatPercent(model.availability_7d) }}
                    </span>
                  </span>
                </div>
              </div>
            </div>
          </section>
        </div>

        <aside
          v-if="showQuickNav"
          class="channel-monitor-toc hidden xl:block"
        >
          <div class="sticky top-24 border-l border-slate-200 pl-4 dark:border-white/10">
            <p class="mb-3 text-xs font-semibold uppercase tracking-[0.2em] text-slate-400">
              {{ t('channelStatus.quickNav') }}
            </p>
            <nav class="space-y-1">
              <a
                v-for="item in sortedItems"
                :key="item.id"
                :href="`#${channelAnchorId(item)}`"
                class="block truncate rounded-md px-2 py-1.5 text-xs text-slate-500 transition hover:bg-white hover:text-slate-950 dark:text-slate-400 dark:hover:bg-white/10 dark:hover:text-white"
              >
                <span class="mr-1.5 inline-block h-1.5 w-1.5 rounded-full" :class="channelStatusClass(item).dot"></span>
                {{ item.name }}
              </a>
            </nav>
          </div>
        </aside>
      </div>
    </section>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores/app'
import { extractApiErrorMessage } from '@/utils/apiError'
import {
  list as listChannelMonitorViews,
  status as fetchChannelMonitorDetail,
  type UserMonitorDetail,
  type UserMonitorModelDetail,
  type UserMonitorView,
} from '@/api/channelMonitor'
import type { MonitorStatus } from '@/api/admin/channelMonitor'
import AppLayout from '@/components/layout/AppLayout.vue'
import AutoRefreshButton from '@/components/common/AutoRefreshButton.vue'
import Icon from '@/components/icons/Icon.vue'
import MonitorTimeline from '@/components/user/monitor/MonitorTimeline.vue'
import { DEFAULT_INTERVAL_SECONDS, STATUS_OPERATIONAL } from '@/constants/channelMonitor'
import { useAutoRefresh } from '@/composables/useAutoRefresh'
import { useChannelMonitorFormat } from '@/composables/useChannelMonitorFormat'

const { t } = useI18n()
const appStore = useAppStore()
const { providerLabel, formatLatency, formatPercent } = useChannelMonitorFormat()

const items = ref<UserMonitorView[]>([])
const loading = ref(false)
const settingsRevision = ref(0)
const detailCache = reactive<Record<number, UserMonitorDetail>>({})
const detailLoading = reactive<Record<number, boolean>>({})

let abortController: AbortController | null = null

const autoRefresh = useAutoRefresh({
  storageKey: 'channel-status-auto-refresh',
  intervals: [30, 60, 120] as const,
  defaultInterval: DEFAULT_INTERVAL_SECONDS,
  onRefresh: () => reload(true),
  shouldPause: () => document.hidden || loading.value,
})
const countdown = autoRefresh.countdown

const featureEnabled = computed(() => {
  void settingsRevision.value
  return appStore.cachedPublicSettings?.channel_monitor_enabled !== false
})

const overallState = computed<'operational' | 'degraded' | 'down'>(() => {
  if (items.value.length === 0) return 'operational'
  const summaries = items.value.map(channelSummary)
  if (summaries.every(s => s.normal === 0 && s.total > 0)) return 'down'
  if (summaries.some(s => s.abnormal > 0)) return 'degraded'
  return 'operational'
})

const overallDotClass = computed(() => statusPresentation(overallState.value).dot)
const overallInlineClass = computed(() => {
  if (overallState.value === 'down') return 'text-red-600 dark:text-red-300'
  if (overallState.value === 'degraded') return 'text-amber-700 dark:text-amber-300'
  return 'text-emerald-600 dark:text-emerald-300'
})
const sortedItems = computed(() => [...items.value].sort((a, b) => channelSeverity(b) - channelSeverity(a)))
const showQuickNav = computed(() => sortedItems.value.length > 10)
const globalStatusMessage = computed(() => {
  if (overallState.value === 'down') return t('channelStatus.global.unavailable')
  if (overallState.value === 'degraded') return t('channelStatus.global.degraded')
  return t('channelStatus.global.operational')
})

async function ensurePublicSettings() {
  if (!appStore.cachedPublicSettings) {
    const settings = await appStore.fetchPublicSettings()
    if (settings && !appStore.cachedPublicSettings) {
      appStore.cachedPublicSettings = settings
    }
  }
  settingsRevision.value += 1
}

async function reload(silent = false) {
  if (!featureEnabled.value) {
    items.value = []
    return
  }
  if (abortController) abortController.abort()
  const ctrl = new AbortController()
  abortController = ctrl
  if (!silent) loading.value = true
  try {
    const res = await listChannelMonitorViews({ signal: ctrl.signal })
    if (ctrl.signal.aborted || abortController !== ctrl) return
    items.value = res.items || []
  } catch (err: unknown) {
    const e = err as { name?: string; code?: string }
    if (e?.name === 'AbortError' || e?.code === 'ERR_CANCELED') return
    appStore.showError(extractApiErrorMessage(err, t('channelStatus.loadError')))
  } finally {
    if (abortController === ctrl) {
      if (!silent) loading.value = false
      countdown.value = DEFAULT_INTERVAL_SECONDS
      abortController = null
    }
  }
}

async function manualReload() {
  await reload(false)
  await loadAllDetails(true)
}

async function loadDetail(id: number, force = false) {
  if (!force && detailCache[id]) return
  detailLoading[id] = true
  try {
    detailCache[id] = await fetchChannelMonitorDetail(id)
  } catch (err: unknown) {
    appStore.showError(extractApiErrorMessage(err, t('channelStatus.detailLoadError')))
  } finally {
    detailLoading[id] = false
  }
}

async function loadAllDetails(force = false) {
  await Promise.all(items.value.map(item => loadDetail(item.id, force)))
}

function formatLatencyMs(value: number | null | undefined): string {
  const formatted = formatLatency(value)
  return formatted === '-' ? formatted : `${formatted}ms`
}

function allModelStatuses(item: UserMonitorView): MonitorStatus[] {
  return [item.primary_status, ...item.extra_models.map(m => m.status)].filter(Boolean) as MonitorStatus[]
}

function channelSummary(item: UserMonitorView) {
  const statuses = allModelStatuses(item)
  const total = statuses.length
  const normal = statuses.filter(s => s === STATUS_OPERATIONAL).length
  const abnormal = Math.max(0, total - normal)
  const state: 'operational' | 'degraded' | 'down' =
    total > 0 && normal === 0 ? 'down' : abnormal > 0 ? 'degraded' : 'operational'
  const label = state === 'down'
    ? t('channelStatus.summary.channelDown')
    : state === 'degraded'
      ? t('channelStatus.summary.partialUnavailable')
      : t('channelStatus.summary.allNormal')
  return { total, normal, abnormal, state, label }
}

function channelStatusClass(item: UserMonitorView) {
  return statusPresentation(channelSummary(item).state)
}

function statusRank(state: 'operational' | 'degraded' | 'down') {
  if (state === 'down') return 2
  if (state === 'degraded') return 1
  return 0
}

function channelSeverity(item: UserMonitorView) {
  const summary = channelSummary(item)
  const primaryPenalty = item.primary_status && item.primary_status !== STATUS_OPERATIONAL ? 0.5 : 0
  return statusRank(summary.state) * 10 + summary.abnormal + primaryPenalty
}

function channelAnchorId(item: UserMonitorView) {
  return `channel-monitor-${item.id}`
}

function statusPresentation(state: 'operational' | 'degraded' | 'down') {
  if (state === 'operational') {
    return {
      chip: 'border-emerald-200 bg-emerald-50 text-emerald-700 dark:border-emerald-500/20 dark:bg-emerald-500/10 dark:text-emerald-300',
      dot: 'bg-emerald-500',
    }
  }
  if (state === 'degraded') {
    return {
      chip: 'border-amber-200 bg-amber-50 text-amber-700 dark:border-amber-500/20 dark:bg-amber-500/10 dark:text-amber-300',
      dot: 'bg-amber-500',
    }
  }
  return {
    chip: 'border-red-200 bg-red-50 text-red-700 dark:border-red-500/20 dark:bg-red-500/10 dark:text-red-300',
    dot: 'bg-red-500',
  }
}

function modelDotClass(status: MonitorStatus | '') {
  if (status === STATUS_OPERATIONAL) return 'bg-emerald-500'
  if (status === 'degraded') return 'bg-amber-500'
  if (status === 'failed' || status === 'error') return 'bg-red-500'
  return 'bg-slate-300'
}

function fallbackModels(item: UserMonitorView): UserMonitorModelDetail[] {
  return [
    {
      model: item.primary_model,
      latest_status: item.primary_status,
      latest_latency_ms: item.primary_latency_ms,
      availability_7d: item.availability_7d,
      availability_15d: 0,
      availability_30d: 0,
      avg_latency_7d_ms: item.primary_latency_ms,
      timeline: item.timeline,
    },
    ...item.extra_models.map(model => ({
      model: model.model,
      latest_status: model.status,
      latest_latency_ms: model.latency_ms,
      availability_7d: 0,
      availability_15d: 0,
      availability_30d: 0,
      avg_latency_7d_ms: model.latency_ms,
      timeline: [],
    })),
  ]
}

watch(featureEnabled, (enabled) => {
  if (!enabled) {
    autoRefresh.stop()
    items.value = []
  } else if (autoRefresh.enabled.value) {
    autoRefresh.start()
    void reload(false).then(() => loadAllDetails(false))
  }
})

onMounted(async () => {
  await ensurePublicSettings()
  if (featureEnabled.value) {
    await reload(false)
    await loadAllDetails(false)
    autoRefresh.setEnabled(true)
  }
})

onBeforeUnmount(() => {
  if (abortController) abortController.abort()
  autoRefresh.stop()
})
</script>
