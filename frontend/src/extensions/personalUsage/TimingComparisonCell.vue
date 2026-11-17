<script setup lang="ts">
import { onBeforeUnmount, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import type { UsageLog } from '@/types'
import { durationSeverity, firstTokenSeverity, LATENCY_TEXT_CLASSES, formatUsageOutputRate } from '@/utils/latencyHealth'
import { fetchTimingComparison, timingDifference, timingDuration, type TimingComparison } from './timing'

const props = withDefaults(defineProps<{ row: UsageLog; allowComparison?: boolean }>(), { allowComparison: true })
const { t } = useI18n()
const result = ref<TimingComparison | null>(null)
const hovered = ref(false)
const position = ref({ left: '0px', top: '0px' })
const busy = ref(false)
let controller: AbortController | undefined
let requestVersion = 0, queriedAt = 0
let timer: ReturnType<typeof setTimeout> | undefined
function reset() {
  requestVersion++; clearTimeout(timer); controller?.abort()
  result.value = null; hovered.value = false; busy.value = false; queriedAt = 0
}
watch(() => [props.row.id, props.row.account_id, props.allowComparison], reset)
onBeforeUnmount(reset)
function enter(event: MouseEvent | FocusEvent) {
  hovered.value = true
  const rect = (event.currentTarget as HTMLElement).getBoundingClientRect()
  position.value = {
    left: `${Math.max(8, Math.min(rect.left, window.innerWidth - 376))}px`,
    top: `${Math.max(8, Math.min(rect.bottom + 6, window.innerHeight - 210))}px`,
  }
  clearTimeout(timer)
  timer = setTimeout(query, 300)
}
function leave() { hovered.value = false; clearTimeout(timer) }
async function query() {
  if (busy.value || !props.row.account_id || !props.allowComparison || (queriedAt && Date.now() - queriedAt < (result.value?.status === 'exact' ? 300000 : 30000))) return
  const version = ++requestVersion
  controller = new AbortController(); busy.value = true; result.value = null
  try {
    const comparison = await fetchTimingComparison(props.row.account_id, props.row.id, controller.signal)
    if (version === requestVersion) result.value = comparison
  } catch { /* Missing records, expired credentials and network errors show no comparison. */ }
  finally {
    if (version === requestVersion) { busy.value = false; queriedAt = Date.now() }
  }
}
</script>

<template>
  <div class="min-w-32 text-xs" data-testid="timing-hover-target" :tabindex="allowComparison ? 0 : undefined" :aria-describedby="hovered && result?.status === 'exact' ? `timing-${row.id}` : undefined" @mouseenter="enter" @mouseleave="leave" @focus="enter" @blur="leave" @keydown.esc="leave">
    <div class="grid grid-cols-[max-content_max-content] gap-x-2 gap-y-0.5">
      <span class="text-gray-400">{{ t('usage.latencyFirstToken') }}</span>
      <span class="tabular-nums" :class="row.first_token_ms == null ? 'text-gray-400' : LATENCY_TEXT_CLASSES[firstTokenSeverity(row.first_token_ms)]">{{ timingDuration(row.first_token_ms) }}</span>
      <span class="text-gray-400">{{ t('usage.latencyDuration') }}</span>
      <span class="tabular-nums" :class="row.duration_ms == null ? 'text-gray-400' : LATENCY_TEXT_CLASSES[durationSeverity(row.duration_ms)]">{{ timingDuration(row.duration_ms) }}</span>
      <span class="text-gray-400" :title="t('usage.outputTpsHint')">{{ t('usage.outputTps') }}</span>
      <span data-testid="output-tps" class="tabular-nums text-gray-700 dark:text-gray-300">{{ formatUsageOutputRate(row) }}</span>
    </div>
    <Teleport to="body">
      <div v-if="hovered && result?.status === 'exact' && result.station" :id="`timing-${row.id}`" role="tooltip" :style="position" class="pointer-events-none fixed z-[100000050] w-[min(360px,calc(100vw-16px))] rounded-lg border border-gray-200 bg-white p-4 shadow-xl dark:border-dark-600 dark:bg-dark-800">
        <p class="mb-2 text-sm font-medium">本地与上游耗时 · 请求 ID 已匹配</p>
        <table class="w-full text-left text-xs"><thead><tr class="text-gray-500"><th>指标</th><th>本地</th><th>上游</th><th>差值</th></tr></thead><tbody>
          <tr><td class="py-1">首字</td><td>{{ timingDuration(result.local.first_token_ms) }}</td><td>{{ timingDuration(result.station.first_token_ms) }}</td><td>{{ timingDifference(result.first_token_difference_ms) }}</td></tr>
          <tr><td class="py-1">总耗时</td><td>{{ timingDuration(result.local.duration_ms) }}</td><td>{{ timingDuration(result.station.duration_ms) }}</td><td>{{ timingDifference(result.duration_difference_ms) }}</td></tr>
        </tbody></table>
        <p v-if="result.station.precision_ms === 1000" class="mt-2 text-gray-500">New API 总耗时精度为秒。</p>
        <p class="mt-2 text-gray-500">差值为本地减上游；两端测量范围不同，不能直接视为网络延迟。</p>
      </div>
    </Teleport>
  </div>
</template>
