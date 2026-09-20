<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { getNetworkTraffic, type NetworkTraffic } from '@/api/admin/networkTraffic'

const { locale } = useI18n()
const copy = computed(() => locale.value.startsWith('zh') ? {
  title: '实时流量', live: '实时 · 2 秒刷新', waiting: '正在采样…', offline: '连接中断，正在重试',
  unavailable: '流量采集暂不可用', source: '服务器公网', upload: '上传', download: '下载',
  history: '最近 5 分钟', total: '网卡累计', graph: '公网上传与下载流量曲线', collecting: '正在收集流量曲线'
} : {
  title: 'Live traffic', live: 'Live · every 2s', waiting: 'Sampling…', offline: 'Disconnected, retrying',
  unavailable: 'Traffic monitoring unavailable', source: 'Server public network', upload: 'Upload', download: 'Download',
  history: 'Last 5 minutes', total: 'Interface totals', graph: 'Public network upload and download traffic', collecting: 'Collecting traffic history'
})

const stats = ref<NetworkTraffic | null>(null)
const failed = ref(false)
let timer: ReturnType<typeof setTimeout> | undefined
let controller: AbortController | undefined
let mounted = false

const live = computed(() => !failed.value && !!stats.value?.available && !!stats.value?.ready)
const status = computed(() => failed.value ? copy.value.offline : stats.value && !stats.value.available
  ? copy.value.unavailable : live.value ? copy.value.live : copy.value.waiting)
const samples = computed(() => live.value ? (stats.value?.samples ?? []) : [])
const ceiling = computed(() => Math.max(1024, ...samples.value.flatMap(s => [s.upload_bps, s.download_bps])))

function formatBytes(value: number, rate = false): string {
  const units = rate ? ['B/s', 'KB/s', 'MB/s', 'GB/s'] : ['B', 'KB', 'MB', 'GB', 'TB']
  let n = Math.max(0, value), unit = 0
  while (n >= 1024 && unit < units.length - 1) { n /= 1024; unit++ }
  return `${n.toFixed(unit === 0 ? 0 : n >= 100 ? 0 : 1)} ${units[unit]}`
}

function points(key: 'upload_bps' | 'download_bps'): string {
  const end = stats.value?.timestamp ?? 0
  return samples.value.map(s => {
    const x = Math.max(0, Math.min(260, (s.timestamp - (end - 300000)) / 300000 * 260))
    const y = 30 - Math.min(1, s[key] / ceiling.value) * 27
    return `${x.toFixed(1)},${y.toFixed(1)}`
  }).join(' ')
}

async function poll() {
  if (!mounted || document.hidden) return
  const request = new AbortController()
  controller = request
  try {
    const result = await getNetworkTraffic(request.signal)
    if (!mounted || request.signal.aborted) return
    stats.value = result
    failed.value = false
  } catch {
    if (!mounted || request.signal.aborted) return
    failed.value = true
  } finally {
    if (controller === request) {
      controller = undefined
      if (mounted && !document.hidden) timer = setTimeout(poll, 2000)
    }
  }
}
function visibilityChanged() {
  clearTimeout(timer)
  controller?.abort()
  controller = undefined
  if (!document.hidden) {
    stats.value = null
    void poll()
  }
}

onMounted(() => {
  mounted = true
  document.addEventListener('visibilitychange', visibilityChanged)
  void poll()
})
onBeforeUnmount(() => {
  mounted = false
  clearTimeout(timer)
  controller?.abort()
  document.removeEventListener('visibilitychange', visibilityChanged)
})
</script>

<template>
  <section class="traffic-banner border-t border-blue-100/70 bg-blue-50/50 text-slate-800 dark:border-slate-700/60 dark:bg-slate-900/50 dark:text-slate-100" :aria-label="copy.title" data-testid="network-traffic-banner">
    <div class="traffic-content px-4 py-2.5 md:px-6">
      <div class="traffic-heading">
        <p class="flex items-center gap-2 text-xs font-semibold">
          <span class="h-1.5 w-1.5 shrink-0 rounded-full" :class="live ? 'bg-emerald-500' : failed ? 'bg-amber-500' : 'bg-slate-400'" aria-hidden="true"></span>
          {{ copy.title }}
          <span class="text-[10px] font-normal text-slate-500 dark:text-slate-400">{{ copy.source }}<span v-if="stats?.interfaces.length"> · {{ stats.interfaces.join(', ') }}</span></span>
        </p>
        <p class="mt-1 text-[10px] text-slate-500 dark:text-slate-400" role="status">{{ status }}</p>
      </div>

      <div class="traffic-rate tabular-nums">
        <span class="text-[11px] text-blue-600 dark:text-blue-400">↓ {{ copy.download }}</span>
        <span class="whitespace-nowrap text-base font-semibold tracking-tight" data-testid="download-rate">{{ live ? formatBytes(stats!.download_bps, true) : '—' }}</span>
      </div>
      <div class="traffic-rate tabular-nums">
        <span class="text-[11px] text-violet-600 dark:text-violet-400">↑ {{ copy.upload }}</span>
        <span class="whitespace-nowrap text-base font-semibold tracking-tight" data-testid="upload-rate">{{ live ? formatBytes(stats!.upload_bps, true) : '—' }}</span>
      </div>

      <div class="traffic-history">
        <div class="min-w-0 flex-1">
          <div class="flex items-center justify-between gap-2 text-[9px] text-slate-500 dark:text-slate-400"><span>{{ copy.history }}</span><span v-if="live" class="tabular-nums">{{ formatBytes(ceiling, true) }}</span></div>
          <div class="relative mt-0.5 overflow-hidden rounded bg-white/60 dark:bg-slate-950/40">
            <svg class="block h-8 w-full" viewBox="0 0 260 32" preserveAspectRatio="none" role="img" :aria-label="copy.graph">
              <path d="M0 15H260 M0 30H260" fill="none" stroke="currentColor" class="text-slate-200 dark:text-slate-700" stroke-width="0.6" stroke-dasharray="3 4" />
              <polyline v-if="samples.length > 1" :points="points('download_bps')" fill="none" stroke="#3b82f6" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round" vector-effect="non-scaling-stroke" />
              <polyline v-if="samples.length > 1" :points="points('upload_bps')" fill="none" stroke="#8b5cf6" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round" vector-effect="non-scaling-stroke" />
            </svg>
            <span v-if="samples.length < 2 && live" class="absolute inset-0 flex items-center justify-center text-[9px] text-slate-400">{{ copy.collecting }}</span>
          </div>
        </div>
        <div class="shrink-0 border-l border-blue-100 pl-3 text-[10px] tabular-nums text-slate-500 dark:border-slate-700 dark:text-slate-400">
          <p>{{ copy.total }}</p>
          <p class="mt-0.5 whitespace-nowrap">↓ {{ stats?.available ? formatBytes(stats.total_received) : '—' }}</p>
          <p class="whitespace-nowrap">↑ {{ stats?.available ? formatBytes(stats.total_sent) : '—' }}</p>
        </div>
      </div>
    </div>
  </section>
</template>

<style scoped>
.traffic-banner { container-type: inline-size; }
.traffic-content { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); align-items: center; gap: 8px 20px; }
.traffic-heading, .traffic-history { grid-column: 1 / -1; min-width: 0; }
.traffic-rate { display: flex; align-items: baseline; gap: 8px; min-width: 0; }
.traffic-history { display: flex; align-items: center; gap: 12px; }
@container (min-width: 760px) {
  .traffic-content { grid-template-columns: minmax(190px, 1fr) 104px 104px minmax(220px, 1.5fr); }
  .traffic-heading, .traffic-history { grid-column: auto; }
  .traffic-rate { flex-direction: column; gap: 2px; }
}
</style>
