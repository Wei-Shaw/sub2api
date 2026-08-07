<template>
  <div v-if="snapshot && snapshot.tiers.length" class="space-y-2">
    <!-- 窗口配额进度条（five_hour / weekly_limit） -->
    <div v-for="tier in windowTiers" :key="tier.name">
      <div class="flex items-center justify-between text-xs">
        <span class="text-gray-500 dark:text-gray-400">{{ tierLabel(tier.name) }}</span>
        <span class="font-medium text-gray-700 dark:text-gray-300">
          {{ formatUtilization(tier.utilization) }}
        </span>
      </div>
      <div class="mt-1 h-1.5 w-full overflow-hidden rounded-full bg-gray-100 dark:bg-dark-700">
        <div
          class="h-full rounded-full transition-all"
          :class="barClass(tier.utilization)"
          :style="{ width: `${clampPct(tier.utilization)}%` }"
        ></div>
      </div>
      <div v-if="tier.resets_at" class="mt-0.5 text-[10px] text-gray-400 dark:text-gray-500">
        {{ t('monitorCommon.quota.resetsAt', { time: formatResetTime(tier.resets_at) }) }}
      </div>
    </div>

    <!-- 余额（balance）：多币种账户每个币种一行（如 CNY + USD） -->
    <div v-for="(tier, idx) in balanceTiers" :key="`balance-${idx}`" class="flex items-baseline justify-between">
      <span class="text-xs text-gray-500 dark:text-gray-400">
        {{ t('monitorCommon.quota.balance') }}
        <template v-if="balanceTiers.length > 1 && tier.currency"> ({{ tier.currency }})</template>
      </span>
      <span class="text-sm font-semibold text-gray-900 dark:text-gray-100">
        {{ tier.balance }} <span class="text-xs font-normal text-gray-400">{{ tier.currency }}</span>
      </span>
    </div>

    <!-- 套餐等级 / 余额可用性 -->
    <div class="flex flex-wrap items-center gap-2 text-[10px]">
      <span
        v-if="snapshot.plan_level"
        class="inline-flex items-center rounded-md bg-gray-100 px-1.5 py-0.5 font-medium text-gray-600 dark:bg-dark-700 dark:text-gray-300"
      >
        {{ t('monitorCommon.quota.planLevel', { level: snapshot.plan_level }) }}
      </span>
      <span
        v-if="snapshot.available === true"
        class="inline-flex items-center rounded-md bg-emerald-100 px-1.5 py-0.5 font-medium text-emerald-700 dark:bg-emerald-500/15 dark:text-emerald-300"
      >
        {{ t('monitorCommon.quota.available') }}
      </span>
      <span
        v-else-if="snapshot.available === false"
        class="inline-flex items-center rounded-md bg-red-100 px-1.5 py-0.5 font-medium text-red-700 dark:bg-red-500/15 dark:text-red-300"
      >
        {{ t('monitorCommon.quota.unavailable') }}
      </span>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import type { MonitorQuotaSnapshot, MonitorQuotaTier } from '@/api/admin/channelMonitor'

const props = defineProps<{
  snapshot: MonitorQuotaSnapshot | null | undefined
}>()

const { t } = useI18n()

const windowTiers = computed<MonitorQuotaTier[]>(() =>
  (props.snapshot?.tiers ?? []).filter((t) => t.name === 'five_hour' || t.name === 'weekly_limit')
)
const balanceTiers = computed<MonitorQuotaTier[]>(() =>
  (props.snapshot?.tiers ?? []).filter((t) => t.name === 'balance')
)

function tierLabel(name: string): string {
  if (name === 'five_hour') return t('monitorCommon.quota.fiveHour')
  if (name === 'weekly_limit') return t('monitorCommon.quota.weekly')
  return name
}

function clampPct(v: number | undefined): number {
  if (v == null || Number.isNaN(v)) return 0
  return Math.max(0, Math.min(100, v))
}

function formatUtilization(v: number | undefined): string {
  if (v == null || Number.isNaN(v)) return '-'
  return t('monitorCommon.quota.used', { pct: v.toFixed(0) })
}

function barClass(v: number | undefined): string {
  const pct = clampPct(v)
  if (pct >= 90) return 'bg-red-500'
  if (pct >= 70) return 'bg-amber-500'
  return 'bg-emerald-500'
}

function formatResetTime(iso: string): string {
  const ts = Date.parse(iso)
  if (Number.isNaN(ts)) return iso
  return new Date(ts).toLocaleString()
}
</script>
