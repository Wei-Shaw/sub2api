<template>
  <span v-if="props.quotas === undefined" class="text-xs text-gray-400 dark:text-gray-500">…</span>
  <span v-else-if="configured.length === 0" class="text-xs text-gray-400 dark:text-gray-500">
    {{ t('admin.users.platformQuota.cellNotConfigured') REDACTEDREDACTED
  </span>
  <div v-else class="space-y-0.5 text-xs">
    <div
      v-for="row in configured"
      :key="row.platform"
      class="flex items-center gap-2 whitespace-nowrap"
    >
      <span class="w-20 shrink-0 font-mono text-gray-700 dark:text-gray-300">{{ row.platform REDACTEDREDACTED</span>
      <span class="text-gray-500 dark:text-gray-400">
        {{ t('admin.users.platformQuota.windowDaily') REDACTEDREDACTED
        <span class="text-gray-900 dark:text-white">{{ fmtUsd(row.daily_usage_usd) REDACTEDREDACTED/{{ fmtLimit(row.daily_limit_usd) REDACTEDREDACTED</span>
      </span>
      <span class="text-gray-500 dark:text-gray-400">
        {{ t('admin.users.platformQuota.windowWeekly') REDACTEDREDACTED
        <span class="text-gray-900 dark:text-white">{{ fmtUsd(row.weekly_usage_usd) REDACTEDREDACTED/{{ fmtLimit(row.weekly_limit_usd) REDACTEDREDACTED</span>
      </span>
      <span class="text-gray-500 dark:text-gray-400">
        {{ t('admin.users.platformQuota.windowMonthly') REDACTEDREDACTED
        <span class="text-gray-900 dark:text-white">{{ fmtUsd(row.monthly_usage_usd) REDACTEDREDACTED/{{ fmtLimit(row.monthly_limit_usd) REDACTEDREDACTED</span>
      </span>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed REDACTED from 'vue'
import { useI18n REDACTED from 'vue-i18n'
import type { PlatformQuotaItem, PlatformQuotaPlatform REDACTED from '@/api/admin/users'

const props = defineProps<{ quotas?: PlatformQuotaItem[] REDACTED>()
const { t REDACTED = useI18n()

const PLATFORM_ORDER: PlatformQuotaPlatform[] = ['anthropic', 'openai', 'gemini', 'antigravity', 'grok']

// 仅展示「至少一档限额非空」的平台（配额列，非用量列）
const configured = computed(() => {
  if (!props.quotas) return []
  return props.quotas
    .filter(
      (q) =>
        q.daily_limit_usd != null ||
        q.weekly_limit_usd != null ||
        q.monthly_limit_usd != null
    )
    .slice()
    .sort((a, b) => PLATFORM_ORDER.indexOf(a.platform) - PLATFORM_ORDER.indexOf(b.platform))
REDACTED)

// 去尾零、最多 2 位小数：100→"100"，90.5→"90.5"，0.42→"0.42"
function fmtUsd(n: number): string {
  if (n == null || Number.isNaN(n)) return '0'
  return String(Math.round(n * 100) / 100)
REDACTED
function fmtLimit(n: number | null): string {
  return n == null ? '—' : fmtUsd(n)
REDACTED
</script>
