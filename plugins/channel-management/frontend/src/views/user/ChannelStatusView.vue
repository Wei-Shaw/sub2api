<template>
  <!-- title/description 已上移到 channel-management manifest descriptions,
       host AppHeader 唯一渲染标题区. PluginPageLayout 继续作为 layout 容器使用,
       MonitorHero 内含状态条/窗口切换/刷新, 不需要再叠 FilterBar/PageActions. -->
  <PluginPageLayout>
    <div class="layout-section-fixed">
      <MonitorHero
        :overall-status="overallStatus"
        :interval-seconds="DEFAULT_INTERVAL_SECONDS"
        :window="currentWindow"
        :loading="loading"
        :auto-refresh="autoRefresh"
        @update:window="handleWindowChange"
        @refresh="manualReload"
      />
    </div>

    <div class="layout-section-scrollable">
      <MonitorCardGrid
        :items="items"
        :window="currentWindow"
        :countdown-seconds="countdown"
        :loading="loading"
        :detail-cache="detailCache"
        @card-click="openDetail"
      />
    </div>

    <MonitorDetailDialog
      :show="showDetail"
      :monitor-id="detailTarget?.id ?? null"
      :title="detailTitle"
      @close="closeDetail"
    />
  </PluginPageLayout>
</template>

<script setup lang="ts">
import { ref, reactive, computed, onMounted, onBeforeUnmount, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { PluginPageLayout } from '@sub2api/plugin-sdk'
import {
  list as listChannelMonitorViews,
  status as fetchChannelMonitorDetail,
  type UserMonitorView,
  type UserMonitorDetail,
} from '../../api/user/channelMonitor'
import MonitorHero, {
  type MonitorWindow,
  type OverallStatus,
} from '../../components/user/monitor/MonitorHero.vue'
import MonitorCardGrid from '../../components/user/monitor/MonitorCardGrid.vue'
import MonitorDetailDialog from '../../components/user/MonitorDetailDialog.vue'
import { DEFAULT_INTERVAL_SECONDS, STATUS_OPERATIONAL } from '../../utils/channelMonitorConstants'
import { useAutoRefresh } from '../../composables/useAutoRefresh'
import { getSdk } from '../../api/sdk'

const { t } = useI18n()
const sdk = getSdk()

// ── State ──
const items = ref<UserMonitorView[]>([])
const loading = ref(false)
const currentWindow = ref<MonitorWindow>('7d')
const detailCache = reactive<Record<number, UserMonitorDetail>>({})
const showDetail = ref(false)
const detailTarget = ref<UserMonitorView | null>(null)

let abortController: AbortController | null = null

const autoRefresh = useAutoRefresh({
  storageKey: 'channel-status-auto-refresh',
  intervals: [30, 60, 120] as const,
  defaultInterval: DEFAULT_INTERVAL_SECONDS,
  onRefresh: () => reload(true),
  shouldPause: () => document.hidden || loading.value,
})
const countdown = autoRefresh.countdown

// ── Computed ──
const overallStatus = computed<OverallStatus>(() => {
  if (items.value.length === 0) return 'operational'
  for (const it of items.value) {
    if (it.primary_status === 'failed' || it.primary_status === 'error') return 'degraded'
    if (it.primary_status !== STATUS_OPERATIONAL) return 'degraded'
  }
  return 'operational'
})

const detailTitle = computed(() => {
  return detailTarget.value?.name || t('channelStatus.detailTitle')
})

function extractMessage(err: unknown, fallback: string): string {
  const msg = err instanceof Error ? err.message : String(err)
  return msg || fallback
}

// ── Loaders ──
async function reload(silent = false) {
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
    sdk.notify.error(extractMessage(err, t('channelStatus.loadError')))
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
  // After base reload, refresh any cached detail records so non-7d availability
  // values stay in sync without forcing the user to switch tabs again.
  if (currentWindow.value !== '7d') {
    await Promise.all(items.value.map(it => loadDetail(it.id, true)))
  }
}

async function loadDetail(id: number, force = false) {
  if (!force && detailCache[id]) return
  try {
    detailCache[id] = await fetchChannelMonitorDetail(id)
  } catch (err: unknown) {
    sdk.notify.error(extractMessage(err, t('channelStatus.detailLoadError')))
  }
}

async function ensureDetailsForWindow() {
  if (currentWindow.value === '7d') return
  await Promise.all(items.value.map(it => loadDetail(it.id)))
}

// ── Handlers ──
async function handleWindowChange(value: MonitorWindow) {
  currentWindow.value = value
  await ensureDetailsForWindow()
}

function openDetail(row: UserMonitorView) {
  detailTarget.value = row
  showDetail.value = true
}

function closeDetail() {
  showDetail.value = false
  detailTarget.value = null
}

watch(items, () => {
  void ensureDetailsForWindow()
})

onMounted(() => {
  void reload(false)
  // Public-settings watch is intentionally not wired in plugin context;
  // auto-refresh defaults to enabled on first visit, but we respect the
  // user's persisted choice afterwards (localStorage as single source of truth).
  const STORAGE_KEY = 'channel-status-auto-refresh'
  let hasPersisted = false
  try { hasPersisted = localStorage.getItem(STORAGE_KEY) !== null } catch { /* ignore */ }
  if (!hasPersisted) {
    autoRefresh.setEnabled(true)
  } else if (autoRefresh.enabled.value) {
    autoRefresh.start()
    autoRefresh.resetCountdown()
  }
})

onBeforeUnmount(() => {
  if (abortController) abortController.abort()
})
</script>
