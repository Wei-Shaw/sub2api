<template>
  <button
    type="button"
    class="group text-left w-full px-4 py-2 rounded-xl bg-white/70 backdrop-blur-xl border border-gray-200/80 shadow-sm dark:bg-dark-800/60 dark:border-dark-700/70 hover:shadow-md dark:hover:border-primary-500/30 hover:border-gray-300 transition-all duration-200 ease-out flex items-center gap-3"
    @click="emit('click')"
  >
    <!-- ① 图标 + 名称 + 供应商徽章 + 模型 + 分组 -->
    <span
      class="w-8 h-8 rounded-lg ring-1 ring-black/5 dark:ring-white/10 grid place-items-center flex-shrink-0"
      :class="[providerGradient(item.provider), providerTintClass]"
    >
      <ProviderIcon :provider="item.provider" :size="17" />
    </span>

    <span class="min-w-0 flex-shrink flex items-center gap-1.5">
      <span class="text-sm font-semibold whitespace-nowrap text-gray-900 dark:text-gray-100">
        {{ item.name }}
      </span>
      <span
        class="inline-flex items-center rounded px-1 py-0.5 text-[10px] font-medium flex-shrink-0"
        :class="providerBadgeClass(item.provider)"
      >
        {{ providerLabel(item.provider) }}
      </span>
      <span class="font-mono text-[11px] truncate text-gray-500 dark:text-gray-400 max-w-[10rem]">
        {{ formatMonitorModel(item.primary_model) }}
      </span>
      <span
        v-if="item.group_name"
        class="inline-flex items-center rounded px-1 py-0.5 text-[10px] font-medium bg-gray-100 text-gray-600 dark:bg-dark-700 dark:text-gray-300 flex-shrink-0"
      >
        {{ item.group_name }}
      </span>
    </span>

    <!-- ② 状态徽章 -->
    <span
      class="px-2 py-0.5 rounded-full text-[11px] font-semibold flex-shrink-0"
      :class="statusBadgeClass(item.primary_status)"
    >
      {{ statusLabel(item.primary_status) }}
    </span>

    <!-- ③ 延迟 / Ping（紧凑指标区，空间紧张时可让位给名称与时间线） -->
    <span class="flex items-center gap-3 flex-shrink min-w-0">
      <span class="inline-flex items-baseline gap-1">
        <Icon name="bolt" size="xs" class="text-gray-400 self-center" />
        <span class="text-[10px] uppercase tracking-wider text-gray-400 hidden lg:inline">{{ t('monitorCommon.dialogLatency') }}</span>
        <span class="text-sm font-bold font-mono tabular-nums text-gray-900 dark:text-gray-100">
          {{ formatLatency(item.primary_latency_ms) }}
        </span>
        <span class="text-[10px] text-gray-400">ms</span>
      </span>
      <span class="inline-flex items-baseline gap-1">
        <Icon name="globe" size="xs" class="text-gray-400 self-center" />
        <span class="text-[10px] uppercase tracking-wider text-gray-400 hidden lg:inline">{{ t('monitorCommon.endpointPing') }}</span>
        <span class="text-sm font-bold font-mono tabular-nums text-gray-900 dark:text-gray-100">
          {{ formatLatency(item.primary_ping_latency_ms) }}
        </span>
        <span class="text-[10px] text-gray-400">ms</span>
      </span>
    </span>

    <!-- ④ 配额快照（仅在开启且有时才显示） -->
    <span v-if="quotaVisible" class="flex-shrink-0 max-w-[16rem] min-w-0 overflow-hidden">
      <MonitorQuotaView :snapshot="item.latest_quota" />
    </span>

    <!-- ⑤ 可用率 + 额外模型数 -->
    <span class="flex items-center gap-1.5 flex-shrink-0">
      <span class="text-[10px] uppercase tracking-wider text-gray-400 whitespace-nowrap">
        {{ t('monitorCommon.availabilityPrefix') }} · {{ t(`channelStatus.windowTab.${window}`) }}
      </span>
      <span class="text-lg font-bold tabular-nums leading-none" :style="availabilityColorStyle">
        {{ availabilityDisplay }}
      </span>
      <span class="text-[11px] font-semibold leading-none" :style="availabilityColorStyle">%</span>
      <span
        v-if="extraModelsCountLabel"
        class="text-[10px] text-gray-400 whitespace-nowrap"
      >
        {{ extraModelsCountLabel }}
      </span>
    </span>

    <!-- ⑥ 时间线（横向铺满剩余空间） -->
    <span class="flex flex-col gap-0.5 flex-1 min-w-[7rem]">
      <span class="flex items-center justify-between text-[9px] font-semibold uppercase tracking-widest text-gray-400">
        <span>{{ t('monitorCommon.history60pts', { n: timelineLength }) }}</span>
        <span class="tabular-nums">{{ t('monitorCommon.nextUpdateIn', { n: countdownSeconds }) }}</span>
      </span>
      <span class="flex items-end gap-[1px] h-3.5 w-full">
        <span
          v-for="(bar, idx) in timelineBars"
          :key="idx"
          class="flex-1 min-w-0 rounded-sm"
          :class="bar.colorClass"
          :style="{ height: bar.heightPct + '%' }"
          :title="bar.title"
        ></span>
      </span>
    </span>
  </button>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import type { UserMonitorView } from '@/api/channelMonitor'
import {
  useChannelMonitorFormat,
  providerGradient,
  hslForPct,
} from '@/composables/useChannelMonitorFormat'
import { isChannelMonitorQuotaVisible } from '@/utils/featureFlags'
import Icon from '@/components/icons/Icon.vue'
import ProviderIcon from './ProviderIcon.vue'
import MonitorQuotaView from '@/components/common/MonitorQuotaView.vue'
import { buildTimelineBars } from './timelineBars'

// 图标配色与 utils/platformColors.ts 的平台色对齐（新 4 家）。
const PROVIDER_TINT: Record<string, string> = {
  openai: 'text-emerald-600 dark:text-emerald-300',
  anthropic: 'text-orange-600 dark:text-orange-300',
  gemini: 'text-sky-600 dark:text-sky-300',
  grok: 'text-zinc-700 dark:text-zinc-200',
  antigravity: 'text-purple-600 dark:text-purple-300',
  kimi: 'text-pink-600 dark:text-pink-300',
  zhipu: 'text-indigo-600 dark:text-indigo-300',
  deepseek: 'text-teal-600 dark:text-teal-300',
  opencode_go: 'text-amber-700 dark:text-amber-300',
}

const props = defineProps<{
  item: UserMonitorView
  window: '7d' | '15d' | '30d'
  availabilityValue: number | null
  countdownSeconds: number
}>()

const emit = defineEmits<{
  (e: 'click'): void
}>()

const { t } = useI18n()
const {
  statusLabel,
  statusBadgeClass,
  providerLabel,
  providerBadgeClass,
  formatLatency,
  formatMonitorModel,
  formatRelativeTime,
} = useChannelMonitorFormat()

const providerTintClass = computed(() =>
  PROVIDER_TINT[props.item.provider] ?? 'text-gray-500 dark:text-gray-300'
)

const quotaVisible = computed(
  () => isChannelMonitorQuotaVisible() && !!props.item.latest_quota
)

const availabilityDisplay = computed(() => {
  const v = props.availabilityValue
  if (v === null || v === undefined || Number.isNaN(v)) {
    return t('monitorCommon.latencyEmpty')
  }
  return v.toFixed(2)
})

const availabilityColorStyle = computed(() => {
  const colour = hslForPct(props.availabilityValue)
  return colour ? { color: colour } : { color: 'rgb(156 163 175)' }
})

const extraModelsCountLabel = computed(() => {
  const count = props.item.extra_models?.length ?? 0
  if (count === 0) return undefined
  return t('monitorCommon.extraModelsCount', { n: count })
})

const timelineLength = 60
const timelineBars = computed(() =>
  buildTimelineBars(props.item.timeline, timelineLength, {
    formatLatency,
    formatRelativeTime,
    statusLabel,
  })
)
</script>
