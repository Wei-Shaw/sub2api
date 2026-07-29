<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import Icon from '@/components/icons/Icon.vue'
import Pagination from '@/components/common/Pagination.vue'
import Toggle from '@/components/common/Toggle.vue'
import { formatDateTime, formatRelativeTime } from '@/utils/format'
import * as api from './api'
import type {
  ConnectionRiskEvent,
  ConnectionRiskEventFilters,
  ConnectionRiskRuntime,
  ConnectionRiskSettings,
} from './types'

const { t } = useI18n()

const activeTab = ref<'events' | 'config' | 'runtime'>('events')
const loading = reactive({ events: false, config: false, runtime: false, action: false, refresh: false })
const error = ref('')
const message = ref('')
const showRawRuntime = ref(false)
const showRawDetail = ref(false)

const filters = reactive<ConnectionRiskEventFilters>({
  status: 'open',
  severity: '',
  user_id: '',
  api_key_id: '',
  rule: '',
})
const page = ref(1)
const pageSize = ref(20)
const total = ref(0)
const events = ref<ConnectionRiskEvent[]>([])
const selected = ref<ConnectionRiskEvent | null>(null)

const settings = ref<ConnectionRiskSettings | null>(null)
const runtime = ref<ConnectionRiskRuntime | null>(null)

const RULE_IDS = ['R1', 'R2', 'R3_abs', 'R4', 'R5', 'R6', 'R7'] as const

const severityClass = (s: string) => {
  switch (s) {
    case 'critical':
      return 'bg-red-100 text-red-800 ring-1 ring-inset ring-red-600/20 dark:bg-red-950/40 dark:text-red-200 dark:ring-red-400/20'
    case 'high':
      return 'bg-orange-100 text-orange-800 ring-1 ring-inset ring-orange-600/20 dark:bg-orange-950/40 dark:text-orange-200 dark:ring-orange-400/20'
    case 'medium':
      return 'bg-amber-100 text-amber-800 ring-1 ring-inset ring-amber-600/15 dark:bg-amber-950/40 dark:text-amber-200 dark:ring-amber-400/20'
    case 'low':
      return 'bg-sky-100 text-sky-800 ring-1 ring-inset ring-sky-600/15 dark:bg-sky-950/40 dark:text-sky-200 dark:ring-sky-400/20'
    default:
      return 'bg-gray-100 text-gray-700 ring-1 ring-inset ring-gray-500/10 dark:bg-dark-700 dark:text-dark-200'
  }
}

const statusClass = (s: string) => {
  switch (s) {
    case 'open':
      return 'bg-rose-50 text-rose-700 ring-1 ring-inset ring-rose-600/15 dark:bg-rose-950/30 dark:text-rose-200'
    case 'acknowledged':
      return 'bg-violet-50 text-violet-700 ring-1 ring-inset ring-violet-600/15 dark:bg-violet-950/30 dark:text-violet-200'
    case 'resolved':
      return 'bg-emerald-50 text-emerald-700 ring-1 ring-inset ring-emerald-600/15 dark:bg-emerald-950/30 dark:text-emerald-200'
    case 'suppressed':
      return 'bg-gray-100 text-gray-600 ring-1 ring-inset ring-gray-500/10 dark:bg-dark-700 dark:text-dark-300'
    default:
      return 'bg-gray-100 text-gray-600 dark:bg-dark-700 dark:text-dark-300'
  }
}

function severityLabel(s: string) {
  const key = `admin.connectionRisk.severity.${s}`
  const translated = t(key)
  return translated === key ? s || '—' : translated
}

function statusLabel(s: string) {
  const key = `admin.connectionRisk.status.${s}`
  const translated = t(key)
  return translated === key ? s || '—' : translated
}

function ruleLabel(id: string) {
  const key = `admin.connectionRisk.rules.${id}`
  const translated = t(key)
  return translated === key ? id : translated
}

function flagBadge(on: boolean | undefined, blocked?: boolean) {
  if (blocked) {
    return {
      text: t('admin.connectionRisk.overview.blocked'),
      class: 'bg-amber-50 text-amber-800 dark:bg-amber-950/30 dark:text-amber-200',
    }
  }
  if (on) {
    return {
      text: t('admin.connectionRisk.overview.on'),
      class: 'bg-emerald-50 text-emerald-700 dark:bg-emerald-950/30 dark:text-emerald-300',
    }
  }
  return {
    text: t('admin.connectionRisk.overview.off'),
    class: 'bg-gray-100 text-gray-600 dark:bg-dark-700 dark:text-dark-300',
  }
}

const metrics = computed(() => runtime.value?.metrics)
const yamlOn = computed(() => !!runtime.value?.yaml_enabled)
const emitEffective = computed(() => !!runtime.value?.effective_emit)
const workerEffective = computed(() => !!runtime.value?.effective_worker)

const overviewCards = computed(() => {
  const m = metrics.value
  const lastTick = m?.last_tick_unix
  const lastTickText =
    lastTick && lastTick > 0
      ? formatRelativeTime(new Date(lastTick * 1000))
      : t('admin.connectionRisk.overview.never')

  const yamlBadge = flagBadge(yamlOn.value)
  const emitBadge = flagBadge(emitEffective.value, yamlOn.value && !emitEffective.value && !!settings.value?.emit_enabled)
  const workerBadge = flagBadge(
    workerEffective.value,
    yamlOn.value && !workerEffective.value && !!settings.value?.worker_enabled,
  )

  return [
    {
      key: 'yaml',
      label: t('admin.connectionRisk.overview.yaml'),
      value: yamlOn.value ? t('admin.connectionRisk.overview.on') : t('admin.connectionRisk.overview.off'),
      meta: yamlBadge.text,
      metaClass: yamlBadge.class,
      icon: 'shield' as const,
      iconClass: yamlOn.value
        ? 'bg-emerald-50 text-emerald-600 dark:bg-emerald-900/20 dark:text-emerald-300'
        : 'bg-gray-100 text-gray-500 dark:bg-dark-700 dark:text-dark-300',
    },
    {
      key: 'emit',
      label: t('admin.connectionRisk.overview.emit'),
      value: emitEffective.value
        ? t('admin.connectionRisk.overview.effective')
        : t('admin.connectionRisk.overview.off'),
      meta: `${t('admin.connectionRisk.overview.emitOk')} ${formatNumber(m?.emit_ok ?? 0)}`,
      metaClass: emitBadge.class,
      icon: 'bolt' as const,
      iconClass: emitEffective.value
        ? 'bg-sky-50 text-sky-600 dark:bg-sky-900/20 dark:text-sky-300'
        : 'bg-gray-100 text-gray-500 dark:bg-dark-700 dark:text-dark-300',
    },
    {
      key: 'worker',
      label: t('admin.connectionRisk.overview.worker'),
      value: workerEffective.value
        ? t('admin.connectionRisk.overview.effective')
        : t('admin.connectionRisk.overview.off'),
      meta: lastTickText,
      metaClass: workerBadge.class,
      icon: 'clock' as const,
      iconClass: workerEffective.value
        ? 'bg-violet-50 text-violet-600 dark:bg-violet-900/20 dark:text-violet-300'
        : 'bg-gray-100 text-gray-500 dark:bg-dark-700 dark:text-dark-300',
    },
    {
      key: 'activeKeys',
      label: t('admin.connectionRisk.overview.activeKeys'),
      value: formatNumber(runtime.value?.active_keys ?? 0),
      meta: `${t('admin.connectionRisk.overview.activeUsers')} ${formatNumber(runtime.value?.active_users ?? 0)}`,
      metaClass: 'bg-gray-50 text-gray-600 dark:bg-dark-700 dark:text-dark-300',
      icon: 'key' as const,
      iconClass: 'bg-primary-50 text-primary-600 dark:bg-primary-900/20 dark:text-primary-300',
    },
    {
      key: 'events',
      label: t('admin.connectionRisk.overview.eventsCreated'),
      value: formatNumber(m?.events_created ?? 0),
      meta: m?.degraded
        ? t('admin.connectionRisk.overview.degraded')
        : t('admin.connectionRisk.overview.healthy'),
      metaClass: m?.degraded
        ? 'bg-amber-50 text-amber-800 dark:bg-amber-950/30 dark:text-amber-200'
        : 'bg-emerald-50 text-emerald-700 dark:bg-emerald-950/30 dark:text-emerald-300',
      icon: 'chart' as const,
      iconClass: 'bg-orange-50 text-orange-600 dark:bg-orange-900/20 dark:text-orange-300',
    },
    {
      key: 'ticks',
      label: t('admin.connectionRisk.overview.workerTicks'),
      value: formatNumber(m?.worker_ticks ?? 0),
      meta: `${t('admin.connectionRisk.overview.emitError')} ${formatNumber(m?.emit_error ?? 0)}`,
      metaClass: 'bg-gray-50 text-gray-600 dark:bg-dark-700 dark:text-dark-300',
      icon: 'server' as const,
      iconClass: 'bg-indigo-50 text-indigo-600 dark:bg-indigo-900/20 dark:text-indigo-300',
    },
  ]
})

const sampleIps = computed(() => {
  const raw = selected.value?.evidence?.sample_ips
  return Array.isArray(raw) ? (raw as string[]) : []
})

const sampleUas = computed(() => {
  const raw = selected.value?.evidence?.sample_uas
  return Array.isArray(raw) ? (raw as string[]) : []
})

function formatNumber(n: number) {
  return Number(n || 0).toLocaleString()
}

function formatScore(score: number | undefined | null) {
  if (score == null || Number.isNaN(Number(score))) return '—'
  return Number(score).toFixed(Number.isInteger(Number(score)) ? 0 : 1)
}

function formatWhen(value?: string | null) {
  if (!value) return '—'
  return formatDateTime(value)
}

function formatWhenRelative(value?: string | null) {
  if (!value) return '—'
  return formatRelativeTime(value)
}

async function loadEvents() {
  loading.events = true
  error.value = ''
  try {
    const res = await api.listEvents(filters, page.value, pageSize.value)
    events.value = res.items ?? []
    total.value = res.total ?? 0
  } catch (e: any) {
    error.value = e?.message || t('admin.connectionRisk.errors.loadEvents')
  } finally {
    loading.events = false
  }
}

async function loadConfig() {
  loading.config = true
  try {
    settings.value = await api.getConfig()
  } catch (e: any) {
    error.value = e?.message || t('admin.connectionRisk.errors.loadConfig')
  } finally {
    loading.config = false
  }
}

async function loadRuntime() {
  loading.runtime = true
  try {
    runtime.value = await api.getRuntime()
  } catch (e: any) {
    error.value = e?.message || t('admin.connectionRisk.errors.loadRuntime')
  } finally {
    loading.runtime = false
  }
}

async function refreshAll() {
  loading.refresh = true
  error.value = ''
  try {
    await Promise.all([loadRuntime(), loadConfig(), loadEvents()])
  } finally {
    loading.refresh = false
  }
}

async function saveConfig() {
  if (!settings.value) return
  loading.action = true
  message.value = ''
  error.value = ''
  try {
    settings.value = await api.updateConfig(settings.value)
    message.value = t('admin.connectionRisk.messages.saved')
    await loadRuntime()
  } catch (e: any) {
    error.value = e?.message || t('admin.connectionRisk.errors.saveConfig')
  } finally {
    loading.action = false
  }
}

async function doAction(kind: 'ack' | 'resolve' | 'suppress' | 'delete', id: number) {
  loading.action = true
  error.value = ''
  try {
    if (kind === 'ack') await api.ackEvent(id)
    else if (kind === 'resolve') await api.resolveEvent(id)
    else if (kind === 'suppress') await api.suppressEvent(id)
    else await api.deleteEvent(id)
    message.value = t('admin.connectionRisk.messages.actionOk')
    selected.value = null
    await Promise.all([loadEvents(), loadRuntime()])
  } catch (e: any) {
    error.value = e?.message || t('admin.connectionRisk.errors.action')
  } finally {
    loading.action = false
  }
}

async function whitelistFromEvidence() {
  if (!selected.value?.api_key_id) return
  if (!sampleIps.value.length) {
    error.value = t('admin.connectionRisk.errors.noSampleIPs')
    return
  }
  loading.action = true
  error.value = ''
  try {
    await api.whitelistIPs(selected.value.api_key_id, sampleIps.value.slice(0, 10))
    if (selected.value.api_key_id) {
      await api.exemptSubject('k', selected.value.api_key_id, 'whitelist-from-ui')
    }
    message.value = t('admin.connectionRisk.messages.whitelisted')
    await loadEvents()
  } catch (e: any) {
    error.value = e?.message || t('admin.connectionRisk.errors.action')
  } finally {
    loading.action = false
  }
}

async function runRetention() {
  loading.action = true
  error.value = ''
  try {
    const res = await api.runRetention()
    message.value = t('admin.connectionRisk.messages.retention', { count: res.deleted ?? 0 })
    await loadEvents()
  } catch (e: any) {
    error.value = e?.message || t('admin.connectionRisk.errors.action')
  } finally {
    loading.action = false
  }
}

function openEvent(ev: ConnectionRiskEvent) {
  selected.value = ev
  showRawDetail.value = false
}

function resetFilters() {
  filters.status = 'open'
  filters.severity = ''
  filters.user_id = ''
  filters.api_key_id = ''
  filters.rule = ''
  page.value = 1
  loadEvents()
}

function searchEvents() {
  page.value = 1
  loadEvents()
}

function onPageUpdate(p: number) {
  page.value = p
  loadEvents()
}

function onPageSizeUpdate(size: number) {
  pageSize.value = size
  page.value = 1
  loadEvents()
}

function switchTab(tab: 'events' | 'config' | 'runtime') {
  activeTab.value = tab
  if (tab === 'config') loadConfig()
  if (tab === 'runtime') loadRuntime()
}

onMounted(async () => {
  await Promise.all([loadEvents(), loadConfig(), loadRuntime()])
})
</script>

<template>
  <AppLayout>
    <div class="mx-auto max-w-[1400px] space-y-6 pb-10">
      <!-- Header -->
      <div class="flex flex-col gap-4 lg:flex-row lg:items-start lg:justify-between">
        <div>
          <p class="text-xs font-semibold uppercase tracking-[0.16em] text-primary-600 dark:text-primary-400">
            {{ t('admin.connectionRisk.subtitle') }}
          </p>
          <h1 class="mt-1 text-2xl font-semibold text-gray-900 dark:text-white">
            {{ t('admin.connectionRisk.title') }}
          </h1>
          <p class="mt-2 max-w-3xl text-sm text-gray-500 dark:text-dark-300">
            {{ t('admin.connectionRisk.description') }}
          </p>
        </div>
        <div class="flex flex-wrap items-center gap-2">
          <button
            type="button"
            class="btn btn-secondary inline-flex items-center gap-2"
            :disabled="loading.refresh"
            @click="refreshAll"
          >
            <Icon name="refresh" size="sm" :class="loading.refresh ? 'animate-spin' : ''" />
            {{ t('admin.connectionRisk.refreshAll') }}
          </button>
          <button
            type="button"
            class="btn btn-secondary inline-flex items-center gap-2"
            :disabled="loading.action"
            @click="runRetention"
          >
            <Icon name="trash" size="sm" />
            {{ t('admin.connectionRisk.actions.runRetention') }}
          </button>
        </div>
      </div>

      <!-- Alerts -->
      <div
        v-if="error"
        role="alert"
        class="rounded-xl border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-700 dark:border-red-900 dark:bg-red-950/30 dark:text-red-300"
      >
        {{ error }}
        <button type="button" class="ml-3 underline" @click="error = ''">×</button>
      </div>
      <div
        v-if="message"
        role="status"
        class="rounded-xl border border-emerald-200 bg-emerald-50 px-4 py-3 text-sm text-emerald-800 dark:border-emerald-900 dark:bg-emerald-950/30 dark:text-emerald-200"
      >
        {{ message }}
        <button type="button" class="ml-3 underline" @click="message = ''">×</button>
      </div>

      <!-- Overview cards -->
      <section>
        <div class="mb-3 flex items-center justify-between">
          <h2 class="text-sm font-semibold text-gray-900 dark:text-white">
            {{ t('admin.connectionRisk.overview.title') }}
          </h2>
          <span
            v-if="settings?.phase"
            class="inline-flex items-center rounded-full bg-gray-100 px-2.5 py-1 text-xs font-medium text-gray-600 dark:bg-dark-700 dark:text-dark-300"
          >
            {{ t('admin.connectionRisk.overview.phase') }} ·
            {{
              settings.phase === 'enforce'
                ? t('admin.connectionRisk.overview.phaseEnforce')
                : t('admin.connectionRisk.overview.phaseObserve')
            }}
          </span>
        </div>
        <div class="grid grid-cols-1 gap-3 sm:grid-cols-2 xl:grid-cols-3 2xl:grid-cols-6">
          <div
            v-for="card in overviewCards"
            :key="card.key"
            class="rounded-xl border border-gray-100 bg-white px-4 py-3 shadow-sm dark:border-dark-700 dark:bg-dark-800"
          >
            <div class="flex min-w-0 items-start gap-3">
              <div
                class="flex h-9 w-9 flex-shrink-0 items-center justify-center rounded-lg"
                :class="card.iconClass"
              >
                <Icon :name="card.icon" size="sm" />
              </div>
              <div class="min-w-0 flex-1">
                <p class="truncate text-xs font-medium text-gray-500 dark:text-dark-400">{{ card.label }}</p>
                <p class="mt-1 truncate text-lg font-semibold leading-7 text-gray-900 dark:text-white">
                  {{ card.value }}
                </p>
                <span
                  class="mt-1 inline-flex max-w-full truncate rounded-full px-2 py-0.5 text-[11px] font-medium"
                  :class="card.metaClass"
                >
                  {{ card.meta }}
                </span>
              </div>
            </div>
          </div>
        </div>
      </section>

      <!-- Tabs -->
      <div class="tabs inline-flex" role="tablist">
        <button
          type="button"
          class="tab"
          :class="{ 'tab-active': activeTab === 'events' }"
          @click="switchTab('events')"
        >
          {{ t('admin.connectionRisk.tabs.events') }}
          <span
            v-if="total > 0 && filters.status === 'open'"
            class="ml-1.5 rounded-full bg-rose-100 px-1.5 py-0.5 text-[10px] font-semibold text-rose-700 dark:bg-rose-950/40 dark:text-rose-200"
          >
            {{ total }}
          </span>
        </button>
        <button
          type="button"
          class="tab"
          :class="{ 'tab-active': activeTab === 'config' }"
          @click="switchTab('config')"
        >
          {{ t('admin.connectionRisk.tabs.config') }}
        </button>
        <button
          type="button"
          class="tab"
          :class="{ 'tab-active': activeTab === 'runtime' }"
          @click="switchTab('runtime')"
        >
          {{ t('admin.connectionRisk.tabs.runtime') }}
        </button>
      </div>

      <!-- Events -->
      <section v-show="activeTab === 'events'" class="space-y-4">
        <div class="card p-4 sm:p-5">
          <div class="flex flex-wrap items-end gap-3">
            <div class="min-w-[140px]">
              <label class="input-label">{{ t('admin.connectionRisk.filters.status') }}</label>
              <select v-model="filters.status" class="input input-sm w-full" @change="searchEvents">
                <option value="">{{ t('admin.connectionRisk.filters.allStatus') }}</option>
                <option value="open">{{ t('admin.connectionRisk.status.open') }}</option>
                <option value="acknowledged">{{ t('admin.connectionRisk.status.acknowledged') }}</option>
                <option value="resolved">{{ t('admin.connectionRisk.status.resolved') }}</option>
                <option value="suppressed">{{ t('admin.connectionRisk.status.suppressed') }}</option>
              </select>
            </div>
            <div class="min-w-[140px]">
              <label class="input-label">{{ t('admin.connectionRisk.filters.severity') }}</label>
              <select v-model="filters.severity" class="input input-sm w-full" @change="searchEvents">
                <option value="">{{ t('admin.connectionRisk.filters.allSeverity') }}</option>
                <option value="critical">{{ t('admin.connectionRisk.severity.critical') }}</option>
                <option value="high">{{ t('admin.connectionRisk.severity.high') }}</option>
                <option value="medium">{{ t('admin.connectionRisk.severity.medium') }}</option>
                <option value="low">{{ t('admin.connectionRisk.severity.low') }}</option>
              </select>
            </div>
            <div class="min-w-[120px]">
              <label class="input-label">{{ t('admin.connectionRisk.filters.rule') }}</label>
              <select v-model="filters.rule" class="input input-sm w-full" @change="searchEvents">
                <option value="">{{ t('admin.connectionRisk.filters.allRules') }}</option>
                <option v-for="id in RULE_IDS" :key="id" :value="id">{{ ruleLabel(id) }}</option>
              </select>
            </div>
            <div class="min-w-[110px]">
              <label class="input-label">{{ t('admin.connectionRisk.filters.userId') }}</label>
              <input
                v-model="filters.user_id"
                class="input input-sm w-full"
                inputmode="numeric"
                @keyup.enter="searchEvents"
              />
            </div>
            <div class="min-w-[110px]">
              <label class="input-label">{{ t('admin.connectionRisk.filters.keyId') }}</label>
              <input
                v-model="filters.api_key_id"
                class="input input-sm w-full"
                inputmode="numeric"
                @keyup.enter="searchEvents"
              />
            </div>
            <div class="flex flex-wrap gap-2 pb-0.5">
              <button type="button" class="btn btn-primary btn-sm" :disabled="loading.events" @click="searchEvents">
                {{ t('admin.connectionRisk.filters.search') }}
              </button>
              <button type="button" class="btn btn-secondary btn-sm" :disabled="loading.events" @click="resetFilters">
                {{ t('admin.connectionRisk.filters.reset') }}
              </button>
            </div>
          </div>
        </div>

        <div class="card overflow-hidden">
          <div v-if="loading.events" class="flex items-center justify-center py-16">
            <div class="h-8 w-8 animate-spin rounded-full border-b-2 border-primary-600" />
          </div>
          <div v-else-if="!events.length" class="px-6 py-14 text-center">
            <div
              class="mx-auto mb-4 flex h-14 w-14 items-center justify-center rounded-2xl bg-gray-100 dark:bg-dark-700"
            >
              <Icon name="shield" size="lg" class="text-gray-400" />
            </div>
            <h3 class="text-base font-semibold text-gray-900 dark:text-white">
              {{ t('admin.connectionRisk.empty') }}
            </h3>
            <p class="mt-2 text-sm text-gray-500 dark:text-dark-400">
              {{ t('admin.connectionRisk.emptyHint') }}
            </p>
          </div>
          <div v-else class="overflow-x-auto">
            <table class="min-w-full text-left text-sm">
              <thead class="border-b border-gray-100 bg-gray-50/80 text-xs uppercase tracking-wide text-gray-500 dark:border-dark-700 dark:bg-dark-900/40 dark:text-dark-400">
                <tr>
                  <th class="px-4 py-3 font-medium">{{ t('admin.connectionRisk.columns.id') }}</th>
                  <th class="px-4 py-3 font-medium">{{ t('admin.connectionRisk.columns.severity') }}</th>
                  <th class="px-4 py-3 font-medium">{{ t('admin.connectionRisk.columns.score') }}</th>
                  <th class="px-4 py-3 font-medium">{{ t('admin.connectionRisk.columns.subject') }}</th>
                  <th class="px-4 py-3 font-medium">{{ t('admin.connectionRisk.columns.rules') }}</th>
                  <th class="px-4 py-3 font-medium">{{ t('admin.connectionRisk.columns.status') }}</th>
                  <th class="px-4 py-3 font-medium">{{ t('admin.connectionRisk.columns.lastSeen') }}</th>
                </tr>
              </thead>
              <tbody class="divide-y divide-gray-100 dark:divide-dark-800">
                <tr
                  v-for="ev in events"
                  :key="ev.id"
                  class="cursor-pointer transition-colors hover:bg-gray-50/80 dark:hover:bg-dark-800/50"
                  @click="openEvent(ev)"
                >
                  <td class="px-4 py-3 font-mono text-xs text-gray-500">#{{ ev.id }}</td>
                  <td class="px-4 py-3">
                    <span
                      class="inline-flex items-center rounded-full px-2.5 py-0.5 text-xs font-semibold"
                      :class="severityClass(ev.severity)"
                    >
                      {{ severityLabel(ev.severity) }}
                    </span>
                  </td>
                  <td class="px-4 py-3 font-semibold tabular-nums text-gray-900 dark:text-white">
                    {{ formatScore(ev.score) }}
                  </td>
                  <td class="px-4 py-3">
                    <div class="min-w-0">
                      <div class="flex items-center gap-1.5 text-sm font-medium text-gray-900 dark:text-white">
                        <Icon name="key" size="xs" class="text-gray-400" />
                        <span class="font-mono">#{{ ev.api_key_id ?? '—' }}</span>
                        <span v-if="ev.api_key_prefix" class="truncate text-xs font-normal text-gray-400">
                          {{ ev.api_key_prefix }}
                        </span>
                      </div>
                      <div class="mt-0.5 flex items-center gap-1.5 text-xs text-gray-500">
                        <Icon name="users" size="xs" class="text-gray-400" />
                        #{{ ev.user_id ?? '—' }}
                      </div>
                    </div>
                  </td>
                  <td class="px-4 py-3">
                    <div class="flex max-w-[220px] flex-wrap gap-1">
                      <span
                        v-for="r in ev.rules_fired || []"
                        :key="`${ev.id}-${r.rule_id}`"
                        class="inline-flex rounded-md bg-gray-100 px-1.5 py-0.5 font-mono text-[11px] text-gray-700 dark:bg-dark-700 dark:text-dark-200"
                        :title="r.detail || ruleLabel(r.rule_id)"
                      >
                        {{ r.rule_id }}
                      </span>
                      <span v-if="!(ev.rules_fired || []).length" class="text-xs text-gray-400">—</span>
                    </div>
                  </td>
                  <td class="px-4 py-3">
                    <span
                      class="inline-flex items-center rounded-full px-2.5 py-0.5 text-xs font-medium"
                      :class="statusClass(ev.status)"
                    >
                      {{ statusLabel(ev.status) }}
                    </span>
                  </td>
                  <td class="px-4 py-3">
                    <div class="text-sm text-gray-700 dark:text-dark-200" :title="formatWhen(ev.last_seen_at)">
                      {{ formatWhenRelative(ev.last_seen_at) }}
                    </div>
                    <div class="mt-0.5 text-[11px] text-gray-400">{{ formatWhen(ev.last_seen_at) }}</div>
                  </td>
                </tr>
              </tbody>
            </table>
          </div>
          <Pagination
            v-if="total > 0"
            :total="total"
            :page="page"
            :page-size="pageSize"
            @update:page="onPageUpdate"
            @update:page-size="onPageSizeUpdate"
          />
        </div>
      </section>

      <!-- Detail drawer -->
      <div
        v-if="selected"
        class="fixed inset-0 z-40 flex justify-end bg-black/40 backdrop-blur-[1px]"
        @click.self="selected = null"
      >
        <aside
          class="flex h-full w-full max-w-xl flex-col border-l border-gray-200 bg-white shadow-2xl dark:border-dark-700 dark:bg-dark-900"
        >
          <div class="flex items-start justify-between gap-3 border-b border-gray-100 px-5 py-4 dark:border-dark-700">
            <div class="min-w-0">
              <p class="text-xs font-medium uppercase tracking-wide text-gray-400">
                {{ t('admin.connectionRisk.detail.title') }} · #{{ selected.id }}
              </p>
              <h2 class="mt-1 truncate text-lg font-semibold text-gray-900 dark:text-white">
                {{ selected.title || `#${selected.id}` }}
              </h2>
              <div class="mt-2 flex flex-wrap items-center gap-2">
                <span
                  class="inline-flex rounded-full px-2.5 py-0.5 text-xs font-semibold"
                  :class="severityClass(selected.severity)"
                >
                  {{ severityLabel(selected.severity) }}
                </span>
                <span
                  class="inline-flex rounded-full px-2.5 py-0.5 text-xs font-medium"
                  :class="statusClass(selected.status)"
                >
                  {{ statusLabel(selected.status) }}
                </span>
                <span class="text-sm font-semibold tabular-nums text-gray-700 dark:text-dark-200">
                  {{ formatScore(selected.score) }}
                </span>
              </div>
            </div>
            <button type="button" class="btn btn-ghost btn-sm" @click="selected = null">
              <Icon name="x" size="sm" />
            </button>
          </div>

          <div class="flex-1 space-y-5 overflow-y-auto px-5 py-5">
            <section v-if="selected.summary">
              <h3 class="text-xs font-semibold uppercase tracking-wide text-gray-400">
                {{ t('admin.connectionRisk.detail.summary') }}
              </h3>
              <p class="mt-2 text-sm leading-6 text-gray-700 dark:text-dark-200">{{ selected.summary }}</p>
            </section>

            <section class="grid grid-cols-2 gap-3">
              <div class="rounded-xl bg-gray-50 p-3 dark:bg-dark-800/80">
                <p class="text-xs text-gray-500">{{ t('admin.connectionRisk.detail.apiKey') }}</p>
                <p class="mt-1 font-mono text-sm font-semibold text-gray-900 dark:text-white">
                  #{{ selected.api_key_id ?? '—' }}
                </p>
                <p v-if="selected.api_key_prefix" class="mt-0.5 truncate text-xs text-gray-400">
                  {{ selected.api_key_prefix }}
                </p>
              </div>
              <div class="rounded-xl bg-gray-50 p-3 dark:bg-dark-800/80">
                <p class="text-xs text-gray-500">{{ t('admin.connectionRisk.detail.user') }}</p>
                <p class="mt-1 font-mono text-sm font-semibold text-gray-900 dark:text-white">
                  #{{ selected.user_id ?? '—' }}
                </p>
              </div>
              <div class="rounded-xl bg-gray-50 p-3 dark:bg-dark-800/80">
                <p class="text-xs text-gray-500">{{ t('admin.connectionRisk.columns.firstSeen') }}</p>
                <p class="mt-1 text-sm text-gray-800 dark:text-dark-100">{{ formatWhen(selected.first_seen_at) }}</p>
              </div>
              <div class="rounded-xl bg-gray-50 p-3 dark:bg-dark-800/80">
                <p class="text-xs text-gray-500">{{ t('admin.connectionRisk.columns.lastSeen') }}</p>
                <p class="mt-1 text-sm text-gray-800 dark:text-dark-100">{{ formatWhen(selected.last_seen_at) }}</p>
              </div>
            </section>

            <section>
              <h3 class="text-xs font-semibold uppercase tracking-wide text-gray-400">
                {{ t('admin.connectionRisk.detail.rules') }}
              </h3>
              <div v-if="(selected.rules_fired || []).length" class="mt-2 space-y-2">
                <div
                  v-for="(r, idx) in selected.rules_fired"
                  :key="`${r.rule_id}-${idx}`"
                  class="rounded-xl border border-gray-100 p-3 dark:border-dark-700"
                >
                  <div class="flex items-center justify-between gap-2">
                    <div class="flex items-center gap-2">
                      <span class="rounded-md bg-gray-100 px-1.5 py-0.5 font-mono text-xs font-semibold dark:bg-dark-700">
                        {{ r.rule_id }}
                      </span>
                      <span class="text-sm font-medium text-gray-800 dark:text-dark-100">
                        {{ ruleLabel(r.rule_id) }}
                      </span>
                    </div>
                    <span
                      class="rounded-full px-2 py-0.5 text-[11px] font-medium"
                      :class="severityClass(r.severity)"
                    >
                      {{ severityLabel(r.severity) }}
                    </span>
                  </div>
                  <p v-if="r.detail" class="mt-2 text-sm text-gray-600 dark:text-dark-300">{{ r.detail }}</p>
                  <div class="mt-2 flex gap-3 text-[11px] text-gray-400">
                    <span>{{ t('admin.connectionRisk.detail.confidence') }}: {{ r.confidence }}</span>
                    <span>{{ t('admin.connectionRisk.detail.weight') }}: {{ r.weight }}</span>
                  </div>
                </div>
              </div>
              <p v-else class="mt-2 text-sm text-gray-400">—</p>
            </section>

            <section>
              <h3 class="text-xs font-semibold uppercase tracking-wide text-gray-400">
                {{ t('admin.connectionRisk.detail.evidence') }}
              </h3>
              <div class="mt-2 space-y-3">
                <div v-if="sampleIps.length">
                  <p class="mb-1.5 text-xs text-gray-500">{{ t('admin.connectionRisk.detail.sampleIps') }}</p>
                  <div class="flex flex-wrap gap-1.5">
                    <span
                      v-for="ip in sampleIps"
                      :key="ip"
                      class="rounded-md bg-sky-50 px-2 py-1 font-mono text-xs text-sky-800 dark:bg-sky-950/30 dark:text-sky-200"
                    >
                      {{ ip }}
                    </span>
                  </div>
                </div>
                <div v-if="sampleUas.length">
                  <p class="mb-1.5 text-xs text-gray-500">{{ t('admin.connectionRisk.detail.sampleUas') }}</p>
                  <div class="space-y-1">
                    <p
                      v-for="(ua, i) in sampleUas.slice(0, 5)"
                      :key="i"
                      class="truncate rounded-md bg-gray-50 px-2 py-1 font-mono text-[11px] text-gray-600 dark:bg-dark-800 dark:text-dark-300"
                      :title="ua"
                    >
                      {{ ua }}
                    </p>
                  </div>
                </div>
                <p v-if="!sampleIps.length && !sampleUas.length" class="text-sm text-gray-400">
                  {{ t('admin.connectionRisk.detail.noEvidence') }}
                </p>
              </div>
            </section>

            <section>
              <button
                type="button"
                class="text-xs font-medium text-primary-600 hover:underline dark:text-primary-400"
                @click="showRawDetail = !showRawDetail"
              >
                {{ showRawDetail ? t('admin.connectionRisk.runtime.hideRaw') : t('admin.connectionRisk.runtime.showRaw') }}
              </button>
              <pre
                v-if="showRawDetail"
                class="mt-2 max-h-64 overflow-auto rounded-xl bg-gray-50 p-3 text-[11px] leading-5 dark:bg-dark-800"
              >{{ JSON.stringify({ rules_fired: selected.rules_fired, evidence: selected.evidence, metrics: selected.metrics }, null, 2) }}</pre>
            </section>
          </div>

          <div class="border-t border-gray-100 px-5 py-4 dark:border-dark-700">
            <p class="mb-3 text-xs font-semibold uppercase tracking-wide text-gray-400">
              {{ t('admin.connectionRisk.detail.actions') }}
            </p>
            <div class="flex flex-wrap gap-2">
              <button
                type="button"
                class="btn btn-secondary btn-sm"
                :disabled="loading.action || selected.status !== 'open'"
                @click="doAction('ack', selected.id)"
              >
                {{ t('admin.connectionRisk.actions.ack') }}
              </button>
              <button
                type="button"
                class="btn btn-secondary btn-sm"
                :disabled="loading.action"
                @click="doAction('resolve', selected.id)"
              >
                {{ t('admin.connectionRisk.actions.resolve') }}
              </button>
              <button
                type="button"
                class="btn btn-secondary btn-sm"
                :disabled="loading.action"
                @click="doAction('suppress', selected.id)"
              >
                {{ t('admin.connectionRisk.actions.suppress') }}
              </button>
              <button
                type="button"
                class="btn btn-primary btn-sm"
                :disabled="loading.action || !selected.api_key_id || !sampleIps.length"
                @click="whitelistFromEvidence"
              >
                {{ t('admin.connectionRisk.actions.whitelist') }}
              </button>
              <button
                type="button"
                class="btn btn-danger btn-sm"
                :disabled="loading.action"
                @click="doAction('delete', selected.id)"
              >
                {{ t('admin.connectionRisk.actions.delete') }}
              </button>
            </div>
          </div>
        </aside>
      </div>

      <!-- Config -->
      <section v-show="activeTab === 'config'" class="space-y-4">
        <div v-if="loading.config" class="flex items-center justify-center py-16">
          <div class="h-8 w-8 animate-spin rounded-full border-b-2 border-primary-600" />
        </div>
        <template v-else-if="settings">
          <div
            class="rounded-xl border px-4 py-3 text-sm"
            :class="
              yamlOn
                ? 'border-emerald-200 bg-emerald-50 text-emerald-800 dark:border-emerald-900 dark:bg-emerald-950/20 dark:text-emerald-200'
                : 'border-amber-200 bg-amber-50 text-amber-800 dark:border-amber-900 dark:bg-amber-950/20 dark:text-amber-200'
            "
          >
            {{ yamlOn ? t('admin.connectionRisk.config.yamlOn') : t('admin.connectionRisk.config.yamlOff') }}
            <span class="mt-1 block text-xs opacity-80">{{ t('admin.connectionRisk.config.yamlHint') }}</span>
          </div>

          <div class="card">
            <div class="border-b border-gray-100 px-5 py-4 dark:border-dark-700">
              <h3 class="text-base font-semibold text-gray-900 dark:text-white">
                {{ t('admin.connectionRisk.config.sectionMaster') }}
              </h3>
              <p class="mt-1 text-sm text-gray-500 dark:text-dark-400">
                {{ t('admin.connectionRisk.config.sectionMasterHint') }}
              </p>
            </div>
            <div class="divide-y divide-gray-100 dark:divide-dark-800">
              <label
                v-for="item in [
                  { key: 'enabled', model: 'enabled', label: t('admin.connectionRisk.config.enabled') },
                  { key: 'emit', model: 'emit_enabled', label: t('admin.connectionRisk.config.emitEnabled') },
                  { key: 'worker', model: 'worker_enabled', label: t('admin.connectionRisk.config.workerEnabled') },
                ]"
                :key="item.key"
                class="flex cursor-pointer items-center justify-between gap-4 px-5 py-4"
              >
                <span class="text-sm text-gray-800 dark:text-dark-100">{{ item.label }}</span>
                <Toggle
                  :model-value="!!(settings as any)[item.model]"
                  @update:model-value="(v: boolean) => ((settings as any)[item.model] = v)"
                />
              </label>
            </div>
          </div>

          <div class="card">
            <div class="border-b border-gray-100 px-5 py-4 dark:border-dark-700">
              <h3 class="text-base font-semibold text-gray-900 dark:text-white">
                {{ t('admin.connectionRisk.config.sectionCollect') }}
              </h3>
              <p class="mt-1 text-sm text-gray-500 dark:text-dark-400">
                {{ t('admin.connectionRisk.config.sectionCollectHint') }}
              </p>
            </div>
            <div class="space-y-4 p-5">
              <label class="flex cursor-pointer items-center justify-between gap-4">
                <span class="text-sm text-gray-800 dark:text-dark-100">
                  {{ t('admin.connectionRisk.config.includeReadOnly') }}
                </span>
                <Toggle v-model="settings.include_read_only_endpoints" />
              </label>
              <label class="flex cursor-pointer items-center justify-between gap-4">
                <span class="text-sm text-gray-800 dark:text-dark-100">
                  {{ t('admin.connectionRisk.config.r7Admin') }}
                </span>
                <Toggle v-model="settings.r7_include_admin_actors" />
              </label>
              <div class="grid gap-4 sm:grid-cols-2">
                <label class="block text-sm">
                  <span class="text-gray-600 dark:text-dark-300">{{ t('admin.connectionRisk.config.sampleRate') }}</span>
                  <input
                    v-model.number="settings.emit_sample_rate_evidence"
                    type="number"
                    min="0"
                    max="1"
                    step="0.05"
                    class="input input-sm mt-1.5 w-full"
                  />
                </label>
                <label class="block text-sm">
                  <span class="text-gray-600 dark:text-dark-300">
                    {{ t('admin.connectionRisk.config.workerInterval') }}
                  </span>
                  <input
                    v-model.number="settings.worker_interval_seconds"
                    type="number"
                    min="60"
                    max="300"
                    class="input input-sm mt-1.5 w-full"
                  />
                </label>
                <label class="block text-sm">
                  <span class="text-gray-600 dark:text-dark-300">
                    {{ t('admin.connectionRisk.config.retentionDays') }}
                  </span>
                  <input
                    v-model.number="settings.retention_days"
                    type="number"
                    min="1"
                    class="input input-sm mt-1.5 w-full"
                  />
                </label>
                <label class="block text-sm">
                  <span class="text-gray-600 dark:text-dark-300">{{ t('admin.connectionRisk.config.phase') }}</span>
                  <select v-model="settings.phase" class="input input-sm mt-1.5 w-full">
                    <option value="observe">{{ t('admin.connectionRisk.config.phaseObserve') }}</option>
                    <option value="enforce">{{ t('admin.connectionRisk.config.phaseEnforce') }}</option>
                  </select>
                </label>
              </div>
            </div>
          </div>

          <div class="card">
            <div class="border-b border-gray-100 px-5 py-4 dark:border-dark-700">
              <h3 class="text-base font-semibold text-gray-900 dark:text-white">
                {{ t('admin.connectionRisk.config.sectionAction') }}
              </h3>
              <p class="mt-1 text-sm text-gray-500 dark:text-dark-400">
                {{ t('admin.connectionRisk.config.sectionActionHint') }}
              </p>
            </div>
            <div v-if="settings.actions" class="space-y-4 p-5">
              <label class="flex cursor-pointer items-center justify-between gap-4">
                <span class="text-sm text-gray-800 dark:text-dark-100">
                  {{ t('admin.connectionRisk.config.softThrottle') }}
                </span>
                <Toggle v-model="settings.actions.soft_throttle_enabled" />
              </label>
              <label class="flex cursor-pointer items-center justify-between gap-4">
                <span class="text-sm font-medium text-rose-700 dark:text-rose-300">
                  {{ t('admin.connectionRisk.config.autoDisable') }}
                </span>
                <Toggle v-model="settings.actions.auto_disable_enabled" />
              </label>
              <label class="block max-w-xs text-sm">
                <span class="text-gray-600 dark:text-dark-300">{{ t('admin.connectionRisk.config.throttleRpm') }}</span>
                <input
                  v-model.number="settings.actions.throttle_abs_rpm"
                  type="number"
                  min="0"
                  class="input input-sm mt-1.5 w-full"
                />
              </label>
            </div>
          </div>

          <div class="flex justify-end">
            <button type="button" class="btn btn-primary" :disabled="loading.action" @click="saveConfig">
              {{ t('admin.connectionRisk.config.save') }}
            </button>
          </div>
        </template>
      </section>

      <!-- Runtime -->
      <section v-show="activeTab === 'runtime'" class="space-y-4">
        <div class="flex items-center justify-between">
          <h2 class="text-base font-semibold text-gray-900 dark:text-white">
            {{ t('admin.connectionRisk.runtime.title') }}
          </h2>
          <button type="button" class="btn btn-secondary btn-sm inline-flex items-center gap-1.5" @click="loadRuntime">
            <Icon name="refresh" size="sm" :class="loading.runtime ? 'animate-spin' : ''" />
            {{ t('common.refresh') }}
          </button>
        </div>

        <div v-if="loading.runtime && !runtime" class="flex items-center justify-center py-16">
          <div class="h-8 w-8 animate-spin rounded-full border-b-2 border-primary-600" />
        </div>

        <template v-else>
          <div class="grid grid-cols-1 gap-4 lg:grid-cols-3">
            <div class="card p-5">
              <h3 class="text-sm font-semibold text-gray-900 dark:text-white">
                {{ t('admin.connectionRisk.runtime.flags') }}
              </h3>
              <dl class="mt-4 space-y-3 text-sm">
                <div class="flex items-center justify-between gap-3">
                  <dt class="text-gray-500">YAML</dt>
                  <dd>
                    <span class="rounded-full px-2 py-0.5 text-xs font-medium" :class="flagBadge(yamlOn).class">
                      {{ flagBadge(yamlOn).text }}
                    </span>
                  </dd>
                </div>
                <div class="flex items-center justify-between gap-3">
                  <dt class="text-gray-500">{{ t('admin.connectionRisk.overview.emit') }}</dt>
                  <dd>
                    <span class="rounded-full px-2 py-0.5 text-xs font-medium" :class="flagBadge(emitEffective).class">
                      {{ flagBadge(emitEffective).text }}
                    </span>
                  </dd>
                </div>
                <div class="flex items-center justify-between gap-3">
                  <dt class="text-gray-500">{{ t('admin.connectionRisk.overview.worker') }}</dt>
                  <dd>
                    <span class="rounded-full px-2 py-0.5 text-xs font-medium" :class="flagBadge(workerEffective).class">
                      {{ flagBadge(workerEffective).text }}
                    </span>
                  </dd>
                </div>
              </dl>
            </div>

            <div class="card p-5">
              <h3 class="text-sm font-semibold text-gray-900 dark:text-white">
                {{ t('admin.connectionRisk.runtime.active') }}
              </h3>
              <div class="mt-4 grid grid-cols-2 gap-3">
                <div class="rounded-xl bg-gray-50 p-3 dark:bg-dark-800/80">
                  <p class="text-xs text-gray-500">{{ t('admin.connectionRisk.overview.activeKeys') }}</p>
                  <p class="mt-1 text-2xl font-semibold tabular-nums text-gray-900 dark:text-white">
                    {{ formatNumber(runtime?.active_keys ?? 0) }}
                  </p>
                </div>
                <div class="rounded-xl bg-gray-50 p-3 dark:bg-dark-800/80">
                  <p class="text-xs text-gray-500">{{ t('admin.connectionRisk.overview.activeUsers') }}</p>
                  <p class="mt-1 text-2xl font-semibold tabular-nums text-gray-900 dark:text-white">
                    {{ formatNumber(runtime?.active_users ?? 0) }}
                  </p>
                </div>
              </div>
            </div>

            <div class="card p-5">
              <h3 class="text-sm font-semibold text-gray-900 dark:text-white">
                {{ t('admin.connectionRisk.runtime.metrics') }}
              </h3>
              <dl class="mt-4 grid grid-cols-2 gap-3 text-sm">
                <div class="rounded-xl bg-emerald-50/80 p-3 dark:bg-emerald-950/20">
                  <dt class="text-xs text-emerald-700/80 dark:text-emerald-300/80">
                    {{ t('admin.connectionRisk.overview.emitOk') }}
                  </dt>
                  <dd class="mt-1 text-xl font-semibold tabular-nums text-emerald-800 dark:text-emerald-200">
                    {{ formatNumber(metrics?.emit_ok ?? 0) }}
                  </dd>
                </div>
                <div class="rounded-xl bg-rose-50/80 p-3 dark:bg-rose-950/20">
                  <dt class="text-xs text-rose-700/80 dark:text-rose-300/80">
                    {{ t('admin.connectionRisk.overview.emitError') }}
                  </dt>
                  <dd class="mt-1 text-xl font-semibold tabular-nums text-rose-800 dark:text-rose-200">
                    {{ formatNumber((metrics?.emit_error ?? 0) + (metrics?.emit_timeout ?? 0)) }}
                  </dd>
                </div>
                <div class="rounded-xl bg-gray-50 p-3 dark:bg-dark-800/80">
                  <dt class="text-xs text-gray-500">{{ t('admin.connectionRisk.overview.workerTicks') }}</dt>
                  <dd class="mt-1 text-xl font-semibold tabular-nums text-gray-900 dark:text-white">
                    {{ formatNumber(metrics?.worker_ticks ?? 0) }}
                  </dd>
                </div>
                <div class="rounded-xl bg-gray-50 p-3 dark:bg-dark-800/80">
                  <dt class="text-xs text-gray-500">{{ t('admin.connectionRisk.overview.eventsCreated') }}</dt>
                  <dd class="mt-1 text-xl font-semibold tabular-nums text-gray-900 dark:text-white">
                    {{ formatNumber(metrics?.events_created ?? 0) }}
                  </dd>
                </div>
              </dl>
            </div>
          </div>

          <div class="card p-5">
            <button
              type="button"
              class="text-sm font-medium text-primary-600 hover:underline dark:text-primary-400"
              @click="showRawRuntime = !showRawRuntime"
            >
              {{
                showRawRuntime
                  ? t('admin.connectionRisk.runtime.hideRaw')
                  : t('admin.connectionRisk.runtime.showRaw')
              }}
            </button>
            <pre
              v-if="showRawRuntime"
              class="mt-3 max-h-96 overflow-auto rounded-xl bg-gray-50 p-4 text-xs dark:bg-dark-800"
            >{{ JSON.stringify(runtime, null, 2) }}</pre>
          </div>
        </template>
      </section>
    </div>
  </AppLayout>
</template>
