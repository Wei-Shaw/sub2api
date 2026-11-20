<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { RouterLink } from 'vue-router'
import { listExpiringAccounts, type ExpiringAccount } from '@/api/admin/opsTrace'
import { formatDateTimeToMinute } from '@/utils/format'
import {
  RENEWAL_GROUP_ORDER,
  countAlertScope,
  countRenewalGroups,
  describeRemaining,
  groupRenewals,
  isExpired,
  type RenewalGroupKey
} from './renewalGroups'

interface Props {
  /** 由 OpsDashboard 统一驱动的刷新信号 */
  refreshToken?: number
}

const props = withDefaults(defineProps<Props>(), {
  refreshToken: 0
})

const { t } = useI18n()

const WINDOW_OPTIONS = [3, 7, 30] as const
type WindowDays = (typeof WINDOW_OPTIONS)[number]

const days = ref<WindowDays>(7)
const items = ref<ExpiringAccount[]>([])
const loading = ref(false)
const loaded = ref(false)
const errorMessage = ref('')
let fetchSeq = 0

async function loadData() {
  const seq = ++fetchSeq
  loading.value = true
  errorMessage.value = ''
  try {
    const data = await listExpiringAccounts({ days: days.value })
    if (seq !== fetchSeq) return
    items.value = Array.isArray(data) ? data : []
    loaded.value = true
  } catch (err: any) {
    if (seq !== fetchSeq) return
    console.error('[OpsRenewalCard] Failed to load expiring accounts', err)
    errorMessage.value = err?.message || t('admin.traceRenewal.loadFailed')
  } finally {
    if (seq === fetchSeq) loading.value = false
  }
}

function selectWindow(n: WindowDays) {
  if (days.value === n) return
  days.value = n
}

watch(days, () => loadData())
watch(
  () => props.refreshToken,
  () => loadData()
)
onMounted(() => loadData())

const groups = computed(() => groupRenewals(items.value))
const counts = computed(() => countRenewalGroups(items.value))
const alertScopeCount = computed(() => countAlertScope(items.value))

/** 头部计数条：「7 天以上」只在 30 天窗口下展示 */
const summaryKeys = computed<RenewalGroupKey[]>(() =>
  RENEWAL_GROUP_ORDER.filter((k) => k !== 'later' || days.value > 7 || counts.value.later > 0)
)

const GROUP_STYLE: Record<RenewalGroupKey, { dot: string; chip: string; bar: string; text: string }> = {
  expiredActive: {
    dot: 'bg-red-500',
    chip: 'bg-red-50 text-red-700 ring-red-200 dark:bg-red-900/30 dark:text-red-300 dark:ring-red-800/60',
    bar: 'border-red-500 bg-red-50/70 dark:bg-red-900/20',
    text: 'text-red-600 dark:text-red-400'
  },
  expiredPaused: {
    dot: 'bg-gray-400 dark:bg-gray-500',
    chip: 'bg-gray-100 text-gray-600 ring-gray-200 dark:bg-dark-700 dark:text-gray-300 dark:ring-dark-600',
    bar: 'border-gray-400 bg-gray-50 dark:border-gray-500 dark:bg-dark-900/60',
    text: 'text-gray-500 dark:text-gray-400'
  },
  within24h: {
    dot: 'bg-orange-500',
    chip: 'bg-orange-50 text-orange-700 ring-orange-200 dark:bg-orange-900/30 dark:text-orange-300 dark:ring-orange-800/60',
    bar: 'border-orange-500 bg-orange-50/70 dark:bg-orange-900/20',
    text: 'text-orange-600 dark:text-orange-400'
  },
  within3d: {
    dot: 'bg-yellow-400',
    chip: 'bg-yellow-50 text-yellow-800 ring-yellow-200 dark:bg-yellow-900/30 dark:text-yellow-300 dark:ring-yellow-800/60',
    bar: 'border-yellow-400 bg-yellow-50/70 dark:bg-yellow-900/20',
    text: 'text-yellow-700 dark:text-yellow-400'
  },
  within7d: {
    dot: 'bg-sky-400',
    chip: 'bg-sky-50 text-sky-700 ring-sky-200 dark:bg-sky-900/20 dark:text-sky-300 dark:ring-sky-800/60',
    bar: 'border-sky-400 bg-sky-50/60 dark:bg-sky-900/10',
    text: 'text-gray-700 dark:text-gray-300'
  },
  later: {
    dot: 'bg-slate-300 dark:bg-slate-500',
    chip: 'bg-slate-50 text-slate-600 ring-slate-200 dark:bg-dark-700 dark:text-slate-300 dark:ring-dark-600',
    bar: 'border-slate-300 bg-slate-50/60 dark:border-slate-600 dark:bg-dark-900/40',
    text: 'text-gray-600 dark:text-gray-400'
  }
}

function groupHint(key: RenewalGroupKey): string {
  if (key === 'expiredActive' || key === 'expiredPaused') return t(`admin.traceRenewal.groupHints.${key}`)
  return ''
}

function remainingText(a: ExpiringAccount): string {
  const parts = describeRemaining(a.hours_left, isExpired(a))
  const unit = parts.unit === 'minutes' ? 'Minutes' : parts.unit === 'hours' ? 'Hours' : 'Days'
  return t(`admin.traceRenewal.remaining.${parts.expired ? 'expired' : 'left'}${unit}`, { n: parts.value })
}

type SchedulingState = 'active' | 'paused' | 'disabled' | 'error'

function schedulingState(a: ExpiringAccount): SchedulingState {
  if (a.status === 'disabled') return 'disabled'
  if (a.status === 'error') return 'error'
  if (a.status === 'active' && a.schedulable) return 'active'
  return 'paused'
}

const SCHEDULING_CLASS: Record<SchedulingState, string> = {
  active: 'bg-green-50 text-green-700 dark:bg-green-900/30 dark:text-green-300',
  paused: 'bg-gray-100 text-gray-600 dark:bg-dark-700 dark:text-gray-300',
  disabled: 'bg-gray-100 text-gray-500 dark:bg-dark-700 dark:text-gray-400',
  error: 'bg-red-50 text-red-600 dark:bg-red-900/30 dark:text-red-300'
}

function accountLink(a: ExpiringAccount) {
  // 账号管理页按名称搜索定位（不改账号管理页逻辑）
  return { path: '/admin/accounts', query: { search: a.name } }
}

function formatExpiresAt(v: string): string {
  return formatDateTimeToMinute(v) || v
}
</script>

<template>
  <div
    class="flex h-full flex-col rounded-3xl bg-white p-6 shadow-sm ring-1 ring-gray-900/5 dark:bg-dark-800 dark:ring-dark-700"
    data-testid="ops-renewal-card"
  >
    <!-- 头部 -->
    <div class="mb-3 flex flex-wrap items-center justify-between gap-3">
      <h3 class="flex items-center gap-2 text-sm font-bold text-gray-900 dark:text-white">
        <svg class="h-4 w-4 text-orange-500" fill="none" viewBox="0 0 24 24" stroke="currentColor">
          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 8v4l3 3m6-3a9 9 0 11-18 0 9 9 0 0118 0z" />
        </svg>
        {{ t('admin.traceRenewal.title') }}
      </h3>
      <div class="flex items-center gap-2">
        <span class="text-[11px] text-gray-500 dark:text-gray-400">{{ t('admin.traceRenewal.windowLabel') }}</span>
        <div class="flex rounded-lg bg-gray-100 p-0.5 dark:bg-dark-700" role="group">
          <button
            v-for="n in WINDOW_OPTIONS"
            :key="n"
            type="button"
            class="rounded-md px-2 py-0.5 text-[11px] font-semibold transition-colors"
            :class="days === n
              ? 'bg-white text-gray-900 shadow-sm dark:bg-dark-600 dark:text-white'
              : 'text-gray-500 hover:text-gray-800 dark:text-gray-400 dark:hover:text-gray-200'"
            :data-testid="`renewal-window-${n}`"
            :aria-pressed="days === n"
            @click="selectWindow(n)"
          >
            {{ t('admin.traceRenewal.windowDays', { n }) }}
          </button>
        </div>
        <button
          type="button"
          class="flex items-center gap-1 rounded-lg bg-gray-100 px-2 py-1 text-[11px] font-semibold text-gray-700 transition-colors hover:bg-gray-200 disabled:cursor-not-allowed disabled:opacity-50 dark:bg-dark-700 dark:text-gray-300 dark:hover:bg-dark-600"
          :disabled="loading"
          :title="t('admin.traceRenewal.refresh')"
          @click="loadData"
        >
          <svg class="h-3 w-3" :class="{ 'animate-spin': loading }" fill="none" viewBox="0 0 24 24" stroke="currentColor">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15" />
          </svg>
          {{ t('admin.traceRenewal.refresh') }}
        </button>
      </div>
    </div>

    <!-- 各组数量 -->
    <div class="mb-2 flex flex-wrap gap-2" data-testid="renewal-summary">
      <span
        v-for="key in summaryKeys"
        :key="key"
        class="inline-flex items-center gap-1.5 rounded-full px-2.5 py-1 text-[11px] font-semibold ring-1 ring-inset"
        :class="[GROUP_STYLE[key].chip, counts[key] === 0 ? 'opacity-50' : '']"
        :data-testid="`renewal-count-${key}`"
      >
        <span class="h-1.5 w-1.5 rounded-full" :class="GROUP_STYLE[key].dot"></span>
        {{ t(`admin.traceRenewal.groups.${key}`) }}
        <span class="font-mono">{{ counts[key] }}</span>
      </span>
    </div>

    <!-- 告警口径说明 -->
    <p class="mb-3 text-[11px] leading-relaxed text-gray-500 dark:text-gray-400" data-testid="renewal-alert-hint">
      {{ t('admin.traceRenewal.alertScopeHint', { n: alertScopeCount }) }}
    </p>

    <!-- 出错 -->
    <div
      v-if="errorMessage"
      class="mb-3 flex items-center justify-between gap-2 rounded-xl bg-red-50 p-2.5 text-xs text-red-600 dark:bg-red-900/20 dark:text-red-400"
      data-testid="renewal-error"
    >
      <span>{{ errorMessage }}</span>
      <button type="button" class="font-semibold underline hover:no-underline" @click="loadData">
        {{ t('admin.traceRenewal.retry') }}
      </button>
    </div>

    <!-- 首次加载 -->
    <div
      v-if="loading && !loaded"
      class="flex flex-1 items-center justify-center py-10 text-sm text-gray-500 dark:text-gray-400"
      data-testid="renewal-loading"
    >
      {{ t('admin.traceRenewal.loading') }}
    </div>

    <!-- 空状态 -->
    <div
      v-else-if="loaded && items.length === 0 && !errorMessage"
      class="flex flex-1 items-center justify-center rounded-xl border border-dashed border-gray-200 py-10 text-sm text-gray-500 dark:border-dark-700 dark:text-gray-400"
      data-testid="renewal-empty"
    >
      {{ t('admin.traceRenewal.empty', { days }) }}
    </div>

    <!-- 列表 -->
    <div
      v-else-if="items.length > 0"
      class="custom-scrollbar max-h-[420px] overflow-auto rounded-xl border border-gray-200 dark:border-dark-700"
    >
      <table class="min-w-full text-xs">
        <thead class="sticky top-0 z-10 bg-gray-50 dark:bg-dark-900">
          <tr class="text-left text-[10px] font-bold uppercase tracking-wider text-gray-500 dark:text-gray-400">
            <th class="px-3 py-2">{{ t('admin.traceRenewal.columns.name') }}</th>
            <th class="px-3 py-2">{{ t('admin.traceRenewal.columns.platform') }}</th>
            <th class="px-3 py-2">{{ t('admin.traceRenewal.columns.remaining') }}</th>
            <th class="px-3 py-2">{{ t('admin.traceRenewal.columns.expiresAt') }}</th>
            <th class="px-3 py-2">{{ t('admin.traceRenewal.columns.autoPause') }}</th>
            <th class="px-3 py-2">{{ t('admin.traceRenewal.columns.scheduling') }}</th>
          </tr>
        </thead>
        <tbody v-for="group in groups" :key="group.key" :data-testid="`renewal-group-${group.key}`">
          <tr>
            <td colspan="6" class="border-l-4 px-3 py-1.5" :class="GROUP_STYLE[group.key].bar">
              <span class="text-[11px] font-bold" :class="GROUP_STYLE[group.key].text">
                {{ t(`admin.traceRenewal.groups.${group.key}`) }} · {{ group.items.length }}
              </span>
              <span v-if="groupHint(group.key)" class="ml-2 text-[11px] text-gray-500 dark:text-gray-400">
                {{ groupHint(group.key) }}
              </span>
            </td>
          </tr>
          <tr
            v-for="a in group.items"
            :key="a.id"
            class="border-t border-gray-100 hover:bg-gray-50 dark:border-dark-700 dark:hover:bg-dark-700/50"
            :data-testid="`renewal-row-${a.id}`"
          >
            <td class="max-w-[220px] px-3 py-2">
              <RouterLink
                :to="accountLink(a)"
                class="block truncate font-medium text-primary-600 hover:underline dark:text-primary-400"
                :title="t('admin.traceRenewal.openAccount')"
              >
                {{ a.name }}
              </RouterLink>
              <span class="text-[10px] text-gray-400 dark:text-gray-500">#{{ a.id }}</span>
            </td>
            <td class="px-3 py-2">
              <span class="rounded bg-gray-100 px-1.5 py-0.5 text-[10px] font-semibold uppercase text-gray-600 dark:bg-dark-700 dark:text-gray-300">
                {{ a.platform }}
              </span>
            </td>
            <td class="whitespace-nowrap px-3 py-2 font-semibold" :class="GROUP_STYLE[group.key].text">
              {{ remainingText(a) }}
            </td>
            <td class="whitespace-nowrap px-3 py-2 font-mono text-gray-600 dark:text-gray-300">
              {{ formatExpiresAt(a.expires_at) }}
            </td>
            <td class="whitespace-nowrap px-3 py-2">
              <span :class="a.auto_pause_on_expired ? 'text-green-600 dark:text-green-400' : 'text-gray-400 dark:text-gray-500'">
                {{ a.auto_pause_on_expired ? t('admin.traceRenewal.autoPauseOn') : t('admin.traceRenewal.autoPauseOff') }}
              </span>
            </td>
            <td class="whitespace-nowrap px-3 py-2">
              <span class="rounded-full px-2 py-0.5 text-[10px] font-semibold" :class="SCHEDULING_CLASS[schedulingState(a)]">
                {{ t(`admin.traceRenewal.scheduling.${schedulingState(a)}`) }}
              </span>
            </td>
          </tr>
        </tbody>
      </table>
    </div>
  </div>
</template>
